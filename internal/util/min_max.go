package util

func MaxInt(vals ...int) int {
	max := vals[0]
	for _, i := range vals {
		if i > max {
			max = i
		}
	}

	return max
}

func MaxInt32(vals ...int32) int32 {
	max := vals[0]
	for _, i := range vals {
		if i > max {
			max = i
		}
	}

	return max
}

func MaxInt64(vals ...int64) int64 {
	max := vals[0]
	for _, i := range vals {
		if i > max {
			max = i
		}
	}

	return max
}

func MinInt(vals ...int) int {
	min := vals[0]
	for _, i := range vals {
		if i < min {
			min = i
		}
	}

	return min
}

func MinInt32(vals ...int32) int32 {
	min := vals[0]
	for _, i := range vals {
		if i < min {
			min = i
		}
	}

	return min
}

func MinInt64(vals ...int64) int64 {
	min := vals[0]
	for _, i := range vals {
		if i < min {
			min = i
		}
	}

	return min
}
