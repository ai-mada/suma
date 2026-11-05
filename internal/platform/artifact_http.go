package platform

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

func artifactBytes(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
func (s *Server) artifactHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {

	parts := strings.Split(path, "/")
	ctx := r.Context()
	if len(parts) >= 2 && parts[0] == "runner" && (parts[1] == "artifact-uploads" || parts[1] == "artifacts") {
		kind, bound := actor.kind, actor.credential.projectID
		if kind != actorProjectRunner || bound != project || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeError(w, forbidden())
			return true
		}
		if path == "runner/artifact-uploads" && r.Method == "POST" {
			var req rc.ArtifactUploadRequest
			if e := releaseDecode(m, &req); e != nil {
				writeError(w, e)
			} else {
				out, e := s.Service.StartArtifactUpload(ctx, actor, project, req)
				respond(w, 201, out, e)
			}
			return true
		}
		if len(parts) == 3 && parts[1] == "artifact-uploads" && r.Method == "GET" {
			out, e := s.Service.InspectArtifactUpload(ctx, actor, project, parts[2])
			respond(w, 200, out, e)
			return true
		}
		if len(parts) == 5 && parts[1] == "artifact-uploads" && parts[3] == "blobs" && r.Method == "PUT" {
			e := s.Service.PutArtifactBlob(ctx, actor, project, parts[2], parts[4], r.Header.Get("X-Suma-Operation-ID"), r.ContentLength, r.Body)
			respond(w, 200, map[string]bool{"acknowledged": e == nil}, e)
			return true
		}
		if len(parts) == 4 && parts[1] == "artifact-uploads" && parts[3] == "seal" && r.Method == "POST" {
			var req rc.ArtifactSealRequest
			if e := releaseDecode(m, &req); e != nil {
				writeError(w, e)
			} else {
				out, e := s.Service.SealArtifactUpload(ctx, actor, project, parts[2], req)
				respond(w, 200, out, e)
			}
			return true
		}
		if path == "runner/artifacts/resolve" && r.Method == "POST" {
			var req rc.ArtifactResolveRequest
			if e := releaseDecode(m, &req); e != nil {
				writeError(w, e)
			} else {
				out, e := s.Service.ResolveArtifactInput(ctx, actor, project, req)
				respond(w, 200, out, e)
			}
			return true
		}
		if len(parts) == 5 && parts[1] == "artifacts" && parts[3] == "blobs" && r.Method == "GET" {
			var a rc.AssignmentRef
			raw, e := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Suma-Assignment"))
			if e != nil || len(raw) > 2048 || json.Unmarshal(raw, &a) != nil || !a.Valid() {
				writeError(w, invalid("下载需携带当前测试执行身份"))
				return true
			}
			b, e := s.Service.RunnerArtifactBlob(ctx, actor, project, parts[2], parts[4], a)
			if e != nil {
				writeError(w, e)
			} else {
				artifactBytes(w, b)
			}
			return true
		}
		writeError(w, missing())
		return true
	}
	if parts[0] == "release-workers" {
		if len(parts) == 1 && r.Method == "GET" {
			out, e := s.Service.ReleaseWorkers(ctx, actor, project)
			respondList(w, out, e)
			return true
		}
		if len(parts) == 1 && r.Method == "POST" {
			out, e := s.Service.RegisterReleaseWorker(ctx, actor, project, m)
			respond(w, 201, out, e)
			return true
		}
		if len(parts) == 3 && parts[2] == "artifact-uploads" && r.Method == http.MethodGet &&
			r.URL.Path == "/api/v1/release-workers/"+parts[1]+"/artifact-uploads" {
			w.Header().Set("Cache-Control", "no-store")
			query, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil || len(query["project_id"]) != 1 || query.Get("project_id") != project ||
				len(query["cursor"]) > 1 || query.Has("cursor") && query.Get("cursor") == "" ||
				len(query) > 1 && (!query.Has("cursor") || len(query) != 2) {
				writeError(w, invalid("无效的执行节点上传核对请求"))
				return true
			}
			out, err := s.Service.ListReleaseWorkerUploadRecovery(ctx, actor, project, parts[1], query.Get("cursor"))
			respond(w, http.StatusOK, out, err)
			return true
		}
		if len(parts) == 5 && parts[2] == "artifact-uploads" && parts[4] == "abandon" && r.Method == http.MethodPost && manualWorkerUploadAbandonAPIPath(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
			var request ReleaseWorkerUploadAbandonRequest
			body, err := json.Marshal(m)
			if r.URL.RawQuery != "" || err != nil || strictjson.Decode(body, &request, manualRunnerRotationBodyLimit) != nil {
				writeError(w, invalid("无效的执行节点上传核对请求"))
				return true
			}
			out, err := s.Service.AbandonReleaseWorkerUpload(ctx, actor, project, parts[1], parts[3], request)
			respond(w, http.StatusOK, out, err)
			return true
		}
		if len(parts) == 3 && parts[2] == "rotate-credential" && r.Method == http.MethodPost && manualRunnerRotationAPIPath(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-store")
			var request ReleaseWorkerRotationRequest
			body, err := json.Marshal(m)
			if r.URL.RawQuery != "" || err != nil || strictjson.Decode(body, &request, manualRunnerRotationBodyLimit) != nil {
				writeError(w, invalid("执行节点凭证轮换请求无效"))
				return true
			}
			out, err := s.Service.RotateReleaseWorkerCredential(ctx, actor, project, parts[1], request)
			respond(w, 200, out, err)
			return true
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			out, e := s.Service.RevokeReleaseWorker(ctx, actor, project, parts[1], m)
			respond(w, 200, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cursor := r.URL.Query().Get("cursor")
	if parts[0] == "artifacts" {
		if len(parts) == 1 && r.Method == "GET" {
			out, total, next, e := s.Service.ReleaseArtifacts(ctx, actor, project, limit, cursor)
			respond(w, 200, map[string]any{"items": out, "total": total, "next_cursor": next}, e)
			return true
		}
		if len(parts) == 2 && r.Method == "GET" {
			out, e := s.Service.ReleaseArtifact(ctx, actor, project, parts[1])
			respond(w, 200, out, e)
			return true
		}
		if len(parts) == 4 && parts[2] == "blobs" && r.Method == "GET" {
			b, e := s.Service.ReadArtifactBlob(ctx, actor, project, parts[1], parts[3])
			if e != nil {
				writeError(w, e)
			} else {
				artifactBytes(w, b)
			}
			return true
		}
		writeError(w, missing())
		return true
	}
	if parts[0] == "release-candidates" {
		if len(parts) == 1 && r.Method == "GET" {
			out, total, next, e := s.Service.ReleaseCandidates(ctx, actor, project, limit, cursor)
			respond(w, 200, map[string]any{"items": out, "total": total, "next_cursor": next}, e)
			return true
		}
		if len(parts) == 1 && r.Method == "POST" {
			out, e := s.Service.CreateReleaseCandidate(ctx, actor, project, m)
			respond(w, 201, out, e)
			return true
		}
		if len(parts) == 2 && r.Method == "GET" {
			out, e := s.Service.ReleaseCandidate(ctx, actor, project, parts[1])
			respond(w, 200, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	return false
}
