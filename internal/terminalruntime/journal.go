package terminalruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func inputName(sequence uint64, kind string) string {
	return fmt.Sprintf("input-%020d.%s.json", sequence, kind)
}
func outputName(chunk OutputChunk) string {
	return fmt.Sprintf("output-%020d-%020d.bin", chunk.Sequence, chunk.ByteOffset)
}
func (s *Session) persist(name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.persistBytes(name, append(raw, '\n'))
}
func (s *Session) persistBytes(name string, raw []byte) error {
	f, err := s.journal.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(raw)
	if writeErr == nil && n != len(raw) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	return errors.Join(writeErr, f.Close())
}

// JournalInspection contains original durable facts only. Running/starting is
// always changed to reconciling when no live Session owns the process; a PID in
// a file is never sufficient authority to attach, send input, or kill a process.
type JournalInspection struct {
	Session Snapshot       `json:"session"`
	Inputs  []InputReceipt `json:"inputs"`
}

func InspectJournal(dir string) (JournalInspection, error) {
	j, err := os.OpenRoot(dir)
	if err != nil {
		return JournalInspection{}, err
	}
	defer j.Close()
	read := func(name string, target any) error {
		f, err := j.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		d := json.NewDecoder(io.LimitReader(f, 64*1024))
		d.DisallowUnknownFields()
		if err := d.Decode(target); err != nil {
			return err
		}
		var more any
		if err := d.Decode(&more); err != io.EOF {
			return errors.New("terminal journal has trailing data")
		}
		return nil
	}
	var result JournalInspection
	if err = read("session.json", &result.Session); err != nil {
		return result, err
	}
	if err = read("terminal.json", &result.Session); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		result.Session.State = "reconciling"
		result.Session.Reason = "original terminal process ownership must be reconciled"
	}
	d, err := j.Open(".")
	if err != nil {
		return result, err
	}
	defer d.Close()
	const maxEntries = 2*65536 + MaxRetainedChunks + 16
	names, err := d.Readdirnames(maxEntries + 1)
	if err != nil && err != io.EOF {
		return result, err
	}
	if len(names) > maxEntries {
		return result, errors.New("terminal journal entry limit exceeded")
	}
	sort.Strings(names)
	result.Inputs = []InputReceipt{}
	for _, name := range names {
		if !strings.HasPrefix(name, "input-") || !strings.HasSuffix(name, ".intent.json") {
			continue
		}
		var r InputReceipt
		if err = read(name, &r); err != nil {
			return result, err
		}
		if r.State != "delivery_unknown" || !identifier.MatchString(r.InputID) || r.Sequence == 0 || r.Bytes < 1 || r.Bytes > MaxInputBytes || len(r.SHA256) != 64 {
			return result, errors.New("terminal input intent is invalid")
		}
		var final InputReceipt
		if err = read(inputName(r.Sequence, "result"), &final); err == nil {
			if final.InputID != r.InputID || final.Sequence != r.Sequence || final.SHA256 != r.SHA256 || final.Bytes != r.Bytes || (final.State != "delivered" && final.State != "delivery_unknown") || final.BytesWritten < 0 || final.BytesWritten > final.Bytes || (final.State == "delivered" && final.BytesWritten != final.Bytes) {
				return result, errors.New("terminal input receipt differs from original intent")
			}
			r = final
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
		result.Inputs = append(result.Inputs, r)
	}
	return result, nil
}
