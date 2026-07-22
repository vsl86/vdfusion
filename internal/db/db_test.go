package db

import (
	"math"
	"path/filepath"
	"testing"

	"vdfusion/internal/neural"

	"github.com/viant/sqlite-vec/vector"
)

func floatEq(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-5
}

func vectorsEqual(t *testing.T, got, want [][]float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d len(want)=%d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("vec[%d] len(got)=%d len(want)=%d", i, len(got[i]), len(want[i]))
		}
		for j := range want[i] {
			if !floatEq(got[i][j], want[i][j]) {
				t.Fatalf("vec[%d][%d]=%v want %v", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func openTestDB(t *testing.T) *Database {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestNewRegistersVecAndShadow(t *testing.T) {
	d := openTestDB(t)

	// Virtual table must exist.
	var name string
	err := d.conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='file_embeddings'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("file_embeddings vtab missing: %v", err)
	}

	// Shadow table must exist.
	err = d.conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, embeddingsShadowTable,
	).Scan(&name)
	if err != nil {
		t.Fatalf("shadow table missing: %v", err)
	}

	// vector_storage must exist.
	err = d.conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='vector_storage'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("vector_storage missing: %v", err)
	}
}

func TestStoreAndLoadNeuralEmbeddings(t *testing.T) {
	d := openTestDB(t)

	vecs := [][]float32{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0.5, 0.5, 0, 0},
	}
	packed := neural.PackEmbeddings(vecs)

	err := d.UpsertFileWithNeural(
		"/videos/a.mp4", 100, 1000, 10.0, 1920, 1080,
		[]uint64{1, 2, 3}, "h264", 1_000_000, 30.0, nil, packed,
	)
	if err != nil {
		t.Fatalf("UpsertFileWithNeural: %v", err)
	}

	// GetFileByPath loads embeddings on demand.
	rec, err := d.GetFileByPath("/videos/a.mp4")
	if err != nil {
		t.Fatalf("GetFileByPath: %v", err)
	}
	if rec == nil {
		t.Fatal("expected record")
	}
	vectorsEqual(t, rec.NeuralEmbeddings, vecs)

	// GetNeuralEmbeddingsByPath works independently.
	got, err := d.GetNeuralEmbeddingsByPath("/videos/a.mp4")
	if err != nil {
		t.Fatalf("GetNeuralEmbeddingsByPath: %v", err)
	}
	vectorsEqual(t, got, vecs)

	// HasNeuralEmbeddingsByID
	ok, err := d.HasNeuralEmbeddingsByID(rec.ID)
	if err != nil {
		t.Fatalf("HasNeuralEmbeddingsByID: %v", err)
	}
	if !ok {
		t.Fatal("expected embeddings to exist")
	}

	// Embeddings must live in shadow table, not files.neural_v1.
	var neuralBlob []byte
	err = d.conn.QueryRow("SELECT neural_v1 FROM files WHERE id = ?", rec.ID).Scan(&neuralBlob)
	if err != nil {
		t.Fatalf("select neural_v1: %v", err)
	}
	if len(neuralBlob) > 0 {
		t.Fatalf("neural_v1 should be empty after sqlite-vec store, got %d bytes", len(neuralBlob))
	}

	var frameCount int
	err = d.conn.QueryRow(
		"SELECT COUNT(*) FROM "+embeddingsShadowTable+" WHERE dataset_id = ?",
		datasetIDForFile(rec.ID),
	).Scan(&frameCount)
	if err != nil {
		t.Fatalf("count shadow rows: %v", err)
	}
	if frameCount != len(vecs) {
		t.Fatalf("shadow frame count=%d want %d", frameCount, len(vecs))
	}
}

func TestGetAllFilesDoesNotLoadEmbeddings(t *testing.T) {
	d := openTestDB(t)

	vecs := [][]float32{{1, 0}, {0, 1}}
	packed := neural.PackEmbeddings(vecs)
	if err := d.UpsertFileWithNeural("/v/a.mp4", 1, 1, 1, 1, 1, nil, "h264", 0, 0, nil, packed); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	all, err := d.GetAllFiles()
	if err != nil {
		t.Fatalf("GetAllFiles: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len=%d", len(all))
	}
	if len(all[0].NeuralEmbeddings) != 0 {
		t.Fatalf("GetAllFiles should not attach embeddings, got %d frames", len(all[0].NeuralEmbeddings))
	}
}

func TestGetFilesByPrefixesAttachesEmbeddings(t *testing.T) {
	d := openTestDB(t)

	v1 := [][]float32{{1, 0, 0}}
	v2 := [][]float32{{0, 1, 0}}
	if err := d.UpsertFileWithNeural("/media/a.mp4", 1, 1, 1, 1, 1, nil, "h264", 0, 0, nil, neural.PackEmbeddings(v1)); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertFileWithNeural("/media/b.mp4", 2, 2, 1, 1, 1, nil, "h264", 0, 0, nil, neural.PackEmbeddings(v2)); err != nil {
		t.Fatal(err)
	}
	// Outside prefix
	if err := d.UpsertFileWithNeural("/other/c.mp4", 3, 3, 1, 1, 1, nil, "h264", 0, 0, nil, neural.PackEmbeddings(v1)); err != nil {
		t.Fatal(err)
	}

	recs, err := d.GetFilesByPrefixes([]string{"/media"})
	if err != nil {
		t.Fatalf("GetFilesByPrefixes: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("len=%d want 2", len(recs))
	}
	for _, r := range recs {
		if len(r.NeuralEmbeddings) == 0 {
			t.Fatalf("expected embeddings for %s", r.Path)
		}
	}
}

func TestUpdateNeuralEmbeddings(t *testing.T) {
	d := openTestDB(t)

	if err := d.UpsertFile("/x.mp4", 1, 1, 1, 1, 1, nil, "h264", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	rec, err := d.GetFileByPath("/x.mp4")
	if err != nil || rec == nil {
		t.Fatalf("GetFileByPath: %v rec=%v", err, rec)
	}
	if len(rec.NeuralEmbeddings) != 0 {
		t.Fatal("expected no embeddings yet")
	}

	vecs := [][]float32{{0.1, 0.2, 0.3}}
	if err := d.UpdateNeuralEmbeddings("/x.mp4", neural.PackEmbeddings(vecs)); err != nil {
		t.Fatalf("UpdateNeuralEmbeddings: %v", err)
	}
	rec, err = d.GetFileByPath("/x.mp4")
	if err != nil {
		t.Fatal(err)
	}
	vectorsEqual(t, rec.NeuralEmbeddings, vecs)
}

func TestDeleteFileRemovesEmbeddings(t *testing.T) {
	d := openTestDB(t)

	vecs := [][]float32{{1, 0}}
	if err := d.UpsertFileWithNeural("/del.mp4", 1, 1, 1, 1, 1, nil, "h264", 0, 0, nil, neural.PackEmbeddings(vecs)); err != nil {
		t.Fatal(err)
	}
	rec, _ := d.GetFileByPath("/del.mp4")
	if rec == nil {
		t.Fatal("missing record")
	}
	id := rec.ID

	if err := d.DeleteFile("/del.mp4"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	var count int
	_ = d.conn.QueryRow(
		"SELECT COUNT(*) FROM "+embeddingsShadowTable+" WHERE dataset_id = ?",
		datasetIDForFile(id),
	).Scan(&count)
	if count != 0 {
		t.Fatalf("orphan embeddings remain: %d", count)
	}
}

func TestMigrateLegacyNeuralV1(t *testing.T) {
	// Build a DB with legacy neural_v1 blobs by writing them directly, then
	// re-open via New() so migrateLegacyEmbeddings runs.
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	d1, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	vecs := [][]float32{
		{1, 0, 0, 0},
		{0, 0, 1, 0},
	}
	packed := neural.PackEmbeddings(vecs)

	// Insert a file row with neural_v1 set the old way (bypass store helpers).
	_, err = d1.conn.Exec(`
		INSERT INTO files (path, size, modified, duration, width, height, phashes, codec, bitrate, fps, warnings, identifier_hash, neural_v1)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"/legacy.mp4", 10, 20, 1.0, 64, 64, nil, "h264", 0, 0.0, "", "ABC", packed,
	)
	if err != nil {
		t.Fatalf("insert legacy: %v", err)
	}
	// Clear any shadow rows that shouldn't exist yet for this path.
	_, _ = d1.conn.Exec("DELETE FROM " + embeddingsShadowTable)
	_ = d1.Close()

	// Re-open triggers migrateLegacyEmbeddings.
	d2, err := New(path)
	if err != nil {
		t.Fatalf("reopen New: %v", err)
	}
	defer d2.Close()

	rec, err := d2.GetFileByPath("/legacy.mp4")
	if err != nil {
		t.Fatalf("GetFileByPath: %v", err)
	}
	if rec == nil {
		t.Fatal("missing record after migration")
	}
	vectorsEqual(t, rec.NeuralEmbeddings, vecs)

	// Legacy column cleared.
	var blob []byte
	if err := d2.conn.QueryRow("SELECT neural_v1 FROM files WHERE path = ?", "/legacy.mp4").Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if len(blob) > 0 {
		t.Fatalf("neural_v1 not cleared after migration, len=%d", len(blob))
	}
}

func TestReplaceEmbeddingsOnUpdate(t *testing.T) {
	d := openTestDB(t)

	old := [][]float32{{1, 0}, {0, 1}}
	if err := d.UpsertFileWithNeural("/r.mp4", 1, 1, 1, 1, 1, nil, "h264", 0, 0, nil, neural.PackEmbeddings(old)); err != nil {
		t.Fatal(err)
	}

	// Replace with a single different vector.
	newer := [][]float32{{0, 0, 1}}
	if err := d.UpdateNeuralEmbeddings("/r.mp4", neural.PackEmbeddings(newer)); err != nil {
		t.Fatal(err)
	}

	got, err := d.GetNeuralEmbeddingsByPath("/r.mp4")
	if err != nil {
		t.Fatal(err)
	}
	vectorsEqual(t, got, newer)
}

func TestEncodeDecodeRoundTripMatchesVectorPkg(t *testing.T) {
	// Sanity: storage uses vector.EncodeEmbedding, which must round-trip.
	src := []float32{0.1, -0.2, 0.3, 0.4}
	blob, err := vector.EncodeEmbedding(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := vector.DecodeEmbedding(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(src) {
		t.Fatalf("len")
	}
	for i := range src {
		if !floatEq(got[i], src[i]) {
			t.Fatalf("mismatch at %d", i)
		}
	}
}

func TestResetClearsEmbeddings(t *testing.T) {
	d := openTestDB(t)
	if err := d.UpsertFileWithNeural("/z.mp4", 1, 1, 1, 1, 1, nil, "h264", 0, 0, nil, neural.PackEmbeddings([][]float32{{1}})); err != nil {
		t.Fatal(err)
	}
	if err := d.Reset(); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = d.conn.QueryRow("SELECT COUNT(*) FROM " + embeddingsShadowTable).Scan(&n)
	if n != 0 {
		t.Fatalf("shadow rows remain after Reset: %d", n)
	}
	_ = d.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&n)
	if n != 0 {
		t.Fatalf("files remain after Reset: %d", n)
	}
}
