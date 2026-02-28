package media

// VanDerCorput calculates the n-th element of the Van der Corput sequence in base 2.
// This sequence is used to generate stable timestamps that are well-distributed
// and have a prefix property: any set of N points is a subset of a larger set M.
// n should start at 1.
func VanDerCorput(n int) float64 {
	var q float64 = 0
	var bk float64 = 0.5
	for n > 0 {
		if n%2 == 1 {
			q += bk
		}
		bk /= 2
		n /= 2
	}
	// Small epsilon adjustment to avoid exact 0 or 1 boundaries if desired,
	// but here we just return the value.
	return q
}

// GetStableTimestamp returns a stable timestamp between 0 and duration.
// i is the index (0-based) of the thumbnail/hash.
func GetStableTimestamp(i int, duration float64) float64 {
	// We use i+1 because VanDerCorput(0) is 0.
	// VanDerCorput(1) = 0.5
	// VanDerCorput(2) = 0.25
	// VanDerCorput(3) = 0.75
	// ...
	return duration * VanDerCorput(i+1)
}

// NextPowerOfTwoMinusOne returns the smallest value 2^k - 1 that is >= n.
// These "magic" numbers (1, 3, 7, 15, 31...) result in perfectly even distributions.
func NextPowerOfTwoMinusOne(n int) int {
	if n <= 0 {
		return 0
	}
	p := 1
	for p < (n + 1) {
		p <<= 1
	}
	return p - 1
}
