//go:build !linux && !darwin && !windows

package library

import "fmt"

func moveNoReplace(string, string) error {
	return fmt.Errorf("atomic no-replace move is unsupported on this platform")
}
