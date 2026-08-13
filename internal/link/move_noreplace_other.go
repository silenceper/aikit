//go:build !linux && !darwin && !windows

package link

import "fmt"

func moveNoReplace(from, to string) error {
	return fmt.Errorf("atomic no-replace move is unsupported on this platform")
}
