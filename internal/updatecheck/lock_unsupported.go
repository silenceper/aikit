//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package updatecheck

import (
	"context"
	"fmt"
)

func acquireCacheLock(context.Context, string) (func(), error) {
	return nil, fmt.Errorf("update cache locking is unsupported on this platform")
}
