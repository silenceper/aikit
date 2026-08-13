//go:build !windows

package library

import "os"

func replaceJournalFile(from, to string) error { return os.Rename(from, to) }
