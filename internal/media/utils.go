package media

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
	return q
}

func GetStableTimestamp(i int, duration float64) float64 {
	return duration * VanDerCorput(i+1)
}

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
