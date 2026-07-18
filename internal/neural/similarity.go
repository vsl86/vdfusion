package neural

import (
	"encoding/binary"
	"math"
)

// CosineSimilarity returns the cosine similarity between two L2-normalised
// vectors. Because the backend returns L2-normalised embeddings, this is
// equivalent to a dot product and always in [-1, 1].
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	// Clamp to [-1, 1] to guard against floating-point drift
	return math.Max(-1.0, math.Min(1.0, dot))
}

// AverageCosineSimilarity computes the mean cosine similarity across
// corresponding frame embeddings from two files. Returns (0, false) when
// either slice is empty.
func AverageCosineSimilarity(as, bs [][]float32) (float64, bool) {
	n := min(len(as), len(bs))
	if n == 0 {
		return 0, false
	}
	var total float64
	for i := range n {
		total += CosineSimilarity(as[i], bs[i])
	}
	return total / float64(n), true
}

// PackEmbeddings serialises a slice of float32 vectors to a flat byte blob
// (little-endian). Layout: [dim0_f0, dim1_f0, …, dim0_f1, …]
// A leading uint32 encodes the number of vectors, and a second uint32 encodes
// the dimension of each vector (so the blob is self-describing).
func PackEmbeddings(vecs [][]float32) []byte {
	if len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	buf := make([]byte, 8+len(vecs)*dim*4)
	binary.LittleEndian.PutUint32(buf[0:], uint32(len(vecs)))
	binary.LittleEndian.PutUint32(buf[4:], uint32(dim))
	offset := 8
	for _, vec := range vecs {
		for _, f := range vec {
			bits := math.Float32bits(f)
			binary.LittleEndian.PutUint32(buf[offset:], bits)
			offset += 4
		}
	}
	return buf
}

// UnpackEmbeddings is the inverse of PackEmbeddings. Returns nil on any error.
func UnpackEmbeddings(blob []byte) [][]float32 {
	if len(blob) < 8 {
		return nil
	}
	count := int(binary.LittleEndian.Uint32(blob[0:]))
	dim := int(binary.LittleEndian.Uint32(blob[4:]))
	if count == 0 || dim == 0 {
		return nil
	}
	expected := 8 + count*dim*4
	if len(blob) < expected {
		return nil
	}
	vecs := make([][]float32, count)
	offset := 8
	for i := range count {
		vecs[i] = make([]float32, dim)
		for j := range dim {
			bits := binary.LittleEndian.Uint32(blob[offset:])
			vecs[i][j] = math.Float32frombits(bits)
			offset += 4
		}
	}
	return vecs
}
