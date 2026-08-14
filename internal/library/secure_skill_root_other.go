//go:build !darwin && !linux && !windows

package library

import "fmt"

func openVerifiedSkillRoot(string, string, func(int) error) (VerifiedSkillRoot, error) {
	return nil, fmt.Errorf("verified skill-root reads are unsupported on this platform")
}
