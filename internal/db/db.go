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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type FileRecord struct {
	ID             int64
	Path           string
	Size           int64
	Modified       int64
	Duration       float64
	Width          int
	Height         int
	PHashV2s       []uint64
	Codec          string
	Bitrate        int64
	FPS            float64
	Warnings       []string
	IdentifierHash string
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

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &Database{conn: db, path: path}

	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}
	_, err = db.Exec("PRAGMA busy_timeout=5000;")
	if err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
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

	d.conn.Exec("ALTER TABLE files ADD COLUMN warnings TEXT DEFAULT ''")
	d.conn.Exec("ALTER TABLE files ADD COLUMN width INTEGER DEFAULT 0")
	d.conn.Exec("ALTER TABLE files ADD COLUMN height INTEGER DEFAULT 0")
	d.conn.Exec("ALTER TABLE files ADD COLUMN identifier_hash TEXT")
	d.conn.Exec("CREATE INDEX IF NOT EXISTS idx_files_hash ON files(identifier_hash)")

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
					d.conn.Exec("UPDATE files SET identifier_hash = ? WHERE id = ?", hashStr, id)
				}
			}
		}
	}

	return nil
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
	packed := packHashes(hashes)

	var warningsJSON string
	if len(warnings) > 0 {
		b, _ := json.Marshal(warnings)
		warningsJSON = string(b)
	}

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
		_, err := d.conn.Exec(query, path, size, modified, duration, width, height, packed, codec, bitrate, fps, warningsJSON, hashStr)
		return err
	})
}

func (d *Database) GetAllFiles() ([]FileRecord, error) {
	rows, err := d.conn.Query("SELECT id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '') FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var r FileRecord
		var packed []byte
		var warningsJSON string
		if err := rows.Scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &packed, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash); err != nil {
			return nil, err
		}
		r.PHashV2s = unpackHashes(packed)
		if warningsJSON != "" {
			json.Unmarshal([]byte(warningsJSON), &r.Warnings)
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *Database) GetFileByPath(path string) (*FileRecord, error) {
	var r FileRecord
	var packed []byte
	var warningsJSON string
	err := d.conn.QueryRow("SELECT id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '') FROM files WHERE path = ?", path).
		Scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &packed, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.PHashV2s = unpackHashes(packed)
	if warningsJSON != "" {
		json.Unmarshal([]byte(warningsJSON), &r.Warnings)
	}
	return &r, nil
}

func (d *Database) GetFilesByContent(size, modified int64) ([]FileRecord, error) {
	rows, err := d.conn.Query("SELECT id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '') FROM files WHERE size = ? AND modified = ?", size, modified)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var r FileRecord
		var packed []byte
		var warningsJSON string
		if err := rows.Scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &packed, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash); err != nil {
			return nil, err
		}
		r.PHashV2s = unpackHashes(packed)
		if warningsJSON != "" {
			json.Unmarshal([]byte(warningsJSON), &r.Warnings)
		}
		records = append(records, r)
	}
	return records, nil
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
	rows.Close()

	for i := range groups {
		groups[i].IdentifierHashes = make([]string, 0)
		groups[i].ResolvedPaths = make([]string, 0)

		query := `
			SELECT igi.identifier_hash, COALESCE(f.path, '')
			FROM ignored_group_items igi
			LEFT JOIN files f ON strings.ToUpper(igi.identifier_hash) = strings.ToUpper(f.identifier_hash)
			WHERE igi.group_id = ?
		`
		query = `
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
	_, err := d.conn.Exec("DELETE FROM files WHERE path = ?", path)
	return err
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

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("DELETE FROM files WHERE path = ?")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var deleted int64
	for _, p := range paths {
		res, err := stmt.Exec(p)
		if err != nil {
			continue
		}
		rows, _ := res.RowsAffected()
		deleted += rows
	}

	return deleted, tx.Commit()
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
	query := fmt.Sprintf("SELECT id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '') FROM files WHERE path IN (%s)", strings.Join(placeholders, ","))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []FileRecord
	for rows.Next() {
		var r FileRecord
		var phashesBlob []byte
		var warningsJSON string
		if err := rows.Scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &phashesBlob, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash); err != nil {
			return nil, err
		}
		r.PHashV2s = unpackHashes(phashesBlob)
		if warningsJSON != "" {
			json.Unmarshal([]byte(warningsJSON), &r.Warnings)
		}
		results = append(results, r)
	}
	return results, nil
}

func (d *Database) GetFilesByPrefixes(prefixes []string) ([]FileRecord, error) {
	if len(prefixes) == 0 {
		return nil, nil
	}

	var conditions []string
	var args []any
	for _, p := range prefixes {
		conditions = append(conditions, "path LIKE ?")
		if !strings.HasSuffix(p, string(os.PathSeparator)) {
			p += string(os.PathSeparator)
		}
		args = append(args, p+"%")
	}

	query := fmt.Sprintf("SELECT id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '') FROM files WHERE %s", strings.Join(conditions, " OR "))

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var r FileRecord
		var packed []byte
		var warningsJSON string
		if err := rows.Scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &packed, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash); err != nil {
			return nil, err
		}
		r.PHashV2s = unpackHashes(packed)
		if warningsJSON != "" {
			json.Unmarshal([]byte(warningsJSON), &r.Warnings)
		}
		records = append(records, r)
	}
	return records, nil
}

func (d *Database) GetSuspiciousFiles() ([]FileRecord, error) {
	rows, err := d.conn.Query("SELECT id, path, size, modified, duration, width, height, phashes, codec, bitrate, fps, COALESCE(warnings,''), COALESCE(identifier_hash, '') FROM files WHERE warnings != '' AND warnings != '[]'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []FileRecord
	for rows.Next() {
		var r FileRecord
		var packed []byte
		var warningsJSON string
		if err := rows.Scan(&r.ID, &r.Path, &r.Size, &r.Modified, &r.Duration, &r.Width, &r.Height, &packed, &r.Codec, &r.Bitrate, &r.FPS, &warningsJSON, &r.IdentifierHash); err != nil {
			return nil, err
		}
		r.PHashV2s = unpackHashes(packed)
		if warningsJSON != "" {
			json.Unmarshal([]byte(warningsJSON), &r.Warnings)
		}
		records = append(records, r)
	}
	return records, nil
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
