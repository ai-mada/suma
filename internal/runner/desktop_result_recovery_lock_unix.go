//go:build !windows

package runner

import "os"

func lockExistingJournalForRecovery(path string) (*os.File, error) { return lockExistingJournal(path) }
