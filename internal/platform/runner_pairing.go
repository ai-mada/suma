package platform

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func jsonPairingReceipt(raw string, out *RunnerPairing) error {
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return err
	}
	if !desktopWorkspaceHex(out.ID, 32) || !handoffID(out.ProjectID, false) || out.Version < 1 || out.CanApprove || out.ReasonCode != "" {
		return runnerPairingError(409, "changed")
	}
	return nil
}

func runnerPairingError(status int, reason string) error {
	messages := map[string]string{
		"invalid":     "Invalid Runner pairing request",
		"changed":     "Runner pairing changed; refresh the original request",
		"expired":     "Runner pairing expired; create a new pairing",
		"unavailable": "A safe operator-controlled URL is required for Runner pairing",
		"limit":       "This project already has eight active Runner pairings",
		"rejected":    "Runner pairing was rejected",
	}
	return &APIError{status, "runner_pairing_" + reason, messages[reason], nil}
}

func runnerPairingControlURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || strings.TrimSpace(raw) != raw || strings.Contains(raw, "\\") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return "", runnerPairingError(503, "unavailable")
	}
	return strings.TrimRight(raw, "/"), nil
}

func runnerPairingSameControl(fixed, current string) bool {
	canonical, err := runnerPairingControlURL(current)
	return err == nil && fixed == canonical
}

func runnerPairingMachineInput(id, project, key, token string) bool {
	return desktopWorkspaceHex(id, 32) && handoffID(project, false) && planningKey(key) && len(token) >= 32 && len(token) <= 512 && strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0 && utf8.ValidString(token)
}

func runnerPairingTerminal(p RunnerPairing) error {
	if p.State == "expired" {
		return runnerPairingError(409, "expired")
	}
	if p.State == "rejected" {
		return runnerPairingError(409, "rejected")
	}
	return nil
}

func (s *Service) CreateRunnerPairing(parent context.Context, actor Actor, request RunnerPairingCreateRequest, control string) (RunnerPairing, error) {
	var out RunnerPairing
	if !handoffID(request.ProjectID, false) || !planningKey(request.IdempotencyKey) {
		return out, runnerPairingError(400, "invalid")
	}
	ctx, tx, close, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer close()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, request.ProjectID)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"runner_pairing_create": request})
	if err != nil {
		return out, err
	}
	if replay {
		var receipt RunnerPairing
		if err := jsonPairingReceipt(hash, &receipt); err != nil {
			return out, err
		}
		if receipt.ProjectID != request.ProjectID {
			return out, runnerPairingError(409, "changed")
		}
		return receipt, nil
	}
	canonical, err := runnerPairingControlURL(control)
	if err != nil {
		return out, err
	}
	var active int
	// authorizeTx already holds the current project row, serializing this cap
	// across administrators before either count or insert (SQLite writer guard).
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_runner_pairings WHERE tenant_id=? AND project_id=? AND state IN ('waiting','proposed') AND expires_at>?`), auth.target.tenantID, request.ProjectID, automationStamp(auth.checkedAt)).Scan(&active); err != nil {
		return out, err
	}
	if active >= 8 {
		return out, runnerPairingError(409, "limit")
	}
	out = RunnerPairing{ID: newID(), ProjectID: request.ProjectID, Version: 1, State: "waiting", ControlURL: canonical, CreatedAt: automationStamp(auth.checkedAt), ExpiresAt: automationStamp(auth.checkedAt.Add(10 * time.Minute))}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_runner_pairings(id,tenant_id,project_id,version,state,expires_at,proposal_operation_id,proposal_sha256,payload)VALUES(?,?,?,1,'waiting',?,'','',?)`), out.ID, auth.target.tenantID, out.ProjectID, out.ExpiresAt, stringify(out))
	if err != nil {
		return RunnerPairing{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return RunnerPairing{}, err
	}
	if err = s.commandActivity(ctx, auth, "runner_pairing.created", out.ID, "Remote Runner pairing prepared"); err != nil {
		return RunnerPairing{}, err
	}
	return out, tx.Commit()
}

func (s *Service) GetRunnerPairing(parent context.Context, actor Actor, project, id, control string) (RunnerPairing, error) {
	var out RunnerPairing
	if !desktopWorkspaceHex(id, 32) || !handoffID(project, false) {
		return out, runnerPairingError(400, "invalid")
	}
	ctx, tx, close, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer close()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, project)
	if err != nil {
		return out, err
	}
	record, err := s.runnerPairingTx(ctx, tx, id, project, auth.target.tenantID, true)
	if err != nil {
		return out, err
	}
	return runnerPairingView(record, auth.checkedAt, true, control)
}

func (s *Service) CommandRunnerPairing(parent context.Context, actor Actor, id string, request RunnerPairingCommandRequest, control string) (RunnerPairing, error) {
	var out RunnerPairing
	if !desktopWorkspaceHex(id, 32) || !handoffID(request.ProjectID, false) || !planningKey(request.IdempotencyKey) || request.ExpectedVersion < 1 || request.ExpectedVersion > 2147483645 || (request.Action != "approve" && request.Action != "reject") {
		return out, runnerPairingError(400, "invalid")
	}
	ctx, tx, close, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer close()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, request.ProjectID)
	if err != nil {
		return out, err
	}
	record, err := s.runnerPairingTx(ctx, tx, id, request.ProjectID, auth.target.tenantID, true)
	if err != nil {
		return out, err
	}
	if request.Action == "approve" && !runnerPairingSameControl(record.ControlURL, control) {
		return out, runnerPairingError(409, "changed")
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"runner_pairing_command": id, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		if err = jsonPairingReceipt(hash, &out); err != nil {
			return RunnerPairing{}, err
		}
		return out, nil
	}
	out, err = runnerPairingView(record, auth.checkedAt, true, control)
	if err != nil {
		return RunnerPairing{}, err
	}
	if err = runnerPairingTerminal(out); err != nil {
		return RunnerPairing{}, err
	}
	if record.Version != request.ExpectedVersion || record.Version >= 2147483646 || record.State == "approved" || (request.Action == "approve" && record.State != "proposed") {
		return RunnerPairing{}, runnerPairingError(409, "changed")
	}
	before := record.Version
	record.Version++
	record.CanApprove = false
	if request.Action == "reject" {
		record.State = "rejected"
	} else {
		record.State = "approved"
		record.WorkerID = "remote-" + newID()
		record.CredentialExpiresAt, err = s.issueRegisteredRunnerDigestTx(ctx, tx, auth, "remote-runner", "release-worker:"+record.WorkerID, request.IdempotencyKey, record.CredentialSHA256)
		if err != nil {
			return RunnerPairing{}, err
		}
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_workers(id,tenant_id,project_id,worker_id,token_hash,version,status,profiles,deployment,created_by,created_at,expires_at)VALUES(?,?,?,?,?,1,'active','[]',0,?,?,?)`), newID(), auth.target.tenantID, request.ProjectID, record.WorkerID, record.CredentialSHA256, auth.currentPrincipal.ID, automationStamp(auth.checkedAt), record.CredentialExpiresAt)
		if err != nil {
			return RunnerPairing{}, err
		}
	}
	if err = s.saveRunnerPairingTx(ctx, tx, record, before); err != nil {
		return RunnerPairing{}, err
	}
	out = record.RunnerPairing
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return RunnerPairing{}, err
	}
	activity := "runner_pairing.approved"
	if request.Action == "reject" {
		activity = "runner_pairing.rejected"
	}
	if err = s.commandActivity(ctx, auth, activity, id, "Remote Runner pairing "+record.State); err != nil {
		return RunnerPairing{}, err
	}
	return out, tx.Commit()
}

func (s *Service) ProposeRunnerPairing(ctx context.Context, id string, request RunnerPairingProposalRequest, token, control string) (RunnerPairing, error) {
	out, approved, err := s.proposeRunnerPairing(ctx, id, request, token, control)
	if err != nil || !approved {
		return out, err
	}
	// End the proposal row transaction before taking normal Runner authority
	// locks, preserving the shared authority -> project -> domain lock order.
	return s.RunnerPairingStatus(ctx, id, RunnerPairingStatusRequest{ProjectID: request.ProjectID, IdempotencyKey: request.IdempotencyKey}, token, control)
}

// This independent read is only an early rejection filter. It never upgrades a
// SQLite read transaction into a writer or replaces the locked checks below.
// Invalid public locators/proofs cannot wait for the global authority writer.
func (s *Service) runnerPairingMachinePrecheck(ctx context.Context, id, project, operation, token, control string, proposal *RunnerPairingProposalRequest) error {
	record, err := s.runnerPairingTx(ctx, s.DB, id, project, "", false)
	if err != nil {
		if proposal == nil {
			return forbidden()
		}
		return err
	}
	if proposal == nil {
		if record.proposalOperation != operation || subtle.ConstantTimeCompare([]byte(record.CredentialSHA256), []byte(digest(token))) != 1 {
			return forbidden()
		}
	} else if record.State != "waiting" {
		if record.proposalOperation != operation || record.proposalSHA != digest(stringify(*proposal)) || record.CredentialSHA256 != proposal.CredentialSHA256 {
			return runnerPairingError(409, "changed")
		}
	}
	if !runnerPairingSameControl(record.ControlURL, control) {
		return runnerPairingError(409, "changed")
	}
	clock, err := s.authorityNow(ctx, s.DB)
	if err != nil {
		return err
	}
	view, err := runnerPairingView(record, clock, false, control)
	if err != nil {
		return err
	}
	if err = runnerPairingTerminal(view); err != nil {
		return err
	}
	if proposal != nil && record.State == "waiting" {
		var existing int
		if err = s.DB.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_credential_authority WHERE token_hash=?`), proposal.CredentialSHA256).Scan(&existing); err != nil {
			return err
		}
		if existing != 0 {
			return runnerPairingError(409, "changed")
		}
	}
	return nil
}

func (s *Service) proposeRunnerPairing(parent context.Context, id string, request RunnerPairingProposalRequest, token, control string) (RunnerPairing, bool, error) {
	var out RunnerPairing
	if !runnerPairingMachineInput(id, request.ProjectID, request.IdempotencyKey, token) || !desktopWorkspaceHex(request.CredentialSHA256, 64) || !utf8.ValidString(request.DeviceName) || len(request.DeviceName) < 1 || len(request.DeviceName) > 100 || strings.TrimSpace(request.DeviceName) != request.DeviceName || strings.IndexFunc(request.DeviceName, unicode.IsControl) >= 0 {
		return out, false, runnerPairingError(400, "invalid")
	}
	if subtle.ConstantTimeCompare([]byte(digest(token)), []byte(request.CredentialSHA256)) != 1 {
		return out, false, forbidden()
	}
	precheck, cancel := context.WithTimeout(parent, authorityTransactionBudget)
	defer cancel()
	if err := s.runnerPairingMachinePrecheck(precheck, id, request.ProjectID, request.IdempotencyKey, token, control, &request); err != nil {
		return out, false, err
	}
	ctx, tx, close, err := s.beginAuthorityTx(precheck)
	if err != nil {
		return out, false, err
	}
	defer close()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return out, false, err
	}
	record, err := s.runnerPairingTx(ctx, tx, id, request.ProjectID, "", true)
	if err != nil {
		return out, false, err
	}
	if !runnerPairingSameControl(record.ControlURL, control) {
		return out, false, runnerPairingError(409, "changed")
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, false, err
	}
	out, err = runnerPairingView(record, clock, false, control)
	if err != nil {
		return out, false, err
	}
	if err = runnerPairingTerminal(out); err != nil {
		return RunnerPairing{}, false, err
	}
	sha := digest(stringify(request))
	if record.State != "waiting" {
		if record.proposalOperation != request.IdempotencyKey || record.proposalSHA != sha || record.CredentialSHA256 != request.CredentialSHA256 {
			return RunnerPairing{}, false, runnerPairingError(409, "changed")
		}
		return out, record.State == "approved", nil
	}
	var existing int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_credential_authority WHERE token_hash=?`), request.CredentialSHA256).Scan(&existing); err != nil {
		return RunnerPairing{}, false, err
	}
	if existing != 0 {
		return RunnerPairing{}, false, runnerPairingError(409, "changed")
	}
	before := record.Version
	record.Version++
	record.State = "proposed"
	record.DeviceName = request.DeviceName
	record.CredentialSHA256 = request.CredentialSHA256
	record.proposalOperation = request.IdempotencyKey
	record.proposalSHA = sha
	if err = s.saveRunnerPairingTx(ctx, tx, record, before); err != nil {
		return RunnerPairing{}, false, err
	}
	return record.RunnerPairing, false, tx.Commit()
}

func (s *Service) RunnerPairingStatus(parent context.Context, id string, request RunnerPairingStatusRequest, token, control string) (RunnerPairing, error) {
	var out RunnerPairing
	if !runnerPairingMachineInput(id, request.ProjectID, request.IdempotencyKey, token) {
		return out, runnerPairingError(400, "invalid")
	}
	precheck, cancel := context.WithTimeout(parent, authorityTransactionBudget)
	defer cancel()
	if err := s.runnerPairingMachinePrecheck(precheck, id, request.ProjectID, request.IdempotencyKey, token, control, nil); err != nil {
		return out, err
	}
	ctx, tx, close, err := s.beginAuthorityTx(precheck)
	if err != nil {
		return out, err
	}
	defer close()
	defer tx.Rollback()
	// Approved reads enter the original current-authority protocol below. On
	// SQLite acquire its writer serialization before the candidate snapshot.
	if err = s.authorityWriter(ctx, tx); err != nil {
		return out, err
	}
	record, err := s.runnerPairingTx(ctx, tx, id, request.ProjectID, "", false)
	if err != nil {
		return out, forbidden()
	}
	if record.proposalOperation != request.IdempotencyKey || subtle.ConstantTimeCompare([]byte(record.CredentialSHA256), []byte(digest(token))) != 1 {
		return out, forbidden()
	}
	if !runnerPairingSameControl(record.ControlURL, control) {
		return out, runnerPairingError(409, "changed")
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	out, err = runnerPairingView(record, clock, false, control)
	if err != nil {
		return RunnerPairing{}, err
	}
	if err = runnerPairingTerminal(out); err != nil {
		return RunnerPairing{}, err
	}
	if record.State == "approved" {
		actor, err := s.runnerActorByHashTx(ctx, tx, record.CredentialSHA256)
		if err != nil {
			return RunnerPairing{}, err
		}
		auth, err := s.runnerIdentityTx(ctx, tx, actor, request.ProjectID, record.WorkerID)
		if err != nil {
			return RunnerPairing{}, err
		}
		current, err := s.runnerPairingTx(ctx, tx, id, request.ProjectID, auth.target.tenantID, true)
		if err != nil {
			return RunnerPairing{}, err
		}
		if current.State != "approved" || current.WorkerID != record.WorkerID || current.CredentialSHA256 != record.CredentialSHA256 || current.proposalOperation != request.IdempotencyKey {
			return RunnerPairing{}, runnerPairingError(409, "changed")
		}
		return runnerPairingView(current, auth.checkedAt, false, control)
	}
	return out, nil
}
