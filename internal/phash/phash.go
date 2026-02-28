package phash

import (
	"math"
	"sort"
)

const (
	N = 32 // working size
	K = 8  // low-frequency block size (1..8,1..8)
)

var (
	cos   [N][N]float64
	alpha [N]float64
)

func init() {
	buildCos()
	buildAlpha()
}

func buildCos() {
	for k := range N {
		for i := range N {
			cos[k][i] = math.Cos(((2*float64(i) + 1) * float64(k) * math.Pi) / (2.0 * float64(N)))
		}
	}
}

func buildAlpha() {
	invN := 1.0 / float64(N)
	alpha[0] = math.Sqrt(invN)
	for k := 1; k < N; k++ {
		alpha[k] = math.Sqrt(2.0 * invN)
	}
}

// ComputeV2 computes the pHash v2 from a 32x32 grayscale buffer (1024 bytes).
func ComputeV2(gray []byte) uint64 {
	if len(gray) != N*N {
		panic("expected 32x32=1024 bytes")
	}

	input := normalizeHistogram(gray)
	temp := make([]float64, N*N)
	dct := make([]float64, N*N)

	// DCT rows
	for y := range N {
		yBase := y * N
		for u := range N {
			sum := 0.0
			for x := range N {
				sum += input[yBase+x] * cos[u][x]
			}
			temp[yBase+u] = alpha[u] * sum
		}
	}

	// DCT cols
	for u := range N {
		for v := range N {
			sum := 0.0
			for y := range N {
				sum += temp[y*N+u] * cos[v][y]
			}
			dct[v*N+u] = alpha[v] * sum
		}
	}

	// Standard Top-Left 8x8 AC (including 0 row/col, but median excludes DC component)
	ac := make([]float64, K*K)
	acForMedian := make([]float64, (K*K)-1)

	kIdx := 0
	mIdx := 0
	for v := range K {
		vBase := v * N
		for u := range K {
			val := dct[vBase+u]
			ac[kIdx] = val
			kIdx++
			// Skip DC component (0,0) for median calculation to prevent skewing
			if v != 0 || u != 0 {
				acForMedian[mIdx] = val
				mIdx++
			}
		}
	}

	median := median63(acForMedian)
	var hash uint64
	// Generate 64-bit hash using all 64 low frequency components
	for i := range ac {
		if ac[i] > median {
			hash |= 1 << i
		}
	}

	return hash
}

func normalizeHistogram(input []byte) []float64 {
	var min byte = 255
	var max byte = 0
	for _, b := range input {
		if b < min {
			min = b
		}
		if b > max {
			max = b
		}
	}

	output := make([]float64, len(input))
	if max == min {
		for i, b := range input {
			output[i] = float64(b)
		}
		return output
	}

	rangeVal := float64(max - min)
	scale := 255.0 / rangeVal
	for i, b := range input {
		output[i] = float64(b-min) * scale
	}
	return output
}

func median63(values []float64) float64 {
	sort.Float64s(values)
	return values[31] // odd length = 63, middle element is at index 31
}
