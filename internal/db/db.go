package db

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vdfusion/internal/neural"

	"github.com/viant/sqlite-vec/vec"
	"github.com/viant/sqlite-vec/vector"

	_ "modernc.org/sqlite"
)

// Shadow table used by the file_embeddings vec virtual table.
// viant/sqlite-vec stores documents here; the virtual table is for MATCH queries.
const embeddingsShadowTable = "_vec_file_embeddings"

type FileRecord struct {
	ID               int64
	Path             string
	Size             int64
	Modified         int64
	Duration         float64
	Width            int
	Height           int
	PHashV2s         []uint64
	Codec            string
	Bitrate          int64
	FPS              float64
	Warnings         []string
	IdentifierHash   string
	NeuralEmbeddings [][]float32 // L2-normalised CLIP ViT-B/32 embeddings, one per frame
}

func (r FileRecord) GetIdentifierHash() string {
	input := fmt.Sprintf("%d_%d", r.Size, r.Modified)
	hash := sha256.Sum256([]byte(input))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

type IgnoredGroup struct {
	ID               int64    `json:"id"`
	Label            string   `json:"label"`
	IdentifierHashes []string `json:"identifier_hashes"`
	ResolvedPaths    []string `json:"resolved_paths"`
}

type Database struct {
	conn *sql.DB
	path string
}

func New(path string) (*Database, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Single connection keeps vec virtual-table module registration visible
	// across all statements (required by viant/sqlite-vec + modernc).
	db.SetMaxOpenConns(1)

	// modernc applies vtab modules only to connections opened AFTER registration.
	// Register before Ping/Exec so the first real connection has the vec module.
	if err := vec.Register(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to register sqlite-vec: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &Database{conn: db, path: path}

	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}
	_, err = db.Exec("PRAGMA busy_timeout=5000;")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := d.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	if err := d.ensureVectorTables(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ensure sqlite-vec tables: %w", err)
	}

	if err := d.migrateLegacyEmbeddings(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate legacy embeddings: %w", err)
	}

	return d, nil
}

func (d *Database) Close() error {
	return d.conn.Close()
}

func (d *Database) GetPath() string {
	return d.path
}

func (d *Database) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE,
		size INTEGER,
		modified INTEGER,
		duration REAL,
		width INTEGER DEFAULT 0,
		height INTEGER DEFAULT 0,
		phashes BLOB,
		codec TEXT,
		bitrate INTEGER,
		fps REAL,
		warnings TEXT DEFAULT '',
		neural_v1 BLOB,
		flags INTEGER DEFAULT 0,
		identifier_hash TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
	CREATE INDEX IF NOT EXISTS idx_files_content ON files(size, modified);

	CREATE TABLE IF NOT EXISTS ignored_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		label TEXT
	);

	CREATE TABLE IF NOT EXISTS ignored_group_items (
		group_id INTEGER,
		identifier_hash TEXT,
		FOREIGN KEY(group_id) REFERENCES ignored_groups(id) ON DELETE CASCADE,
		UNIQUE(group_id, identifier_hash)
	);
	CREATE INDEX IF NOT EXISTS idx_igi_hash ON ignored_group_items(identifier_hash);

	CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`

	_, err := d.conn.Exec(query)
	if err != nil {
		return err
	}

	// Ensure columns exist on older databases (ALTER fails harmlessly if present).
	ensureColumn(d.conn, "files", "warnings", "TEXT DEFAULT ''")
	ensureColumn(d.conn, "files", "width", "INTEGER DEFAULT 0")
	ensureColumn(d.conn, "files", "height", "INTEGER DEFAULT 0")
	ensureColumn(d.conn, "files", "identifier_hash", "TEXT")
	ensureColumn(d.conn, "files", "neural_v1", "BLOB")

	_, _ = d.conn.Exec("CREATE INDEX IF NOT EXISTS idx_files_hash ON files(identifier_hash)")

	// Backfill missing identifier hashes.
	var missingCount int
	err = d.conn.QueryRow("SELECT COUNT(*) FROM files WHERE identifier_hash IS NULL OR identifier_hash = ''").Scan(&missingCount)
	if err == nil && missingCount > 0 {
		fmt.Printf("DB: Backfilling %d identifier hashes...\n", missingCount)
		rows, err := d.conn.Query("SELECT id, size, modified FROM files WHERE identifier_hash IS NULL OR identifier_hash = ''")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var size, mod int64
				if err := rows.Scan(&id, &size, &mod); err == nil {
					input := fmt.Sprintf("%d_%d", size, mod)
					hash := sha256.Sum256([]byte(input))
					hashStr := strings.ToUpper(hex.EncodeToString(hash[:]))
					_, _ = d.conn.Exec("UPDATE files SET identifier_hash = ? WHERE id = ?", hashStr, id)
				}
			}
		}
	}

	return nil
}

func ensureColumn(db *sql.DB, tableName, columnName, columnDefinition string) {
	var exists bool
	query := fmt.Sprintf("SELECT EXISTS (SELECT 1 FROM pragma_table_info('%s') WHERE name='%s')", tableName, columnName)
	err := db.QueryRow(query).Scan(&exists)
	if err != nil {
		return
	}
	if !exists {
		alterQuery := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDefinition)
		_, _ = db.Exec(alterQuery)
	}
}

// ensureVectorTables creates the vec virtual table, its shadow store, and
// vector_storage used for persisted similarity indexes.
func (d *Database) ensureVectorTables() error {
	// Virtual table for MATCH-based similarity (dataset_id + doc_id + match_score).
	if _, err := d.conn.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS file_embeddings USING vec(doc_id)`); err != nil {
		return fmt.Errorf("create file_embeddings vtab: %w", err)
	}

	// Shadow table holds the actual embedding BLOBs (one row per frame).
	// Column layout matches viant/sqlite-vec's expected shadow schema.
	if _, err := d.conn.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	dataset_id TEXT NOT NULL,
	id         TEXT NOT NULL,
	content    TEXT,
	meta       TEXT,
	embedding  BLOB,
	PRIMARY KEY(dataset_id, id)
)`, embeddingsShadowTable)); err != nil {
		return fmt.Errorf("create embeddings shadow table: %w", err)
	}

	if _, err := d.conn.Exec(`
CREATE TABLE IF NOT EXISTS vector_storage (
	shadow_table_name TEXT NOT NULL,
	dataset_id        TEXT NOT NULL DEFAULT '',
	"index"           BLOB,
	PRIMARY KEY (shadow_table_name, dataset_id)
)`); err != nil {
		return fmt.Errorf("create vector_storage: %w", err)
	}

	return nil
}

// migrateLegacyEmbeddings moves neural_v1 BLOBs from the files table into the
// sqlite-vec shadow store, then clears the legacy column.
func (d *Database) migrateLegacyEmbeddings() error {
	rows, err := d.conn.Query("SELECT id, neural_v1 FROM files WHERE neural_v1 IS NOT NULL AND length(neural_v1) > 0")
	if err != nil {
		// Column may not exist on brand-new schemas that somehow skipped ensureColumn.
		if strings.Contains(err.Error(), "no such column") {
			return nil
		}
		return err
	}
	defer rows.Close()

	type legacy struct {
		id   int64
		blob []byte
	}
	var pending []legacy
	for rows.Next() {
		var item legacy
		if err := rows.Scan(&item.id, &item.blob); err != nil {
			return err
		}
		if len(item.blob) == 0 {
			continue
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	fmt.Printf("DB: Migrating %d legacy neural_v1 embeddings into sqlite-vec...\n", len(pending))
	for _, item := range pending {
		if err := d.storeNeuralEmbeddingsByID(item.id, item.blob); err != nil {
			return fmt.Errorf("migrate file id %d: %w", item.id, err)
		}
		if _, err := d.conn.Exec("UPDATE files SET neural_v1 = NULL WHERE id = ?", item.id); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) fileIDByPath(path string) (int64, error) {
	var id int64
	err := d.conn.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func datasetIDForFile(fileID int64) string {
	return strconv.FormatInt(fileID, 10)
}

// storeNeuralEmbeddingsByID unpacks a PackEmbeddings blob and writes each
// frame vector into the sqlite-vec shadow table for the given file.
func (d *Database) storeNeuralEmbeddingsByID(fileID int64, packed []byte) error {
	vecs := neural.UnpackEmbeddings(packed)
	if len(vecs) == 0 {
		return nil
	}
	datasetID := datasetIDForFile(fileID)

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE dataset_id = ?", embeddingsShadowTable), datasetID); err != nil {
		return err
	}

	stmt, err := tx.Prepare(fmt.Sprintf(
		"INSERT INTO %s(dataset_id, id, content, meta, embedding) VALUES (?, ?, ?, ?, ?)",
		embeddingsShadowTable,
	))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, v := range vecs {
		emb, err := vector.EncodeEmbedding(v)
		if err != nil {
			return err
		}
		docID := fmt.Sprintf("%08d", i)
		if _, err := stmt.Exec(datasetID, docID, "", "{}", emb); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// deleteNeuralEmbeddingsByID removes all frame embeddings for a file.
func (d *Database) deleteNeuralEmbeddingsByID(fileID int64) error {
	datasetID := datasetIDForFile(fileID)
	_, err := d.conn.Exec(fmt.Sprintf("DELETE FROM %s WHERE dataset_id = ?", embeddingsShadowTable), datasetID)
	return err
}

func (d *Database) loadNeuralEmbeddingsByID(fileID int64) ([][]float32, error) {
	datasetID := datasetIDForFile(fileID)
	rows, err := d.conn.Query(
		fmt.Sprintf("SELECT embedding FROM %s WHERE dataset_id = ? ORDER BY id", embeddingsShadowTable),
		datasetID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result [][]float32
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		v, err := vector.DecodeEmbedding(blob)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// loadNeuralEmbeddingsByFileCondition loads embeddings for all files matching
// the given SQL WHERE condition on the files table. It JOINs the embeddings
// shadow table directly instead of building an IN (?,?,...) clause, so there
// is no bound-parameter count limit regardless of how many files match.
//
// cond is appended after "WHERE", e.g. "path LIKE ? OR path LIKE ?".
// args are the corresponding bind values.
func (d *Database) loadNeuralEmbeddingsByFileCondition(cond string, args []any) (map[int64][][]float32, error) {
	out := make(map[int64][][]float32)
	query := fmt.Sprintf(`
SELECT e.dataset_id, e.embedding
FROM %s e
INNER JOIN files f ON e.dataset_id = CAST(f.id AS TEXT)
WHERE %s
ORDER BY e.dataset_id, e.id`,
		embeddingsShadowTable, cond,
	)
	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var datasetID string
		var blob []byte
		if err := rows.Scan(&datasetID, &blob); err != nil {
			return nil, err
		}
		fileID, err := strconv.ParseInt(datasetID, 10, 64)
		if err != nil {
			continue
		}
		v, err := vector.DecodeEmbedding(blob)
		if err != nil {
			return nil, err
		}
		out[fileID] = append(out[fileID], v)
	}
	return out, rows.Err()
}

func (d *Database) GetNeuralEmbeddingsByPath(path string) ([][]float32, error) {
	id, err := d.fileIDByPath(path)
	if err != nil {
		return nil, err
	}
	return d.GetNeuralEmbeddingsByID(id)
}

func (d *Database) GetNeuralEmbeddingsByID(fileID int64) ([][]float32, error) {
	return d.loadNeuralEmbeddingsByID(fileID)
}

func (d *Database) HasNeuralEmbeddingsByID(fileID int64) (bool, error) {
	var exists int
	datasetID := datasetIDForFile(fileID)
	err := d.conn.QueryRow(
		fmt.Sprintf("SELECT 1 FROM %s WHERE dataset_id = ? LIMIT 1", embeddingsShadowTable),
		datasetID,
	).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *Database) GetMeta(key string) (string, error) {
	var val string
	err := d.conn.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (d *Database) SetMeta(key, value string) error {
	_, err := d.conn.Exec("INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)", key, value)
	return err
}

func (d *Database) GetInstanceID() string {
	id, err := d.GetMeta("instance_id")
	if err == nil && id != "" {
		return id
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	id = fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	_ = d.SetMeta("instance_id", id)
	return id
}

func packHashes(hashes []uint64) []byte {
	buf := make([]byte, 8*len(hashes))
	for i, h := range hashes {
		binary.LittleEndian.PutUint64(buf[i*8:], h)
	}
	return buf
}

func unpackHashes(buf []byte) []uint64 {
	if len(buf)%8 != 0 {
		return nil
	}
	hashes := make([]uint64, len(buf)/8)
	for i := range hashes {
		hashes[i] = binary.LittleEndian.Uint64(buf[i*8:])
	}
	return hashes
}

func (d *Database) withRetry(fn func() error) error {
	var err error
	for i := range 5 {
		err = fn()
		if err == nil {
			return nil
		}
		if strings.Contains(err.Error(), "database is locked") {
			time.Sleep(time.Duration(10*(i+1)) * time.Millisecond)
			continue
		}
		return err
	}
	return err
}

func (d *Database) UpsertFile(path string, size int64, modified int64, duration float64, width int, height int, hashes []uint64, codec string, bitrate int64, fps float64, warnings []string) error {
	return d.upsertFile(path, size, modified, duration, width, height, hashes, codec, bitrate, fps, warnings, nil)
}

// UpsertFileWithNeural is like UpsertFile but also stores neural embeddings
// in the sqlite-vec shadow table (not in files.neural_v1).
func (d *Database) UpsertFileWithNeural(path string, size int64, modified int64, duration float64, width int, height int, hashes []uint64, codec string, bitrate int64, fps float64, warnings []string, packedNeural []byte) error {
	return d.upsertFile(path, size, modified, duration, width, height, hashes, codec, bitrate, fps, warnings, packedNeural)
}

// UpdateNeuralEmbeddings stores neural embeddings for an existing file record
// without touching any other fields. Used when a scan enriches an already-indexed file.
func (d *Database) UpdateNeuralEmbeddings(path string, packedNeural []byte) error {
	return d.withRetry(func() error {
		fileID, err := d.fileIDByPath(path)
		if err != nil {
			return err
		}
		return d.storeNeuralEmbeddingsByID(fileID, packedNeural)
	})
}

func (d *Database) upsertFile(path string, size int64, modified int64, duration float64, width int, height int, hashes []uint64, codec string, bitrate int64, fps float64, warnings []string, packedNeural []byte) error {
	packed := packHashes(hashes)

	var warningsJSON string
	if len(warnings) > 0 {
		b, _ := json.Marshal(warnings)
		warningsJSON = string(b)
	}

	// Metadata only — embeddings live in the sqlite-vec shadow table.
	query := `
	INSERT INTO files (path, size, modified, duration, width, height, phashes, codec, bitrate, fps, warnings, identifier_hash)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(path) DO UPDATE SET
		size = excluded.size,
		modified = excluded.modified,
		duration = excluded.duration,
		width = excluded.width,
		height = excluded.height,
		phashes = excluded.phashes,
		codec = excluded.codec,
		bitrate = excluded.bitrate,
		fps = excluded.fps,
		warnings = excluded.warnings;
	`
	input := fmt.Sprintf("%d_%d", size, modified)
	hash := sha256.Sum256([]byte(input))
	hashStr := strings.ToUpper(hex.EncodeToString(hash[:]))

	return d.withRetry(func() error {
		if _, err := d.conn.Exec(query, path, size, modified, duration, width, height, packed, codec, bitrate, fps, warningsJSON, hashStr); err != nil {
			return err
		}
		if len(packedNeural) == 0 {
			return nil
		}
		fileID, err := d.fileIDByPath(path)
		if err != nil {
			return err
		}
		return d.storeNeuralEmbeddingsByID(fileID, packedNeural)
	})
}

// scanFileRecord reads one metadata row (no neural_v1 column).
func scanFileRecord(scan func(...any) error) (FileRecord, error) {
	var r FileRecord
	var packed []byte
	var warningsJSON string
	if err := scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &packed, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash); err != nil {
		return FileRecord{}, err
	}
	r.PHashV2s = unpackHashes(packed)
	if warningsJSON != "" {
		_ = json.Unmarshal([]byte(warningsJSON), &r.Warnings)
	}
	return r, nil
}

const fileSelectCols = "id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '')"

// GetAllFiles returns lightweight file metadata only (no embeddings).
// Per the isolation architecture, embeddings are loaded on demand during
// comparison / enrichment paths.
func (d *Database) GetAllFiles() ([]FileRecord, error) {
	rows, err := d.conn.Query("SELECT " + fileSelectCols + " FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		r, err := scanFileRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (d *Database) GetFileByPath(path string) (*FileRecord, error) {
	row := d.conn.QueryRow("SELECT "+fileSelectCols+" FROM files WHERE path = ?", path)
	r, err := scanFileRecord(row.Scan)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	embeddings, err := d.loadNeuralEmbeddingsByID(r.ID)
	if err != nil {
		return nil, err
	}
	r.NeuralEmbeddings = embeddings
	return &r, nil
}

func (d *Database) GetFilesByContent(size, modified int64) ([]FileRecord, error) {
	rows, err := d.conn.Query("SELECT "+fileSelectCols+" FROM files WHERE size = ? AND modified = ?", size, modified)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		r, err := scanFileRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (d *Database) UpdatePath(oldPath, newPath string) error {
	_, err := d.conn.Exec("UPDATE files SET path = ? WHERE path = ?", newPath, oldPath)
	return err
}

func (d *Database) AddIgnoredGroup(label string, identifierHashes []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec("INSERT INTO ignored_groups (label) VALUES (?)", label)
	if err != nil {
		return err
	}
	groupID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	for _, hash := range identifierHashes {
		_, err = tx.Exec("INSERT OR IGNORE INTO ignored_group_items (group_id, identifier_hash) VALUES (?, ?)", groupID, hash)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *Database) GetIgnoredGroups() ([]IgnoredGroup, error) {
	rows, err := d.conn.Query("SELECT id, label FROM ignored_groups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []IgnoredGroup
	for rows.Next() {
		var g IgnoredGroup
		if err := rows.Scan(&g.ID, &g.Label); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	for i := range groups {
		groups[i].IdentifierHashes = make([]string, 0)
		groups[i].ResolvedPaths = make([]string, 0)

		query := `
			SELECT igi.identifier_hash, COALESCE(f.path, '')
			FROM ignored_group_items igi
			LEFT JOIN files f ON igi.identifier_hash = f.identifier_hash
			WHERE igi.group_id = ?
		`

		itemRows, err := d.conn.Query(query, groups[i].ID)
		if err != nil {
			return nil, err
		}
		for itemRows.Next() {
			var hash, path string
			if err := itemRows.Scan(&hash, &path); err != nil {
				itemRows.Close()
				return nil, err
			}
			groups[i].IdentifierHashes = append(groups[i].IdentifierHashes, hash)
			groups[i].ResolvedPaths = append(groups[i].ResolvedPaths, path)
		}
		itemRows.Close()
	}

	return groups, nil
}

func (d *Database) DeleteFile(path string) error {
	return d.withRetry(func() error {
		fileID, err := d.fileIDByPath(path)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		if err := d.deleteNeuralEmbeddingsByID(fileID); err != nil {
			return err
		}
		_, err = d.conn.Exec("DELETE FROM files WHERE id = ?", fileID)
		return err
	})
}

func (d *Database) DeleteIgnoredGroup(id int64) error {
	fmt.Printf("DB: Deleting ignored group %d\n", id)
	_, err := d.conn.Exec("DELETE FROM ignored_groups WHERE id = ?", id)
	return err
}

func (d *Database) DeleteAllIgnoredGroups() error {
	_, err := d.conn.Exec("DELETE FROM ignored_groups")
	if err != nil {
		return err
	}
	_, err = d.conn.Exec("DELETE FROM ignored_group_items")
	return err
}

func (d *Database) RenameFile(oldPath, newPath string) error {
	info, err := os.Stat(newPath)
	if err != nil {
		_, err = d.conn.Exec("UPDATE files SET path = ? WHERE path = ?", newPath, oldPath)
		return err
	}
	_, err = d.conn.Exec("UPDATE files SET path = ?, modified = ? WHERE path = ?", newPath, info.ModTime().Unix(), oldPath)
	return err
}

func (d *Database) ClearAllWarnings() (int64, error) {
	res, err := d.conn.Exec("UPDATE files SET warnings = '' WHERE warnings != '' AND warnings != '[]'")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *Database) DeleteFiles(paths []string) (int64, error) {
	if len(paths) == 0 {
		return 0, nil
	}

	var deleted int64
	err := d.withRetry(func() error {
		tx, err := d.conn.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		deleted = 0
		for _, p := range paths {
			var fileID int64
			err := tx.QueryRow("SELECT id FROM files WHERE path = ?", p).Scan(&fileID)
			if err != nil {
				continue
			}
			if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE dataset_id = ?", embeddingsShadowTable), datasetIDForFile(fileID)); err != nil {
				return err
			}
			res, err := tx.Exec("DELETE FROM files WHERE id = ?", fileID)
			if err != nil {
				continue
			}
			rows, _ := res.RowsAffected()
			deleted += rows
		}
		return tx.Commit()
	})
	return deleted, err
}

func (d *Database) GetFilesByPaths(paths []string) ([]FileRecord, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(paths))
	args := make([]any, len(paths))
	for i, p := range paths {
		placeholders[i] = "?"
		args[i] = p
	}
	query := fmt.Sprintf("SELECT %s FROM files WHERE path IN (%s)", fileSelectCols, strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FileRecord
	for rows.Next() {
		r, err := scanFileRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetFilesByPrefixesMetadata returns file metadata under the given path prefixes
// without loading neural embeddings (used during scan discovery).
func (d *Database) GetFilesByPrefixesMetadata(prefixes []string) ([]FileRecord, error) {
	return d.queryFilesByPrefixes(prefixes)
}

// GetFilesByPrefixes returns files under the given path prefixes and attaches
// neural embeddings (used by the comparison phase).
func (d *Database) GetFilesByPrefixes(prefixes []string) ([]FileRecord, error) {
	records, cond, args, err := d.queryFilesByPrefixesWithCondition(prefixes)
	if err != nil || len(records) == 0 {
		return records, err
	}

	byID, err := d.loadNeuralEmbeddingsByFileCondition(cond, args)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if embs, ok := byID[records[i].ID]; ok {
			records[i].NeuralEmbeddings = embs
		}
	}
	return records, nil
}

func (d *Database) queryFilesByPrefixes(prefixes []string) ([]FileRecord, error) {
	records, _, _, err := d.queryFilesByPrefixesWithCondition(prefixes)
	return records, err
}

func (d *Database) queryFilesByPrefixesWithCondition(prefixes []string) ([]FileRecord, string, []any, error) {
	if len(prefixes) == 0 {
		return nil, "", nil, nil
	}

	var conditions []string
	var args []any
	for _, p := range prefixes {
		conditions = append(conditions, "f.path LIKE ?")
		if !strings.HasSuffix(p, string(os.PathSeparator)) {
			p += string(os.PathSeparator)
		}
		args = append(args, p+"%")
	}
	cond := strings.Join(conditions, " OR ")

	query := fmt.Sprintf("SELECT %s FROM files f WHERE %s", fileSelectCols, cond)
	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, "", nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		r, err := scanFileRecord(rows.Scan)
		if err != nil {
			return nil, "", nil, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", nil, err
	}
	return records, cond, args, nil
}

func (d *Database) GetSuspiciousFiles() ([]FileRecord, error) {
	rows, err := d.conn.Query("SELECT " + fileSelectCols + " FROM files WHERE warnings != '' AND warnings != '[]'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		r, err := scanFileRecord(rows.Scan)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (d *Database) GetStats() (map[string]any, error) {
	var totalFiles int
	var totalSize int64
	var totalDuration float64
	var suspiciousCount int

	err := d.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(size), 0), COALESCE(SUM(duration), 0.0) FROM files").Scan(&totalFiles, &totalSize, &totalDuration)
	if err != nil {
		return nil, err
	}

	err = d.conn.QueryRow("SELECT COUNT(*) FROM files WHERE warnings != '' AND warnings != '[]'").Scan(&suspiciousCount)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"total_files":      totalFiles,
		"total_size":       totalSize,
		"total_duration":   totalDuration,
		"suspicious_count": suspiciousCount,
	}, nil
}

func (d *Database) Reset() error {
	if _, err := d.conn.Exec(fmt.Sprintf("DELETE FROM %s", embeddingsShadowTable)); err != nil {
		return err
	}
	if _, err := d.conn.Exec(`DELETE FROM vector_storage`); err != nil {
		return err
	}
	_, err := d.conn.Exec("DELETE FROM files")
	return err
}

func (d *Database) Cleanup() (int, error) {
	records, err := d.GetAllFiles()
	if err != nil {
		return 0, err
	}

	removed := 0
	hashExists := make(map[string]bool)
	for _, r := range records {
		if _, err := os.Stat(r.Path); os.IsNotExist(err) {
			if err := d.DeleteFile(r.Path); err == nil {
				removed++
			}
		} else {
			hashExists[r.GetIdentifierHash()] = true
		}
	}

	rows, err := d.conn.Query("SELECT group_id, identifier_hash FROM ignored_group_items")
	if err == nil {
		var toDelete []struct {
			groupID int64
			hash    string
		}
		for rows.Next() {
			var gid int64
			var h string
			if err := rows.Scan(&gid, &h); err == nil {
				if !hashExists[strings.ToUpper(h)] {
					toDelete = append(toDelete, struct {
						groupID int64
						hash    string
					}{gid, h})
				}
			}
		}
		rows.Close()

		for _, item := range toDelete {
			_, _ = d.conn.Exec("DELETE FROM ignored_group_items WHERE group_id = ? AND identifier_hash = ?", item.groupID, item.hash)
		}
	}

	_, _ = d.conn.Exec(`
		DELETE FROM ignored_groups
		WHERE id NOT IN (SELECT group_id FROM ignored_group_items)
	`)

	return removed, nil
}
