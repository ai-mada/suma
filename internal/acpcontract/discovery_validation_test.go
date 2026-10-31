package acpcontract

import (
	"strings"
	"testing"
)

func discoveryCatalogue() ACPModelCatalogue {
	return ACPModelCatalogue{Source: "acp_session_config", ProtocolVersion: 1, ConfigID: "model", CurrentValue: "lite", Models: []ACPModelChoice{{ID: "lite", Name: "Lite"}, {ID: "performance", Name: "Performance"}}}
}
func TestModelCatalogueValidation(t *testing.T) {
	c := discoveryCatalogue()
	if ValidateModelCatalogue(c) != nil {
		t.Fatal("valid exact catalogue rejected")
	}
	c.CurrentValue = ""
	if ValidateModelCatalogue(c) != nil {
		t.Fatal("unverified current value must permit explicit listed choice")
	}
	c.CurrentValue = "not-listed"
	if ValidateModelCatalogue(c) != nil {
		t.Fatal("safe exact unlisted current value must remain an unverified observation")
	}
	for _, mutate := range []func(*ACPModelCatalogue){func(c *ACPModelCatalogue) { c.Source = "default" }, func(c *ACPModelCatalogue) { c.ConfigID = "" }, func(c *ACPModelCatalogue) { c.Models = nil }, func(c *ACPModelCatalogue) { c.Models = append(c.Models, c.Models[0]) }, func(c *ACPModelCatalogue) { c.CurrentValue = " invalid " }, func(c *ACPModelCatalogue) { c.Models[0].ConfigurationSHA256 = "invalid" }} {
		c = discoveryCatalogue()
		mutate(&c)
		if ValidateModelCatalogue(c) == nil {
			t.Error("invalid catalogue accepted")
		}
	}
}
func TestModelDiscoveryResultRequiresActualStop(t *testing.T) {
	c := discoveryCatalogue()
	r := ACPModelDiscoveryResult{Protocol: ModelDiscoveryProtocol, Runtime: "qoder", State: "ready", ExecutableSHA256: strings.Repeat("a", 64), ToolVersion: "Qoder fixture", Catalogue: &c, Evidence: ACPModelDiscoveryEvidence{SessionCreated: true, HostProcessStopped: true, HostProcessTreeStopped: true}}
	if ValidateModelDiscoveryResult(r) != nil {
		t.Fatal("actual stopped ready result rejected")
	}
	for _, mutate := range []func(*ACPModelDiscoveryResult){func(r *ACPModelDiscoveryResult) { r.Evidence.HostProcessStopped = false }, func(r *ACPModelDiscoveryResult) { r.Evidence.HostProcessTreeStopped = false }, func(r *ACPModelDiscoveryResult) { r.Evidence.PromptRequests = 1 }, func(r *ACPModelDiscoveryResult) { r.Evidence.ConfigurationWrites = 1 }, func(r *ACPModelDiscoveryResult) { r.Evidence.PermissionRequests = 1 }, func(r *ACPModelDiscoveryResult) { r.Evidence.UnexpectedActivity = 1 }, func(r *ACPModelDiscoveryResult) { r.Evidence.SessionCloseRequested = true }, func(r *ACPModelDiscoveryResult) { r.ErrorCode = "arbitrary" }} {
		x := r
		mutate(&x)
		if ValidateModelDiscoveryResult(x) == nil {
			t.Error("unsafe ready result accepted")
		}
	}
	r.State = "unknown"
	r.Catalogue = nil
	r.ErrorCode = "acp_discovery_unknown"
	r.Evidence = ACPModelDiscoveryEvidence{}
	if ValidateModelDiscoveryResult(r) != nil {
		t.Fatal("honest unknown observation rejected")
	}
	r.Evidence.PermissionRequests = 33
	if ValidateModelDiscoveryResult(r) != nil {
		t.Fatal("unknown could not report a permission collection limit violation")
	}
	r.Catalogue = &c
	if ValidateModelDiscoveryResult(r) == nil {
		t.Fatal("unknown exposed adoptable catalogue")
	}
}
