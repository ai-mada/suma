package platform

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func artifactCoreAssignment(a cc.AssignmentRef) rc.AssignmentRef {
	return rc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
}

// An artifact is a new data transfer. An older fact callback may drain,
// while a release-eligible artifact requires the current approved grant.
func (s *Service) poolArtifactGrantActiveTx(ctx context.Context, tx *sql.Tx, a dag.Assignment) error {
	var status string
	if err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status FROM suma_execution_pools WHERE id=?`)), a.PoolID).Scan(&status); err != nil {
		return err
	}
	if status != "active" {
		return forbidden()
	}
	scope, err := s.currentPoolActiveScopeTx(ctx, tx, a.PoolID, a.TenantID, a.ProjectID)
	if err != nil {
		return err
	}
	if scope.GrantID != a.PoolGrantID || scope.GrantVersion != a.PoolGrantVersion {
		return forbidden()
	}
	return nil
}

func poolArtifactNodeKind(n dag.Node, p ToolProfile) (string, error) {
	basis, err := rc.DecodeApplicationBuild(n.Config)
	if err != nil {
		return "", invalid("固定应用构建配置无效")
	}
	from, hasInput := n.Config["artifact_from"]
	if basis == nil && !hasInput && p.Profile.ArtifactKind == "" && p.Profile.ArtifactProfileSHA256 == "" {
		return "", nil
	}
	var expected string
	switch {
	case basis != nil && !hasInput && n.Type == "command":
		expected = "build"
	case basis == nil && hasInput && n.Type == "test":
		source, ok := from.(string)
		if ok && source != "" && source != n.ID {
			expected = "package_test"
		}
	}
	id, sha, bound, err := dag.ToolBinding(n)
	if err != nil || expected == "" || !bound || len(n.Command) != 1 || p.ID != id || p.Profile.ProfileSHA256 != sha ||
		p.Profile.CommandID != n.Command[0] || p.Profile.ArtifactKind != expected || !rc.ValidDigest(p.Profile.ArtifactProfileSHA256) {
		return "", invalid("资源池制品节点需要固定并获批的构建或包测试档案")
	}
	return expected, nil
}

func (s *Service) poolArtifactAuthorityTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, a cc.AssignmentRef, kind string) (*candidateAuthority, ToolProfile, error) {
	if !subject.pool {
		return nil, ToolProfile{}, forbidden()
	}
	proof, err := s.candidateAuthorityTx(ctx, tx, subject, a)
	if err != nil {
		return nil, ToolProfile{}, err
	}
	id, _, bound, err := dag.ToolBinding(proof.spec)
	if err != nil || !bound {
		return nil, ToolProfile{}, forbidden()
	}
	p := Principal{TenantID: subject.tenant}
	profile, err := s.toolProfileTx(ctx, tx, p, subject.project, id)
	if err != nil {
		return nil, ToolProfile{}, err
	}
	scope := dag.PoolScope{PoolID: a.PoolID, TenantID: a.TenantID, ProjectID: a.ProjectID, GrantID: a.PoolGrantID, GrantVersion: a.PoolGrantVersion}
	profile, err = s.toolProfileForPoolTx(ctx, tx, proof.machine, scope, profile)
	if err != nil {
		return nil, ToolProfile{}, err
	}
	actual, err := poolArtifactNodeKind(proof.spec, profile)
	if err != nil || actual != kind || !profile.Available {
		return nil, ToolProfile{}, forbidden()
	}
	if err = s.checkPoolTestPlanArtifactProfileTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID, profile, scope); err != nil {
		return nil, ToolProfile{}, err
	}
	basis, err := rc.DecodeApplicationBuild(proof.spec.Config)
	if err != nil {
		return nil, ToolProfile{}, err
	}
	if err = s.checkApplicationBuildBasisTx(ctx, tx, p, subject.project, basis); err != nil {
		return nil, ToolProfile{}, err
	}
	return proof, profile, nil
}

func (s *Server) poolArtifactHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "pool-runner" || (parts[1] != "artifact-uploads" && parts[1] != "artifacts") {
		return false
	}
	header := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		writeError(w, &APIError{401, "pool_runner_token_required", "资源池执行节点需要独立 Bearer 凭证", nil})
		return true
	}
	a, err := candidateAssignmentHeader(r)
	if err != nil || a.ProjectID == "" || a.TenantID == "" || a.PoolRunnerID == "" {
		writeError(w, forbidden())
		return true
	}
	subject := candidateSubject{pool: true, token: token, tenant: a.TenantID, project: a.ProjectID}
	w.Header().Set("Cache-Control", "no-store")
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.Method == http.MethodPost && len(query) != 0 ||
		r.Method != http.MethodPost && (len(query) != 1 || len(query["project_id"]) != 1 || query.Get("project_id") != a.ProjectID) {
		writeError(w, forbidden())
		return true
	}
	ctx := r.Context()
	var body map[string]any
	if r.Method == http.MethodPost {
		body, err = candidateJSONRequest(r)
		if err != nil {
			writeError(w, err)
			return true
		}
	}
	match := func(project string, ref rc.AssignmentRef) bool {
		return project == a.ProjectID && ref == artifactCoreAssignment(a)
	}
	switch {
	case r.Method == http.MethodPost && path == "pool-runner/artifact-uploads":
		var req rc.ArtifactUploadRequest
		if err = releaseDecode(body, &req); err != nil {
			writeError(w, err)
			break
		}
		if !match(req.ProjectID, req.Assignment) {
			writeError(w, forbidden())
			break
		}
		out, err := s.Service.startPoolArtifactUpload(ctx, subject, a, req)
		respond(w, 201, out, err)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "artifact-uploads":
		out, err := s.Service.inspectPoolArtifactUpload(ctx, subject, a, parts[2])
		respond(w, 200, out, err)
	case r.Method == http.MethodPut && len(parts) == 5 && parts[1] == "artifact-uploads" && parts[3] == "blobs":
		err := s.Service.putPoolArtifactBlob(ctx, subject, a, parts[2], parts[4], r.Header.Get("X-Suma-Operation-ID"), r.ContentLength, r.Body)
		respond(w, 200, map[string]bool{"acknowledged": err == nil}, err)
	case r.Method == http.MethodPost && len(parts) == 4 && parts[1] == "artifact-uploads" && parts[3] == "seal":
		var req rc.ArtifactSealRequest
		if err = releaseDecode(body, &req); err != nil {
			writeError(w, err)
			break
		}
		if !match(req.ProjectID, req.Assignment) {
			writeError(w, forbidden())
			break
		}
		out, err := s.Service.sealPoolArtifactUpload(ctx, subject, a, parts[2], req)
		respond(w, 200, out, err)
	case r.Method == http.MethodPost && path == "pool-runner/artifacts/resolve":
		var req rc.ArtifactResolveRequest
		if err = releaseDecode(body, &req); err != nil {
			writeError(w, err)
			break
		}
		if !match(req.ProjectID, req.Assignment) {
			writeError(w, forbidden())
			break
		}
		out, err := s.Service.resolvePoolArtifactInput(ctx, subject, a, req)
		respond(w, 200, out, err)
	case r.Method == http.MethodGet && len(parts) == 5 && parts[1] == "artifacts" && parts[3] == "blobs":
		b, err := s.Service.poolArtifactBlob(ctx, subject, a, parts[2], parts[4])
		if err != nil {
			writeError(w, err)
			break
		}
		artifactBytes(w, b)
	default:
		writeError(w, missing())
	}
	return true
}
