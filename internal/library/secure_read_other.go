//go:build !darwin && !linux && !windows

package library

import (
	"fmt"
	"os"
)

func secureReadRegular(string, string, os.FileInfo) ([]byte, error) {
	return nil, fmt.Errorf("secure source reads are unsupported on this platform")
}
