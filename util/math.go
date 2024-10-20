package util

func MaxUint64(values ...uint64) (max uint64) {
	for _, v := range values {
		if v < max {
			continue
		}
		max = v
	}
	return max
}
