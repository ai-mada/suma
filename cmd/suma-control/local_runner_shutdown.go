package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	desktopRunnerShutdownType   = "suma.local_runner.shutdown"
	desktopRunnerShutdownMarker = "managed-shutdown.json"
	desktopRunnerShutdownLimit  = 1024
)

type desktopRunnerShutdownReceipt struct {
	Type        string `json:"type"`
	WorkerID    string `json:"worker_id"`
	PID         int    `json:"pid"`
	OperationID string `json:"operation_id"`
}

func validDesktopStopOperationID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, digit := range value {
		if !(digit >= '0' && digit <= '9' || digit >= 'a' && digit <= 'f') {
			return false
		}
	}
	return true
}

// Publish a synced proof next to the runtime directory, which is reserved for
// the original Worker Journal. Recovery can only use it with the same intent.
func persistDesktopRunnerShutdown(stateDirectory string, receipt desktopRunnerShutdownReceipt) ([]byte, error) {
	stateDirectory = filepath.Clean(stateDirectory)
	scopeDirectory := filepath.Dir(stateDirectory)
	scope, err := os.Lstat(scopeDirectory)
	if err != nil || !scope.IsDir() || stateDirectory == scopeDirectory {
		return nil, errors.New("stop proof scope is not a private directory")
	}
	root, err := os.OpenRoot(scopeDirectory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(scope, opened) {
		return nil, errors.New("stop proof scope changed")
	}
	state, err := root.Lstat(filepath.Base(stateDirectory))
	if err != nil || !state.IsDir() {
		return nil, errors.New("stop proof runtime directory unavailable")
	}
	if err = checkPreviousDesktopRunnerShutdown(root, receipt.OperationID); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	// The committed name is never opened for writing. A crash or write failure
	// before rename leaves only the previous proof or an untrusted staging file.
	stage := ".managed-shutdown-" + receipt.OperationID + ".tmp"
	file, err := root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	defer root.Remove(stage)
	if n, err := file.Write(raw); err != nil || n != len(raw) {
		return nil, errors.Join(io.ErrShortWrite, err, file.Close())
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return nil, err
	}
	if err := publishDesktopRunnerShutdown(root, stage); err != nil {
		return nil, err
	}
	return raw, nil
}

func publishDesktopRunnerShutdown(root *os.Root, stage string) error {
	if runtime.GOOS == "windows" {
		return root.Rename(stage, desktopRunnerShutdownMarker)
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := root.Rename(stage, desktopRunnerShutdownMarker); err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		return errors.Join(err, root.Remove(desktopRunnerShutdownMarker))
	}
	return nil
}

func checkPreviousDesktopRunnerShutdown(root *os.Root, operationID string) error {
	info, err := root.Lstat(desktopRunnerShutdownMarker)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	invalid := errors.New("original stop proof is invalid")
	if err != nil || !info.Mode().IsRegular() || info.Size() > desktopRunnerShutdownLimit {
		return invalid
	}
	file, err := root.Open(desktopRunnerShutdownMarker)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return invalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, desktopRunnerShutdownLimit+1))
	if err != nil || len(raw) > desktopRunnerShutdownLimit {
		return invalid
	}
	var prior desktopRunnerShutdownReceipt
	if json.Unmarshal(raw, &prior) != nil || prior.Type != desktopRunnerShutdownType || prior.PID <= 0 {
		return nil
	}
	workerSuffix, desktop := strings.CutPrefix(prior.WorkerID, "desktop-")
	if desktop && validDesktopStopOperationID(workerSuffix) && prior.OperationID == operationID {
		return errors.New("stop operation already has a proof")
	}
	return nil
}
