//nolint:revive
package util

import "fmt"

// HasDuplicateHeights checks if the provided slice of heights contains any duplicates.
// Returns (true, duplicateHeight) if a duplicate is found, (false, 0) otherwise.
// Uses a map for O(n) time complexity.
//
// This function is critical for preventing EOTS private key extraction attacks.
// Signing the same height twice with different messages allows an attacker to
// mathematically extract the private key through EOTS vulnerability.
func HasDuplicateHeights(heights []uint64) (bool, uint64) {
	seen := make(map[uint64]struct{}, len(heights))
	for _, height := range heights {
		if _, exists := seen[height]; exists {
			return true, height
		}
		seen[height] = struct{}{}
	}

	return false, 0
}

// ValidateNoDuplicateHeights returns an error if duplicate heights are found in the slice.
// This validation is essential for preventing EOTS key extraction via duplicate signing.
func ValidateNoDuplicateHeights(heights []uint64) error {
	if hasDup, dupHeight := HasDuplicateHeights(heights); hasDup {
		return fmt.Errorf("duplicate height detected: %d", dupHeight)
	}

	return nil
}
