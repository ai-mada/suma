package platform

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type desktopWorkspaceRootProof struct {
	Root, Identity, RegisteredBy, OperationID, Signature, CreatedAt string
}

type workspaceBindingState struct {
	root      string
	bindingID string
	sha256    string
	proof     *desktopWorkspaceRootProof
}

func (s *Service) signDesktopWorkspaceRoot(tenant, project string, proof desktopWorkspaceRootProof) string {
	// This separate domain prevents a short-lived selection MAC from ever
	// becoming a persistent root proof, or a proof crossing project boundaries.
	return desktopWorkspaceMAC(s.DesktopWorkspaceKey, "suma-desktop-workspace-root-v1", tenant, project, proof.Root, proof.Identity, proof.RegisteredBy, proof.OperationID, proof.CreatedAt)
}

func (s *Service) validateDesktopWorkspaceRootProof(tenant, project string, state workspaceBindingState) error {
	proof := state.proof
	if proof == nil || !s.desktopWorkspaceAvailable() || proof.Root != state.root || proof.Identity == "" || !desktopWorkspaceHex(proof.RegisteredBy, 32) || !desktopWorkspaceOperation(proof.OperationID) || !desktopWorkspaceHex(proof.Signature, 64) || !hmac.Equal([]byte(proof.Signature), []byte(s.signDesktopWorkspaceRoot(tenant, project, *proof))) {
		return desktopWorkspaceRootInvalid()
	}
	return nil
}

func (s *Service) workspaceBindingStateTx(ctx context.Context, tx *sql.Tx, tenant, project string) (workspaceBindingState, error) {
	var state workspaceBindingState
	err := tx.QueryRowContext(ctx, s.q(`SELECT root_path,binding_id FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), tenant, project).Scan(&state.root, &state.bindingID)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	proof := &desktopWorkspaceRootProof{}
	err = tx.QueryRowContext(ctx, s.q(`SELECT root_path,root_identity,registered_by,operation_id,signature,created_at FROM suma_desktop_workspace_roots WHERE tenant_id=? AND project_id=?`), tenant, project).Scan(&proof.Root, &proof.Identity, &proof.RegisteredBy, &proof.OperationID, &proof.Signature, &proof.CreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	state.sha256 = workspaceBindingDigest(state)
	if err == nil {
		state.proof = proof
		state.sha256 = workspaceBindingDigest(state)
	}
	return state, nil
}

func workspaceBindingDigest(state workspaceBindingState) string {
	signature := ""
	if state.proof != nil {
		signature = state.proof.Signature
	}
	return digest("suma-workspace-binding-v1\x00" + stringify([]string{state.bindingID, state.root, signature}))
}

func (s *Service) validateWorkspaceBindingRoot(ctx context.Context, tenant, project string, state workspaceBindingState) error {
	if state.root == "" {
		if strings.TrimSpace(os.Getenv("SUMA_WORKSPACE_ROOT")) == "" && !s.desktopWorkspaceAvailable() {
			return errWorkspaceDisabled
		}
		return &APIError{422, "workspace_not_configured", "请先配置当前项目的工作区", nil}
	}
	if !desktopWorkspaceHex(state.bindingID, 32) {
		return forbidden()
	}
	if proof := state.proof; proof != nil {
		if err := s.validateDesktopWorkspaceRootProof(tenant, project, state); err != nil {
			return err
		}
		identity, err := desktopWorkspaceRootIdentity(ctx, state.root)
		if err != nil {
			return err
		}
		if proof.Identity != identity {
			return desktopWorkspaceRootChanged()
		}
		return nil
	}
	// An explicitly configured root remains a separate current source. It is
	// considered only when no native proof exists for this project binding.
	allowed, err := allowedWorkspaceRoot()
	if err != nil {
		return forbidden()
	}
	resolved, err := filepath.EvalSymlinks(state.root)
	if err != nil {
		return invalid("工作区目录不可用")
	}
	if !workspaceTreeSamePath(state.root, resolved) || !under(allowed, resolved) {
		return forbidden()
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return invalid("工作区目录不可用")
	}
	return ctx.Err()
}

// A transaction-bound caller supplies the current project authority; this
// function supplies the current root permission for reads, saves and recovery.
func (s *Service) workspaceRootAuthorityTx(ctx context.Context, tx *sql.Tx, tenant, project, expected string) (string, error) {
	state, err := s.workspaceBindingStateTx(ctx, tx, tenant, project)
	if err != nil {
		return "", err
	}
	if expected != "" && state.root != expected {
		return "", conflict("工作区绑定已变化，请重新加载")
	}
	if err = s.validateWorkspaceBindingRoot(ctx, tenant, project, state); err != nil {
		return "", err
	}
	return state.root, nil
}
