package updatecheck

import (
	"context"
	"time"
)

const DefaultTTL = 10 * time.Minute

type State string

const (
	StateCurrent         State = "current"
	StateUpdateAvailable State = "update-available"
	StatePinned          State = "pinned"
	StateLocal           State = "local"
	StateCheckFailed     State = "check-failed"
	StateOffline         State = "offline"
)

type Result struct {
	SkillID   string `json:"skill_id"`
	Current   string `json:"current,omitempty"`
	Remote    string `json:"remote,omitempty"`
	State     State  `json:"state"`
	FromCache bool   `json:"from_cache,omitempty"`
	Error     string `json:"error,omitempty"`
}

type CheckReport struct {
	Results  []Result `json:"results"`
	Warnings []string `json:"warnings,omitempty"`
}

type CheckOptions struct {
	Offline      bool
	ForceRefresh bool
}

type GitRunner interface {
	RemoteBranchHead(ctx context.Context, source, branch string) (string, error)
}

type Option func(*Checker)

func WithNow(now func() time.Time) Option {
	return func(checker *Checker) {
		if now != nil {
			checker.now = now
		}
	}
}

func WithTTL(ttl time.Duration) Option {
	return func(checker *Checker) {
		if ttl > 0 {
			checker.ttl = ttl
		}
	}
}
