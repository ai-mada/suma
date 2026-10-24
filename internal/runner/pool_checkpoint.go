package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type poolClaimCheckpoint struct {
	SchemaVersion     int              `json:"schema_version"`
	ControlSHA256     string           `json:"control_sha256"`
	ConfigSHA256      string           `json:"config_sha256"`
	CredentialVersion int              `json:"credential_version"`
	Request           PoolClaimRequest `json:"request"`
	Responded         bool             `json:"responded"`
	Complete          bool             `json:"complete"`
	Assignment        *Assignment      `json:"assignment,omitempty"`
}

func poolClaimPath(j *Journal) string { return filepath.Join(filepath.Dir(j.path), "pool-claim.json") }
func loadPoolClaim(j *Journal) (*poolClaimCheckpoint, error) {
	p := poolClaimPath(j)
	st, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 2<<20 {
		return nil, fmt.Errorf("pool claim checkpoint is not a bounded regular file")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var c poolClaimCheckpoint
	if err = json.Unmarshal(b, &c); err != nil || c.SchemaVersion != 1 || len(c.Request.OperationID) != 32 || c.Complete && !c.Responded {
		return nil, fmt.Errorf("invalid pool claim checkpoint")
	}
	if c.Assignment != nil {
		if err = validPoolAssignment(*c.Assignment); err != nil {
			return nil, err
		}
	}
	return &c, nil
}
func savePoolClaim(j *Journal, c *poolClaimCheckpoint) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return writeEvidence(poolClaimPath(j), b)
}
func beginPoolClaim(j *Journal, control, configSHA string, credentialVersion int, req PoolClaimRequest) (*poolClaimCheckpoint, error) {
	old, err := loadPoolClaim(j)
	if err != nil {
		return nil, err
	}
	controlSHA := digest([]byte(strings.TrimRight(control, "/")))
	if old != nil && !old.Complete {
		if old.ControlSHA256 != controlSHA || old.ConfigSHA256 != configSHA || old.CredentialVersion != credentialVersion || old.Request.PoolID != req.PoolID || old.Request.RunnerID != req.RunnerID || old.Request.WorkerID != req.WorkerID || old.Request.SlotID != req.SlotID {
			return nil, fmt.Errorf("pending pool claim requires its original control, credential, slot and local configuration")
		}
		return old, nil
	}
	req.OperationID = operationID()
	req.Scopes = append([]PoolCapabilityScope{}, req.Scopes...)
	for i := range req.Scopes {
		req.Scopes[i].Capabilities = append([]string{}, req.Scopes[i].Capabilities...)
		sort.Strings(req.Scopes[i].Capabilities)
	}
	sort.Slice(req.Scopes, func(i, k int) bool {
		return req.Scopes[i].TenantID+"/"+req.Scopes[i].ProjectID < req.Scopes[k].TenantID+"/"+req.Scopes[k].ProjectID
	})
	c := &poolClaimCheckpoint{SchemaVersion: 1, ControlSHA256: controlSHA, ConfigSHA256: configSHA, CredentialVersion: credentialVersion, Request: req}
	return c, savePoolClaim(j, c)
}
