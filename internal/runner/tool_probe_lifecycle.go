package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

type toolProbeContextKey struct{}

var toolProbeReceiptID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type toolProbeSource struct {
	Protocol         string       `json:"protocol"`
	ScopeSHA256      string       `json:"scope_sha256"`
	Inspection       ti.Reference `json:"inspection"`
	InstallationScan ii.Reference `json:"installation_scan"`
	Preparation      tp.Reference `json:"preparation"`
	SourceSHA256     string       `json:"source_sha256"`
}

// The two real maintenance requests have different immutable sources. Keep
// inspection's original wire bytes; a scan never invents an inspection ID.
func (s toolProbeSource) MarshalJSON() ([]byte, error) {
	type wire struct {
		Protocol    string        `json:"protocol"`
		Scope       string        `json:"scope_sha256"`
		Inspection  *ti.Reference `json:"inspection,omitempty"`
		Scan        *ii.Reference `json:"installation_scan,omitempty"`
		Preparation *tp.Reference `json:"preparation,omitempty"`
		Source      string        `json:"source_sha256"`
	}
	v := wire{Protocol: s.Protocol, Scope: s.ScopeSHA256, Source: s.SourceSHA256}
	if s.Protocol == tp.StopProtocol {
		v.Preparation = &s.Preparation
	} else if s.Protocol == ii.StopProtocol {
		v.Scan = &s.InstallationScan
	} else {
		v.Inspection = &s.Inspection
	}
	return json.Marshal(v)
}
func (s *toolProbeSource) UnmarshalJSON(raw []byte) error {
	fields, err := decodeToolObject(raw)
	if err != nil || len(fields) != 4 {
		return fmt.Errorf("invalid maintenance probe source")
	}
	type wire toolProbeSource
	var v wire
	if json.Unmarshal(raw, &v) != nil {
		return fmt.Errorf("invalid maintenance probe source")
	}
	required := "inspection"
	if v.Protocol == tp.StopProtocol {
		required = "preparation"
		if tp.ValidateReference(v.Preparation) != nil {
			return fmt.Errorf("invalid preparation probe source")
		}
	} else if v.Protocol == ii.StopProtocol {
		required = "installation_scan"
		if ii.ValidateReference(v.InstallationScan) != nil {
			return fmt.Errorf("invalid scan probe source")
		}
	} else if v.Protocol != ti.ProbeStopProtocol || ti.ValidateReference(v.Inspection) != nil {
		return fmt.Errorf("invalid inspection probe source")
	}
	for key := range fields {
		if key != "protocol" && key != "scope_sha256" && key != "source_sha256" && key != required {
			return fmt.Errorf("mixed maintenance probe source")
		}
	}
	if _, ok := fields[required]; !ok || !tc.ValidDigest(v.ScopeSHA256) || !tc.ValidDigest(v.SourceSHA256) {
		return fmt.Errorf("incomplete maintenance probe source")
	}
	*s = toolProbeSource(v)
	return nil
}

type toolProbeIntent struct {
	Source        toolProbeSource `json:"source"`
	ID            string          `json:"id"`
	CommandSHA256 string          `json:"command_sha256"`
	CreatedAt     string          `json:"created_at"`
}
type toolProbeStop struct {
	Source        toolProbeSource `json:"source"`
	ID            string          `json:"id"`
	NativeStarted bool            `json:"native_started"`
	ParentStopped bool            `json:"parent_stopped"`
	TreeStopped   bool            `json:"tree_stopped"`
	Containment   string          `json:"containment"`
	ExitCode      *int            `json:"exit_code,omitempty"`
	ResultCode    string          `json:"result_code"`
	ObservedAt    string          `json:"observed_at"`
}
type toolProbeEntry struct {
	ID           string `json:"id"`
	IntentSHA256 string `json:"intent_sha256"`
	StopSHA256   string `json:"stop_sha256"`
}
type toolProbeReceipt struct {
	Source                toolProbeSource  `json:"source"`
	ProbeReturned         bool             `json:"probe_returned"`
	AllNativeTreesStopped bool             `json:"all_native_trees_stopped"`
	NativeProcessCount    int              `json:"native_process_count"`
	StoppedProcessCount   int              `json:"stopped_process_count"`
	ObservedAt            string           `json:"observed_at"`
	Processes             []toolProbeEntry `json:"processes"`
}
type toolProbeCollector struct {
	mu        sync.Mutex
	directory string
	source    toolProbeSource
	entries   map[string]toolProbeEntry
	stops     map[string]toolProbeStop
	closed    bool
	failed    bool
}

// Immutable local receipts are created exclusively and synced before the
// corresponding network command. Existing bytes may be recovered, not replaced.
func writeToolProbeImmutable(path string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		st, e := os.Lstat(path)
		if e != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 2<<20 {
			return "", fmt.Errorf("original probe receipt is not a bounded regular file")
		}
		old, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(old, raw) {
			return "", fmt.Errorf("original probe receipt differs")
		}
		return digest(old), nil
	}
	if err != nil {
		return "", err
	}
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return "", e
	}
	return digest(raw), nil
}

func (j *Journal) toolProbeDirectory(p toolInspectionCheckpoint) (string, toolProbeSource, error) {
	if validateToolInspectionCheckpoint(p) != nil || p.Inspection == nil || !p.Started {
		return "", toolProbeSource{}, fmt.Errorf("original inspection start checkpoint required")
	}
	source := toolProbeSource{Protocol: ti.ProbeStopProtocol, ScopeSHA256: p.ScopeSHA256, Inspection: inspectionReference(*p.Inspection), SourceSHA256: ti.InspectionSourceSHA256(*p.Inspection)}
	raw, _ := json.Marshal(source)
	return filepath.Join(filepath.Dir(j.path), "tool-inspection-receipts", digest(raw)), source, nil
}

func newToolProbeCollector(j *Journal, p toolInspectionCheckpoint) (*toolProbeCollector, error) {
	dir, source, err := j.toolProbeDirectory(p)
	if err != nil {
		return nil, err
	}
	return newToolProbeCollectorAt(dir, source)
}

func newToolProbeCollectorAt(dir string, source toolProbeSource) (*toolProbeCollector, error) {
	var err error
	for _, part := range []string{filepath.Dir(dir), dir, filepath.Join(dir, "probes")} {
		if err = os.Mkdir(part, 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
		st, e := os.Lstat(part)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("probe receipt directory is not an owned directory")
		}
	}
	if _, err = writeToolProbeImmutable(filepath.Join(dir, "source.json"), source); err != nil {
		return nil, err
	}
	if _, err = os.Lstat(filepath.Join(dir, "returned.json")); err == nil || !os.IsNotExist(err) {
		return nil, fmt.Errorf("original probe was already returned or cannot be inspected")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "probes"))
	if err != nil || len(entries) != 0 {
		return nil, fmt.Errorf("original native probe intents cannot be re-executed")
	}
	return &toolProbeCollector{directory: dir, source: source, entries: map[string]toolProbeEntry{}, stops: map[string]toolProbeStop{}}, nil
}
func (c *toolProbeCollector) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, toolProbeContextKey{}, c)
}
func toolProbeCollectorFrom(ctx context.Context) *toolProbeCollector {
	c, _ := ctx.Value(toolProbeContextKey{}).(*toolProbeCollector)
	return c
}

func (c *toolProbeCollector) begin(cmd *exec.Cmd) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed {
		return "", newProbeFailure(probeStage(cmd.Args[1:]), "stop_unverified", nil)
	}
	if c.closed || len(c.entries) >= ti.MaxNativeProcesses {
		c.failed = true
		return "", fmt.Errorf("native probe receipt boundary is closed or full")
	}
	id := operationID()
	raw, _ := json.Marshal([]any{cmd.Path, cmd.Args, cmd.Dir})
	intent := toolProbeIntent{Source: c.source, ID: id, CommandSHA256: digest(raw), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	sha, err := writeToolProbeImmutable(filepath.Join(c.directory, "probes", id+".start.json"), intent)
	if err != nil {
		c.failed = true
		return "", err
	}
	c.entries[id] = toolProbeEntry{ID: id, IntentSHA256: sha}
	return id, nil
}
func (c *toolProbeCollector) record(stop toolProbeStop) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[stop.ID]
	if !ok || c.closed {
		c.failed = true
		return fmt.Errorf("native probe receipt has no original intent")
	}
	sha, err := writeToolProbeImmutable(filepath.Join(c.directory, "probes", stop.ID+".stop.json"), stop)
	if err != nil {
		c.failed = true
		return err
	}
	entry.StopSHA256 = sha
	c.entries[stop.ID] = entry
	c.stops[stop.ID] = stop
	if stop.ResultCode != "stopped" || !stop.ParentStopped || !stop.TreeStopped {
		c.failed = true
	}
	return nil
}
func (c *toolProbeCollector) closeReturned() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("probe return boundary already closed")
	}
	c.closed = true
	receipt := toolProbeReceipt{Source: c.source, ProbeReturned: true, AllNativeTreesStopped: !c.failed, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Processes: []toolProbeEntry{}}
	ids := make([]string, 0, len(c.entries))
	for id := range c.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		entry := c.entries[id]
		receipt.Processes = append(receipt.Processes, entry)
		stop, ok := c.stops[id]
		if !ok || entry.StopSHA256 == "" {
			receipt.AllNativeTreesStopped = false
			continue
		}
		if stop.NativeStarted {
			receipt.NativeProcessCount++
		}
		if stop.NativeStarted && stop.ParentStopped && stop.TreeStopped {
			receipt.StoppedProcessCount++
		} else {
			receipt.AllNativeTreesStopped = false
		}
	}
	_, err := writeToolProbeImmutable(filepath.Join(c.directory, "returned.json"), receipt)
	return err
}
func (c *toolProbeCollector) hasUnverifiedNativeExit() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed || len(c.entries) != len(c.stops) {
		return true
	}
	for _, stop := range c.stops {
		if stop.ResultCode != "stopped" {
			return true
		}
	}
	return false
}

func readToolProbeJSON(path string, v any) (string, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 2<<20 {
		return "", fmt.Errorf("probe receipt is not a bounded regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if _, err = decodeToolObject(raw); err != nil {
		return "", fmt.Errorf("probe receipt is invalid JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return "", fmt.Errorf("probe receipt shape differs")
	}
	return digest(raw), nil
}

func (j *Journal) readToolProbeReceipt(p toolInspectionCheckpoint) (toolProbeReceipt, string, error) {
	dir, source, err := j.toolProbeDirectory(p)
	if err != nil {
		return toolProbeReceipt{}, "", err
	}
	return readToolProbeReceiptAt(dir, source)
}

func readToolProbeReceiptAt(dir string, source toolProbeSource) (toolProbeReceipt, string, error) {
	var err error
	for _, part := range []string{filepath.Dir(dir), dir, filepath.Join(dir, "probes")} {
		st, e := os.Lstat(part)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return toolProbeReceipt{}, "", fmt.Errorf("original probe receipt directory unavailable or exchanged")
		}
	}
	var storedSource toolProbeSource
	if _, err = readToolProbeJSON(filepath.Join(dir, "source.json"), &storedSource); err != nil || storedSource != source {
		return toolProbeReceipt{}, "", fmt.Errorf("original probe receipt source unavailable or differs")
	}
	var receipt toolProbeReceipt
	sha, err := readToolProbeJSON(filepath.Join(dir, "returned.json"), &receipt)
	if err != nil || receipt.Source != source || !receipt.ProbeReturned || len(receipt.Processes) > ti.MaxNativeProcesses {
		return receipt, "", fmt.Errorf("original returned probe receipt unavailable or differs")
	}
	if _, err = time.Parse(time.RFC3339Nano, receipt.ObservedAt); err != nil {
		return receipt, "", fmt.Errorf("invalid original probe observation")
	}
	files, err := os.ReadDir(filepath.Join(dir, "probes"))
	if err != nil || len(files) != len(receipt.Processes)*2 {
		return receipt, "", fmt.Errorf("original probe process set is incomplete")
	}
	seen := map[string]bool{}
	native, stopped := 0, 0
	all := true
	for _, entry := range receipt.Processes {
		if !toolProbeReceiptID.MatchString(entry.ID) || seen[entry.ID] {
			return receipt, "", fmt.Errorf("invalid original probe identity")
		}
		seen[entry.ID] = true
		var intent toolProbeIntent
		var stop toolProbeStop
		a, e := readToolProbeJSON(filepath.Join(dir, "probes", entry.ID+".start.json"), &intent)
		b, x := readToolProbeJSON(filepath.Join(dir, "probes", entry.ID+".stop.json"), &stop)
		if e != nil || x != nil || a != entry.IntentSHA256 || b != entry.StopSHA256 || intent.Source != source || stop.Source != source || intent.ID != entry.ID || stop.ID != entry.ID {
			return receipt, "", fmt.Errorf("original native stop receipt differs")
		}
		if !tc.ValidDigest(intent.CommandSHA256) {
			return receipt, "", fmt.Errorf("invalid original probe command digest")
		}
		if _, e = time.Parse(time.RFC3339Nano, intent.CreatedAt); e != nil {
			return receipt, "", fmt.Errorf("invalid original probe start time")
		}
		if _, e = time.Parse(time.RFC3339Nano, stop.ObservedAt); e != nil {
			return receipt, "", fmt.Errorf("invalid original probe stop time")
		}
		if stop.NativeStarted {
			native++
		}
		if stop.NativeStarted && stop.ParentStopped && stop.TreeStopped && authoritativeProcessContainment(stop.Containment) && stop.ResultCode == "stopped" {
			stopped++
		} else {
			all = false
		}
	}
	if native != receipt.NativeProcessCount || stopped != receipt.StoppedProcessCount || receipt.AllNativeTreesStopped && !all {
		return receipt, "", fmt.Errorf("native stop totals differ from original receipts")
	}
	return receipt, sha, nil
}

// A thin wrapper preserves the existing native containment implementation. It
// records only observed wait/stop facts, never treats handle close as proof.
type probeProcessController struct {
	native       *processController
	cmd          *exec.Cmd
	collector    *toolProbeCollector
	id           string
	mu           sync.Mutex
	waitReturned bool
	exitCode     *int
	stopErr      error
	finished     bool
	privateScope *privateProbeScope
	closeScope   func()
}

func startProbeProcess(ctx context.Context, cmd *exec.Cmd) (*probeProcessController, error) {
	if !nativeProbeContainmentConfigured() {
		return nil, newProbeFailure("source", "configuration_invalid", nil)
	}
	p := &probeProcessController{cmd: cmd, collector: toolProbeCollectorFrom(ctx)}
	var err error
	p.privateScope, p.closeScope, err = prepareProbeScope(ctx, cmd)
	if err != nil {
		return nil, newProbeFailure("source", "source_unavailable", err)
	}
	if p.collector != nil {
		p.id, err = p.collector.begin(cmd)
		if err != nil {
			p.closeScope()
			return nil, err
		}
	}
	// The owner handles cancellation and bounds both process wait and inherited
	// output pipes; CommandContext's independent parent-only kill is not proof.
	cmd.Cancel = nil
	cmd.WaitDelay = 2 * time.Second
	if err = p.privateScope.begin(); err != nil {
		p.closeScope()
		return nil, err
	}
	native, err := startNativeProcess(cmd, true)
	if err != nil {
		p.privateScope.end(false)
		p.closeScope()
		if p.collector != nil {
			_ = p.collector.record(toolProbeStop{Source: p.collector.source, ID: p.id, Containment: "unavailable", ResultCode: "start_unverified", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)})
		}
		return nil, newProbeFailure(probeStage(cmd.Args[1:]), "launch_failed", err)
	}
	p.native = native
	return p, nil
}
func (p *probeProcessController) Wait() error {
	err := p.cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waitReturned = true
	if p.cmd.ProcessState != nil {
		code := p.cmd.ProcessState.ExitCode()
		p.exitCode = &code
	}
	return err
}
func (p *probeProcessController) waitFact() (bool, *int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.exitCode == nil {
		return p.waitReturned, nil
	}
	code := *p.exitCode
	return p.waitReturned, &code
}
func (p *probeProcessController) Stop() error {
	err := p.native.Stop()
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil && p.stopErr == nil {
		p.stopErr = err
	}
	return err
}

// A missing Unix process group proves neither detached-descendant absence nor
// the stronger tree-stop contract. Keep that fact consistent for standalone
// probes as well as collectors; retaining a directory alone is not failure.
func requireProbeTreeStop(waitReturned bool, nativeErr error, containment string) error {
	if waitReturned && nativeErr == nil && authoritativeProcessContainment(containment) {
		return nil
	}
	return &toolProbeFailure{stage: "protocol", code: "stop_unverified", cause: nativeErr}
}

func (p *probeProcessController) Finish() error {
	err := p.native.Finish()
	p.mu.Lock()
	defer p.mu.Unlock()
	err = errors.Join(p.stopErr, err)
	containment := p.native.containment()
	err = requireProbeTreeStop(p.waitReturned, err, containment)
	if p.finished {
		return err
	}
	p.finished = true
	if p.collector != nil {
		code := "stopped"
		if err != nil {
			code = "stop_unverified"
		}
		stop := toolProbeStop{Source: p.collector.source, ID: p.id, NativeStarted: true, ParentStopped: p.waitReturned, TreeStopped: err == nil, Containment: containment, ExitCode: p.exitCode, ResultCode: code, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if writeErr := p.collector.record(stop); writeErr != nil {
			err = errors.Join(err, writeErr)
		}
	}
	p.privateScope.end(err == nil)
	return err
}
func (p *probeProcessController) Close() {
	p.mu.Lock()
	unfinished := !p.finished
	p.finished = true
	p.mu.Unlock()
	if unfinished && p.collector != nil {
		_ = p.collector.record(toolProbeStop{Source: p.collector.source, ID: p.id, NativeStarted: true, Containment: p.native.containment(), ResultCode: "stop_unverified", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	}
	if unfinished {
		p.privateScope.end(false)
	}
	p.native.Close()
	p.closeScope()
}
func waitProbeProcess(ctx context.Context, p *probeProcessController, done <-chan error) error {
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	stopErr := p.Stop()
	return errors.Join(ctx.Err(), stopErr, waitStoppedProbeProcess(p, done))
}
func waitStoppedProbeProcess(p *probeProcessController, done <-chan error) error {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
	}
	// Kill only the still-owned parent handle, retaining unknown tree facts.
	_ = p.cmd.Process.Kill()
	timer.Reset(2 * time.Second)
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("native probe wait remains unverified")
	}
}
func runProbeProcess(ctx context.Context, cmd *exec.Cmd) (error, error) {
	p, err := startProbeProcess(ctx, cmd)
	if err != nil {
		return err, err
	}
	defer p.Close()
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	waitErr := waitProbeProcess(ctx, p, done)
	stopErr := p.Finish()
	return waitErr, stopErr
}
