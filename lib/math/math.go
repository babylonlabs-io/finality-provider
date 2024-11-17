package math

func MaxUint64(values ...uint64) uint64 {
	var maxVal uint64
	for _, v := range values {
		if v < maxVal {
			continue
		}
		maxVal = v
	}
	return maxVal
}
