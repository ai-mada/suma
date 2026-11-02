package runner

import (
	"fmt"
	"os"
	"path/filepath"
)

func (j *Journal) requireNoPendingBusiness() error {
	for _, e := range j.entries {
		if !e.Acked || e.Result == nil || e.ArtifactPending != nil || e.CandidatePending != nil {
			return fmt.Errorf("original business execution or receipt remains pending")
		}
	}
	if j.claim != nil && !j.claim.Complete {
		return fmt.Errorf("original business claim remains pending")
	}
	if j.workspaceWrites.Claim != nil && !j.workspaceWrites.Claim.Complete {
		return fmt.Errorf("original workspace claim remains pending")
	}
	for _, e := range j.workspaceWrites.Entries {
		if !e.Acked && !e.Retired {
			return fmt.Errorf("original workspace operation remains pending")
		}
	}
	for _, e := range j.workspaceObservations {
		if !e.Acked && !e.Retired {
			return fmt.Errorf("original workspace observation remains pending")
		}
	}
	return nil
}

func requireDrainedPoolSlots(directory string) error {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("pool slot state is unavailable")
		}
		j, e := OpenMaintenanceJournal(filepath.Join(directory, entry.Name()))
		if e != nil {
			return e
		}
		_ = j.Close()
	}
	return nil
}
