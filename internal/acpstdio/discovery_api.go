package acpstdio

import (
	"context"
	"errors"
	"fmt"
	"io"

	ac "github.com/ai-mada/suma/internal/acpcontract"
)

type DiscoveryOptions struct {
	Cwd     string
	Runtime string
	Secrets []string
}

// DiscoveryObservation is provisional protocol evidence. The caller must add
// its real process/containment facts before adopting a public ready result.
type DiscoveryObservation struct {
	Catalogue                          *ac.ACPModelCatalogue
	Evidence                           ac.ACPModelDiscoveryEvidence
	AgentName, AgentVersion, ErrorCode string
}
type ModelDiscovery struct{ d *Driver }

func (DiscoveryOptions) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "acpstdio.DiscoveryOptions{redacted}")
}
func (DiscoveryObservation) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "acpstdio.DiscoveryObservation{redacted}")
}
func (*ModelDiscovery) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "acpstdio.ModelDiscovery{redacted}")
}

// NewModelDiscovery has no goal, fixed model or permission hook. Its sole
// background SDK stack ends naturally; CopyInput then drains and returns.
// The caller closes process stdin and waits before invoking Finish.
func NewModelDiscovery(ctx context.Context, o DiscoveryOptions) (*ModelDiscovery, error) {
	d, err := newTransport(ctx, Options{Cwd: o.Cwd, Runtime: o.Runtime, Secrets: o.Secrets}, &discoveryState{})
	if err != nil {
		return nil, errors.New("acp_discovery_invalid")
	}
	return &ModelDiscovery{d: d}, nil
}
func (m *ModelDiscovery) Write(b []byte) (int, error)          { return m.d.Write(b) }
func (m *ModelDiscovery) CopyInput(w io.Writer) (int64, error) { return m.d.CopyInput(w) }
func (m *ModelDiscovery) WaitCatalogue(ctx context.Context) (ac.ACPModelCatalogue, error) {
	if ctx == nil {
		return ac.ACPModelCatalogue{}, errors.New("acp_discovery_invalid")
	}
	select {
	case <-m.d.done:
	case <-ctx.Done():
		return ac.ACPModelCatalogue{}, errors.New("acp_discovery_unknown")
	}
	o := m.observation()
	if o.ErrorCode != "" || o.Catalogue == nil {
		return ac.ACPModelCatalogue{}, errors.New(o.ErrorCode)
	}
	return *o.Catalogue, nil
}
func (m *ModelDiscovery) Finish() DiscoveryObservation {
	m.d.mu.Lock()
	if !m.d.discovery.finalized && m.d.ctx.Err() != nil && m.d.bad == nil {
		m.d.bad = errors.New("acp_discovery_unknown")
	}
	m.d.discovery.finalized = true
	m.d.mu.Unlock()
	m.d.Finish()
	return m.observation()
}
func (m *ModelDiscovery) Close() error {
	m.d.mu.Lock()
	finalized := m.d.discovery.finalized
	m.d.mu.Unlock()
	if !finalized {
		m.d.fail("acp_discovery_unknown")
	}
	return m.d.Close()
}
