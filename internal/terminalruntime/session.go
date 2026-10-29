// Package terminalruntime owns one real PTY process, its bounded byte history,
// and the original input receipts. It does not authorize users, dispatch work,
// infer task completion, or restart a process after a lost receipt. Those are
// responsibilities of the platform DAG and terminal-session aggregate.
package terminalruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	pty "github.com/aymanbagabas/go-pty"
)

const (
	MaxInputBytes       = 8 * 1024
	MaxChunkBytes       = 32 * 1024
	DefaultHistoryBytes = 2 * 1024 * 1024
	MaxRetainedChunks   = 4096
)

var (
	ErrInvalidConfig = errors.New("terminal runtime configuration is invalid")
	ErrJournalExists = errors.New("terminal journal already exists; do not restart the original process")
	ErrNotRunning    = errors.New("terminal process is not accepting input")
	ErrInputConflict = errors.New("terminal input identity or sequence conflicts with the original receipt")
	ErrInputLimit    = errors.New("terminal input receipt limit reached")
	ErrInputUnknown  = errors.New("terminal input delivery is unknown; do not replay its bytes")
	ErrTreeUnknown   = errors.New("terminal descendant containment could not be confirmed")
	identifier       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// Config is supplied only after the caller has authorized the original DAG
// assignment, acquired its exclusive workspace writer lease, and built an
// explicit environment. Nil Env is rejected rather than inherited from Runner.
type Config struct {
	ID                  string
	Executable          string
	BootstrapExecutable string
	BootstrapArgs       []string
	Args                []string
	WorkDir             string
	Env                 []string
	JournalDir          string
	Columns             int
	Rows                int
	MaxOutputBytes      int
	MaxOutputChunks     int
	MaxInputs           int
	MaxDuration         time.Duration
}

type InputReceipt struct {
	InputID      string `json:"input_id"`
	Sequence     uint64 `json:"sequence"`
	SHA256       string `json:"sha256"`
	Bytes        int    `json:"bytes"`
	State        string `json:"state"`
	BytesWritten int    `json:"bytes_written"`
}

// OutputChunk preserves original PTY bytes. UTF-8 and ANSI may span chunks;
// consumers must pass them to a streaming terminal decoder, never HTML.
type OutputChunk struct {
	Sequence   uint64 `json:"sequence"`
	ByteOffset uint64 `json:"byte_offset"`
	Data       []byte `json:"data"`
	SHA256     string `json:"sha256"`
}

type OutputPage struct {
	Chunks        []OutputChunk `json:"chunks"`
	FirstSequence uint64        `json:"first_sequence"`
	LastSequence  uint64        `json:"last_sequence"`
	NextSequence  uint64        `json:"next_sequence"`
	ByteOffset    uint64        `json:"byte_offset"`
	Gap           bool          `json:"gap"`
}

// Snapshot separates shell exit from confirmed descendant termination. Done
// may close with State=reconciling; that does not release an effect/write lease.
type Snapshot struct {
	ID                 string     `json:"id"`
	State              string     `json:"state"`
	ProcessID          int        `json:"process_id"`
	WorkDir            string     `json:"work_dir"`
	Columns            int        `json:"columns"`
	Rows               int        `json:"rows"`
	StartedAt          time.Time  `json:"started_at"`
	EndedAt            *time.Time `json:"ended_at,omitempty"`
	ExitCode           *int       `json:"exit_code,omitempty"`
	TreeStopped        bool       `json:"tree_stopped"`
	LastOutputSequence uint64     `json:"last_output_sequence"`
	OutputBytes        uint64     `json:"output_bytes"`
	InputCount         int        `json:"input_count"`
	Reason             string     `json:"reason"`
}

type treeController interface {
	Stop() error
	Finish() error
	Close()
	ActiveProcesses() (int, error)
}

type Session struct {
	mu            sync.Mutex
	inputMu       sync.Mutex
	stopOnce      sync.Once
	closeOnce     sync.Once
	pty           pty.Pty
	cmd           *pty.Cmd
	tree          treeController
	config        Config
	journal       *os.Root
	state         Snapshot
	chunks        []OutputChunk
	retained      int
	inputs        map[string]InputReceipt
	pendingInput  bool
	finishing     bool
	inputWrites   sync.WaitGroup
	stopRequested bool
	outputErr     error
	done          chan struct{}
	readDone      chan struct{}
}

// Probe checks the local OS PTY provider without starting a shell, loading a
// profile, calling a model, or touching a user workspace.
func Probe() error {
	if runtime.GOOS != "windows" {
		return ErrTreeUnknown
	}
	p, err := pty.New()
	if err != nil {
		return err
	}
	return p.Close()
}

// Start never reattaches by PID or starts from an existing journal. A caller
// recovering an older assignment must inspect the original receipt instead.
func Start(c Config) (*Session, error) {
	if c.Columns == 0 {
		c.Columns = 100
	}
	if c.Rows == 0 {
		c.Rows = 28
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = DefaultHistoryBytes
	}
	if c.MaxOutputChunks == 0 {
		c.MaxOutputChunks = MaxRetainedChunks
	}
	if c.MaxInputs == 0 {
		c.MaxInputs = 4096
	}
	if c.MaxDuration == 0 {
		c.MaxDuration = 30 * time.Minute
	}
	if !identifier.MatchString(c.ID) || !filepath.IsAbs(c.Executable) || !filepath.IsAbs(c.WorkDir) || !filepath.IsAbs(c.JournalDir) || c.Env == nil || !validSize(c.Columns, c.Rows) || c.MaxOutputBytes < MaxChunkBytes || c.MaxOutputBytes > 4*1024*1024 || c.MaxOutputChunks < 1 || c.MaxOutputChunks > MaxRetainedChunks || c.MaxInputs < 1 || c.MaxInputs > 65536 || c.MaxDuration < time.Second || c.MaxDuration > 2*time.Hour {
		return nil, ErrInvalidConfig
	}
	for _, arg := range append(append([]string{}, c.Args...), c.Env...) {
		if strings.ContainsRune(arg, 0) {
			return nil, ErrInvalidConfig
		}
	}
	for _, env := range c.Env {
		key, _, ok := strings.Cut(env, "=")
		if !ok || key == "" {
			return nil, ErrInvalidConfig
		}
	}
	info, err := os.Stat(c.WorkDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: working directory", ErrInvalidConfig)
	}
	if err := os.MkdirAll(c.JournalDir, 0700); err != nil {
		return nil, err
	}
	info, err = os.Lstat(c.JournalDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalidConfig
	}
	files, err := os.ReadDir(c.JournalDir)
	if err != nil {
		return nil, err
	}
	if len(files) != 0 {
		return nil, ErrJournalExists
	}
	j, err := os.OpenRoot(c.JournalDir)
	if err != nil {
		return nil, err
	}
	s := &Session{config: c, journal: j, inputs: map[string]InputReceipt{}, done: make(chan struct{}), readDone: make(chan struct{})}
	s.state = Snapshot{ID: c.ID, State: "starting", WorkDir: c.WorkDir, Columns: c.Columns, Rows: c.Rows, StartedAt: time.Now().UTC()}
	if err := s.persist("session.json", s.state); err != nil {
		j.Close()
		return nil, err
	}
	p, err := pty.New()
	if err != nil {
		_ = s.persist("start-error.json", map[string]string{"state": "failed", "reason": err.Error()})
		j.Close()
		return nil, err
	}
	s.pty = p
	if err = p.Resize(c.Columns, c.Rows); err != nil {
		s.closePTY()
		j.Close()
		return nil, err
	}
	s.cmd, err = newPTYCommand(p, c)
	if err != nil {
		s.closePTY()
		j.Close()
		return nil, err
	}
	s.cmd.Dir = c.WorkDir
	s.cmd.Env = append([]string{}, c.Env...)
	s.tree, err = startPTYProcess(s.cmd)
	if err != nil {
		s.closePTY()
		_ = s.persist("start-error.json", map[string]string{"state": "failed", "reason": err.Error()})
		j.Close()
		return nil, err
	}
	s.state.ProcessID = s.cmd.Process.Pid
	s.state.State = "running"
	if err = s.persist("running.json", s.state); err != nil {
		_ = s.tree.Stop()
		s.closePTY()
		_ = s.cmd.Wait()
		s.tree.Close()
		j.Close()
		return nil, err
	}
	go s.readLoop()
	go s.waitLoop()
	go func() {
		timer := time.NewTimer(c.MaxDuration)
		defer timer.Stop()
		select {
		case <-s.done:
			return
		case <-timer.C:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}()
	return s, nil
}

func validSize(columns, rows int) bool {
	return columns >= 2 && columns <= 512 && rows >= 1 && rows <= 256
}
func digest(b []byte) string             { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
func (s *Session) Done() <-chan struct{} { return s.done }

// ActiveProcesses queries the original process containment object, never a
// remembered PID. It is useful for local health and actual descendant tests.
func (s *Session) ActiveProcesses() (int, error) {
	if s.Snapshot().TreeStopped {
		return 0, nil
	}
	return s.tree.ActiveProcesses()
}
func (s *Session) InputReceipt(id string) (InputReceipt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.inputs[id]
	return r, ok
}
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.state
	if v.EndedAt != nil {
		x := *v.EndedAt
		v.EndedAt = &x
	}
	if v.ExitCode != nil {
		x := *v.ExitCode
		v.ExitCode = &x
	}
	return v
}

// SubmitInput writes only once. An accepted intent stores identity/hash/size,
// not sensitive terminal input text. Unknown/partial writes are never replayed.
func (s *Session) SubmitInput(ctx context.Context, id string, sequence uint64, data []byte) (InputReceipt, error) {
	if !identifier.MatchString(id) || sequence == 0 || len(data) == 0 || len(data) > MaxInputBytes {
		return InputReceipt{}, ErrInvalidConfig
	}
	r := InputReceipt{InputID: id, Sequence: sequence, SHA256: digest(data), Bytes: len(data), State: "delivery_unknown"}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.mu.Lock()
	if prior, ok := s.inputs[id]; ok {
		s.mu.Unlock()
		if prior.Sequence != sequence || prior.SHA256 != r.SHA256 || prior.Bytes != r.Bytes {
			return InputReceipt{}, ErrInputConflict
		}
		if prior.State == "delivery_unknown" {
			return prior, ErrInputUnknown
		}
		return prior, nil
	}
	if s.state.State != "running" || s.pendingInput || s.finishing {
		s.mu.Unlock()
		return InputReceipt{}, ErrNotRunning
	}
	if len(s.inputs) >= s.config.MaxInputs {
		s.mu.Unlock()
		return InputReceipt{}, ErrInputLimit
	}
	if sequence != uint64(len(s.inputs)+1) {
		s.mu.Unlock()
		return InputReceipt{}, ErrInputConflict
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return InputReceipt{}, ctx.Err()
	default:
	}
	if err := s.persist(inputName(sequence, "intent"), r); err != nil {
		return InputReceipt{}, err
	}
	s.mu.Lock()
	s.inputs[id] = r
	s.pendingInput = true
	s.state.InputCount = len(s.inputs)
	s.mu.Unlock()
	finished := make(chan InputReceipt, 1)
	payload := append([]byte{}, data...)
	s.inputWrites.Add(1)
	go func() {
		defer s.inputWrites.Done()
		n, err := s.pty.Write(payload)
		result := r
		result.BytesWritten = n
		if err == nil && n == len(payload) {
			result.State = "delivered"
		}
		// A durable final receipt must exist before reporting delivery. Missing
		// final bytes after a crash leave the pre-write intent explicitly unknown.
		if e := s.persist(inputName(sequence, "result"), result); e != nil {
			result.State = "delivery_unknown"
		}
		s.mu.Lock()
		s.inputs[id] = result
		s.pendingInput = false
		s.mu.Unlock()
		finished <- result
	}()
	select {
	case result := <-finished:
		if result.State != "delivered" {
			return result, ErrInputUnknown
		}
		return result, nil
	case <-ctx.Done():
		s.mu.Lock()
		result := s.inputs[id]
		s.mu.Unlock()
		if result.State == "delivered" {
			return result, nil
		}
		return result, ErrInputUnknown
	}
}

func (s *Session) Resize(columns, rows int) error {
	if !validSize(columns, rows) {
		return ErrInvalidConfig
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.State != "running" {
		return ErrNotRunning
	}
	if err := s.pty.Resize(columns, rows); err != nil {
		return err
	}
	s.state.Columns = columns
	s.state.Rows = rows
	return nil
}

// ReadOutput returns a bounded clone of original chunks. A gap reports lost
// retained history explicitly; it must not be displayed as a blank transcript.
func (s *Session) ReadOutput(after uint64, maxBytes int) OutputPage {
	if maxBytes <= 0 || maxBytes > 128*1024 {
		maxBytes = 128 * 1024
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := OutputPage{Chunks: []OutputChunk{}, LastSequence: s.state.LastOutputSequence, NextSequence: after, ByteOffset: s.state.OutputBytes}
	if len(s.chunks) == 0 {
		return p
	}
	p.FirstSequence = s.chunks[0].Sequence
	p.Gap = after < p.FirstSequence-1
	size := 0
	for _, chunk := range s.chunks {
		if chunk.Sequence <= after {
			continue
		}
		if size > 0 && size+len(chunk.Data) > maxBytes {
			break
		}
		v := chunk
		v.Data = append([]byte{}, chunk.Data...)
		p.Chunks = append(p.Chunks, v)
		p.NextSequence = v.Sequence
		size += len(v.Data)
	}
	return p
}

func (s *Session) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.state.State == "exited" || s.state.State == "stopped" {
			s.mu.Unlock()
			return
		}
		s.stopRequested = true
		s.state.State = "stop_requested"
		snapshot := s.state
		s.mu.Unlock()
		if err := s.persist("stop-requested.json", snapshot); err != nil {
			s.mu.Lock()
			s.state.Reason = err.Error()
			s.mu.Unlock()
		}
		go func() {
			if err := s.tree.Stop(); err != nil {
				s.mu.Lock()
				s.state.State = "reconciling"
				s.state.Reason = err.Error()
				s.mu.Unlock()
			}
			s.closePTY()
		}()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
	}
	if !s.Snapshot().TreeStopped {
		return ErrTreeUnknown
	}
	return nil
}

func (s *Session) closePTY() { s.closeOnce.Do(func() { _ = s.pty.Close() }) }
func (s *Session) readLoop() {
	defer close(s.readDone)
	buffer := make([]byte, MaxChunkBytes)
	for {
		n, err := s.pty.Read(buffer)
		if n > 0 {
			if e := s.retainOutput(buffer[:n]); e != nil {
				s.mu.Lock()
				s.outputErr = e
				s.mu.Unlock()
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = s.Stop(ctx)
				}()
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				s.mu.Lock()
				if s.state.State == "running" && !s.finishing && !errors.Is(err, os.ErrClosed) {
					s.outputErr = err
				}
				s.mu.Unlock()
			}
			return
		}
	}
}

// retainOutput enforces byte and file-count limits together. Small key-echo
// chunks cannot create an unbounded number of durable output files.
func (s *Session) retainOutput(raw []byte) error {
	if len(raw) < 1 || len(raw) > MaxChunkBytes {
		return ErrInvalidConfig
	}
	data := append([]byte{}, raw...)
	s.mu.Lock()
	chunk := OutputChunk{Sequence: s.state.LastOutputSequence + 1, ByteOffset: s.state.OutputBytes, Data: data, SHA256: digest(data)}
	s.mu.Unlock()
	if err := s.persistBytes(outputName(chunk), data); err != nil {
		return err
	}
	s.mu.Lock()
	s.chunks = append(s.chunks, chunk)
	s.retained += len(data)
	s.state.OutputBytes += uint64(len(data))
	s.state.LastOutputSequence = chunk.Sequence
	var discard []OutputChunk
	for (s.retained > s.config.MaxOutputBytes || len(s.chunks) > s.config.MaxOutputChunks) && len(s.chunks) > 1 {
		old := s.chunks[0]
		s.chunks = s.chunks[1:]
		s.retained -= len(old.Data)
		discard = append(discard, old)
	}
	s.mu.Unlock()
	for _, old := range discard {
		if err := s.journal.Remove(outputName(old)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) waitLoop() {
	err := s.cmd.Wait()
	stopErr := s.tree.Finish()
	s.inputMu.Lock()
	s.mu.Lock()
	s.finishing = true
	s.mu.Unlock()
	s.inputMu.Unlock()
	s.closePTY()
	<-s.readDone
	s.inputWrites.Wait()
	s.mu.Lock()
	ended := time.Now().UTC()
	s.state.EndedAt = &ended
	if s.cmd.ProcessState != nil {
		code := s.cmd.ProcessState.ExitCode()
		s.state.ExitCode = &code
	}
	s.state.TreeStopped = stopErr == nil
	s.state.State = "exited"
	if s.stopRequested {
		s.state.State = "stopped"
	}
	if err != nil {
		s.state.Reason = err.Error()
	}
	if stopErr != nil || s.outputErr != nil {
		s.state.State = "reconciling"
		s.state.Reason = errors.Join(stopErr, s.outputErr).Error()
	}
	final := s.state
	s.mu.Unlock()
	if err := s.persist("terminal.json", final); err != nil {
		s.mu.Lock()
		s.state.State = "reconciling"
		s.state.Reason = err.Error()
		s.mu.Unlock()
	}
	s.tree.Close()
	_ = s.journal.Close()
	close(s.done)
}
