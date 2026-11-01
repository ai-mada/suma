package acpstdio

import (
	"context"
	"errors"
	"strings"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	sdk "github.com/coder/acp-go-sdk"
)

type discoveryState struct {
	catalogue                            *ac.ACPModelCatalogue
	evidence                             ac.ACPModelDiscoveryEvidence
	closeAdvertised, complete, finalized bool
}

// Only the exact raw canonical advertisement permits a CloseSession call;
// the SDK's case-insensitive struct decoding is never the authority for it.
func discoveryCloseAdvertised(raw []byte) bool {
	caps := object(raw)
	sessions := object(caps["sessionCapabilities"])
	return closed(object(sessions["close"]), "_meta")
}
func (d *Driver) runModelDiscovery() {
	defer close(d.done)
	defer close(d.outEnd)
	initialized, err := d.conn.Initialize(d.ctx, sdk.InitializeRequest{ProtocolVersion: 1, ClientInfo: &sdk.Implementation{Name: "suma-runner", Version: "1"}, ClientCapabilities: sdk.ClientCapabilities{}})
	if err != nil || initialized.ProtocolVersion != 1 {
		d.fail("acp_discovery_unknown")
		return
	}
	d.mu.Lock()
	if initialized.AgentInfo != nil {
		d.result.Evidence.AgentName, _ = safeText(initialized.AgentInfo.Name, d.options.Secrets)
		d.result.Evidence.AgentVersion, _ = safeText(initialized.AgentInfo.Version, d.options.Secrets)
	}
	validInfo := initialized.AgentInfo == nil && d.options.Runtime == "cursor" || text(d.result.Evidence.AgentName, 256, false) && text(d.result.Evidence.AgentVersion, 256, false)
	if !validInfo {
		d.result.Evidence.AgentName = ""
		d.result.Evidence.AgentVersion = ""
	}
	d.mu.Unlock()
	if !validInfo {
		d.fail("acp_discovery_invalid")
		return
	}
	session, err := d.conn.NewSession(d.ctx, sdk.NewSessionRequest{Cwd: d.options.Cwd, McpServers: []sdk.McpServer{}})
	if err != nil {
		d.fail("acp_discovery_unknown")
		return
	}
	catalogue, err := discoveryCatalogue(session.ConfigOptions, d.options.Secrets)
	if err != nil {
		d.fail("acp_discovery_model_unavailable")
		return
	}
	d.mu.Lock()
	closeSession := d.discovery.closeAdvertised
	d.mu.Unlock()
	if closeSession {
		ctx, cancel := context.WithTimeout(d.ctx, 1500*time.Millisecond)
		_, err = d.conn.CloseSession(ctx, sdk.CloseSessionRequest{SessionId: session.SessionId})
		cancel()
		if err != nil {
			d.fail("acp_discovery_unknown")
			return
		}
	}
	// Unexpected requests are actually refused by the SDK callbacks. Let their
	// already-generated replies reach stdin before closing this protocol stack.
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		d.mu.Lock()
		pending := false
		for _, p := range d.remote {
			pending = pending || !p.decided
		}
		if !pending {
			if d.discovery.evidence.UnexpectedActivity != 0 {
				d.bad = errors.New("acp_discovery_unexpected_activity")
			} else if d.bad == nil {
				d.discovery.catalogue = &catalogue
				d.discovery.complete = true
			}
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()
		select {
		case <-tick.C:
		case <-timer.C:
			d.fail("acp_discovery_unknown")
			return
		case <-d.ctx.Done():
			return
		}
	}
}
func discoveryCatalogue(options []sdk.SessionConfigOption, secrets []string) (ac.ACPModelCatalogue, error) {
	o, values, err := modelSelection(options)
	if err != nil {
		return ac.ACPModelCatalogue{}, err
	}
	c := ac.ACPModelCatalogue{Source: "acp_session_config", ProtocolVersion: 1, ConfigID: string(o.Id), CurrentValue: string(o.CurrentValue), Models: []ac.ACPModelChoice{}}
	if hasSecret(c.ConfigID, secrets) || hasSecret(string(o.CurrentValue), secrets) {
		return ac.ACPModelCatalogue{}, errors.New("acp_discovery_invalid")
	}
	for _, v := range values {
		id := string(v.Value)
		if hasSecret(id, secrets) {
			return ac.ACPModelCatalogue{}, errors.New("acp_discovery_invalid")
		}
		name, _ := safeText(v.Name, secrets)
		c.Models = append(c.Models, ac.ACPModelChoice{ID: id, Name: name})
	}
	if ac.ValidateModelCatalogue(c) != nil {
		return ac.ACPModelCatalogue{}, errors.New("acp_discovery_invalid")
	}
	return c, nil
}
func (m *ModelDiscovery) observation() DiscoveryObservation {
	d := m.d
	d.mu.Lock()
	defer d.mu.Unlock()
	o := DiscoveryObservation{Evidence: d.discovery.evidence, AgentName: d.result.Evidence.AgentName, AgentVersion: d.result.Evidence.AgentVersion}
	if d.discovery.evidence.UnexpectedActivity != 0 {
		o.ErrorCode = "acp_discovery_unexpected_activity"
	} else if d.bad != nil {
		code := d.bad.Error()
		switch code {
		case "acp_discovery_invalid", "acp_discovery_unknown", "acp_discovery_model_unavailable", "acp_discovery_unexpected_activity":
			o.ErrorCode = code
		default:
			if strings.Contains(code, "invalid") || strings.Contains(code, "frame") {
				o.ErrorCode = "acp_discovery_invalid"
			} else {
				o.ErrorCode = "acp_discovery_unknown"
			}
		}
	} else if !d.discovery.complete || d.discovery.catalogue == nil {
		o.ErrorCode = "acp_discovery_unknown"
	}
	if o.ErrorCode == "" {
		c := *d.discovery.catalogue
		c.Models = append([]ac.ACPModelChoice{}, c.Models...)
		o.Catalogue = &c
	}
	return o
}
