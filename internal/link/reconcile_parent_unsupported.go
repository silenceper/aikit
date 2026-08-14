//go:build !darwin && !linux

package link

import (
	"fmt"

	"github.com/silenceper/aikit/pkg/config"
)

func openReconcileParent(config.PendingOperation) (reconcileParent, error) {
	return nil, fmt.Errorf("anchored reconcile recovery is unsupported on this platform")
}

func openReconcileParentReadOnly(config.PendingOperation) (reconcileParent, error) {
	return nil, fmt.Errorf("anchored reconcile recovery is unsupported on this platform")
}
