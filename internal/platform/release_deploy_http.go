package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func (s *Server) releaseDeploymentHTTP(w http.ResponseWriter, r *http.Request, p Principal, project, path string, m map[string]any) bool {
	parts := strings.Split(path, "/")
	ctx := r.Context()
	token := tokenOf(r)
	switch {
	case path == "release-providers" && r.Method == "GET":
		items, err := s.Service.ReleaseProviderOptions(ctx, p, project)
		respondList(w, items, err)
	case path == "environments" && r.Method == "GET":
		items, err := s.Service.ReleaseEnvironments(ctx, p, project)
		if err != nil {
			writeError(w, err)
			return true
		}
		actions, err := s.Service.ReleaseEnvironmentActions(ctx, p, project)
		respond(w, 200, map[string]any{"items": items, "total": len(items), "allowed_actions": actions}, err)
	case path == "environments" && r.Method == "POST":
		out, err := s.Service.SaveReleaseEnvironment(ctx, p, project, "", token, m)
		respond(w, 201, out, err)
	case len(parts) == 2 && parts[0] == "environments" && r.Method == "GET":
		out, err := s.Service.ReleaseEnvironment(ctx, p, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 2 && parts[0] == "environments" && r.Method == "PATCH":
		out, err := s.Service.SaveReleaseEnvironment(ctx, p, project, parts[1], token, m)
		respond(w, 200, out, err)
	case path == "release-plans/preview" && r.Method == "POST":
		out, err := s.Service.PreviewReleasePlan(ctx, p, project, token, m)
		respond(w, 200, out, err)
	case path == "release-plans" && r.Method == "POST":
		out, err := s.Service.CreateReleasePlan(ctx, p, project, token, m)
		respond(w, 201, out, err)
	case path == "release-plans" && r.Method == "GET":
		items, err := s.Service.ReleasePlans(ctx, p, project)
		respondList(w, items, err)
	case len(parts) == 2 && parts[0] == "release-plans" && r.Method == "GET":
		out, err := s.Service.ReleasePlan(ctx, p, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "release-plans" && parts[2] == "approve" && r.Method == "POST":
		out, err := s.Service.ApproveReleasePlan(ctx, p, project, parts[1], token, m)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "release-plans" && parts[2] == "execute" && r.Method == "POST":
		out, err := s.Service.ExecuteReleasePlan(ctx, p, project, parts[1], token, m, s.Engine)
		respond(w, 201, out, err)
	case path == "deployments" && r.Method == "GET":
		items, err := s.Service.ReleaseDeployments(ctx, p, project)
		respondList(w, items, err)
	case len(parts) == 2 && parts[0] == "deployments" && r.Method == "GET":
		out, err := s.Service.ReleaseDeployment(ctx, p, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "deployments" && parts[2] == "reconcile" && r.Method == "POST":
		out, err := s.Service.ReconcileReleaseDeployment(ctx, p, project, parts[1], token, m, s.Engine)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "deployments" && parts[2] == "rollback-preview" && r.Method == "POST":
		out, err := s.Service.RollbackReleasePreview(ctx, p, project, parts[1], token, m)
		respond(w, 200, out, err)
	case path == "runner/release-authorizations" && r.Method == "POST":
		var req rc.ReleaseAuthorizationRequest
		if err := releaseDecode(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.IssueReleaseAuthorization(ctx, p, token, req)
		respond(w, 200, out, err)
	default:
		return false
	}
	return true
}

// RejectDuplicateReleaseJSON is shared with the root HTTP decoder. A map would
// otherwise discard duplicate fields before strict typed decoding can reject them.
func RejectDuplicateReleaseJSON(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 128 {
			return invalid("请求 JSON 层级超过128")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return invalid("请求包含重复 JSON 字段")
				}
				seen[key] = true
				if err = value(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case '[':
			for d.More() {
				if err = value(depth + 1); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		default:
			return invalid("JSON 结构无效")
		}
	}
	if err := value(0); err != nil {
		return invalid("请求 JSON 无效、层级超限或存在重复字段")
	}
	if _, err := d.Token(); err != io.EOF {
		return invalid("请求包含多余 JSON 内容")
	}
	return nil
}
func releaseProviderBody(r *http.Request, out any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return invalid("提供方请求不可读或超限")
	}
	if err = RejectDuplicateReleaseJSON(b); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return invalid("提供方请求存在未知字段或错误类型")
	}
	return nil
}
func (s *Server) releaseProviderHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	if !strings.HasPrefix(path, "provider/") {
		return false
	}
	ctx := r.Context()
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(header) <= 7 {
		writeError(w, forbidden())
		return true
	}
	token := strings.TrimPrefix(header, "Bearer ")
	switch {
	case path == "provider/deployment-authorizations/inspect" && r.Method == "POST":
		var req rc.InspectDeploymentAuthorization
		if err := releaseProviderBody(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.InspectReleaseAuthorization(ctx, token, req)
		respond(w, 200, out, err)
	case path == "provider/deployment-authorizations/consume" && r.Method == "POST":
		var req rc.ConsumeDeploymentAuthorization
		if err := releaseProviderBody(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ConsumeReleaseAuthorization(ctx, token, req)
		respond(w, 200, out, err)
	case path == "provider/deployment-authorizations/begin-effect" && r.Method == "POST":
		var req rc.BeginDeploymentEffect
		if err := releaseProviderBody(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.BeginReleaseEffect(ctx, token, req)
		respond(w, 200, out, err)
	case path == "provider/deployment-observations" && r.Method == "POST":
		var req rc.DeploymentObservation
		if err := releaseProviderBody(r, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ObserveReleaseDeployment(ctx, token, req)
		respond(w, 200, out, err)
	case strings.HasPrefix(path, "provider/artifacts/") && r.Method == "GET":
		parts := strings.Split(path, "/")
		if len(parts) != 3 && !(len(parts) == 5 && parts[3] == "blobs") {
			writeError(w, missing())
			return true
		}
		operation := r.URL.Query().Get("operation_id")
		p, project, err := s.Service.ProviderArtifactAccess(ctx, token, operation, parts[2])
		if err != nil {
			writeError(w, err)
			return true
		}
		tx, err := s.Service.DB.BeginTx(ctx, s.Service.executionWriteOptions())
		if err != nil {
			writeError(w, err)
			return true
		}
		defer tx.Rollback()
		if err = s.Service.lockOrganization(ctx, tx, p.TenantID); err != nil {
			writeError(w, err)
			return true
		}
		if err = s.Service.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
			writeError(w, err)
			return true
		}
		d, err := s.Service.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, operation)
		if err == nil {
			_, err = s.Service.releaseDeploymentBasisTx(ctx, tx, p.TenantID, project, d)
		}
		if err != nil {
			writeError(w, err)
			return true
		}
		if len(parts) == 3 {
			out, e := s.Service.releaseArtifactTx(ctx, tx, p, project, parts[2])
			respond(w, 200, out, e)
		} else {
			out, e := s.Service.releaseArtifactBlobTx(ctx, tx, p, project, parts[2], parts[4])
			if e != nil {
				writeError(w, e)
				return true
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(200)
			_, _ = w.Write(out)
		}
	default:
		writeError(w, missing())
	}
	return true
}
