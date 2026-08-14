package library

import (
	"io"
	"io/fs"
)

// VerifiedSkillRoot is a skill-root filesystem anchored to an already-opened
// library directory. Implementations open every skill ID component without
// following symlinks or Windows reparse points.
type VerifiedSkillRoot interface {
	fs.FS
	io.Closer
	// Readlink returns the raw link payload relative to the already verified
	// skill root. It never resolves or opens the link destination.
	Readlink(string) (string, error)
}

func OpenVerifiedSkillRoot(libraryRoot, id string) (VerifiedSkillRoot, error) {
	return openVerifiedSkillRoot(libraryRoot, id, nil)
}
