package platform

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func artifactBytes(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
func (s *Server) artifactHTTP(w http.ResponseWriter, r *http.Request, p Principal, project, path string, m map[string]any) bool {
	parts := strings.Split(path, "/")
	ctx := r.Context()
	token := tokenOf(r)
	if len(parts) >= 2 && parts[0] == "runner" && (parts[1] == "artifact-uploads" || parts[1] == "artifacts") {
		kind, bound := s.Service.TokenScope(ctx, token)
		if kind != "runner" || bound != project || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeError(w, forbidden())
			return true
		}
		if path == "runner/artifact-uploads" && r.Method == "POST" {
			var req rc.ArtifactUploadRequest
			if e := releaseDecode(m, &req); e != nil {
				writeError(w, e)
			} else {
				out, e := s.Service.StartArtifactUpload(ctx, p, project, token, req)
				respond(w, 201, out, e)
			}
			return true
		}
		if len(parts) == 3 && parts[1] == "artifact-uploads" && r.Method == "GET" {
			out, e := s.Service.InspectArtifactUpload(ctx, p, project, token, parts[2])
			respond(w, 200, out, e)
			return true
		}
		if len(parts) == 5 && parts[1] == "artifact-uploads" && parts[3] == "blobs" && r.Method == "PUT" {
			e := s.Service.PutArtifactBlob(ctx, p, project, token, parts[2], parts[4], r.Header.Get("X-Suma-Operation-ID"), r.ContentLength, r.Body)
			respond(w, 200, map[string]bool{"acknowledged": e == nil}, e)
			return true
		}
		if len(parts) == 4 && parts[1] == "artifact-uploads" && parts[3] == "seal" && r.Method == "POST" {
			var req rc.ArtifactSealRequest
			if e := releaseDecode(m, &req); e != nil {
				writeError(w, e)
			} else {
				out, e := s.Service.SealArtifactUpload(ctx, p, project, token, parts[2], req)
				respond(w, 200, out, e)
			}
			return true
		}
		if path == "runner/artifacts/resolve" && r.Method == "POST" {
			var req rc.ArtifactResolveRequest
			if e := releaseDecode(m, &req); e != nil {
				writeError(w, e)
			} else {
				out, e := s.Service.ResolveArtifactInput(ctx, p, project, token, req)
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
			b, e := s.Service.RunnerArtifactBlob(ctx, p, project, token, parts[2], parts[4], a)
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
			out, e := s.Service.ReleaseWorkers(ctx, p, project)
			respondList(w, out, e)
			return true
		}
		if len(parts) == 1 && r.Method == "POST" {
			out, e := s.Service.RegisterReleaseWorker(ctx, p, project, token, m)
			respond(w, 201, out, e)
			return true
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			out, e := s.Service.RevokeReleaseWorker(ctx, p, project, parts[1], token, m)
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
			out, total, next, e := s.Service.ReleaseArtifacts(ctx, p, project, limit, cursor)
			respond(w, 200, map[string]any{"items": out, "total": total, "next_cursor": next}, e)
			return true
		}
		if len(parts) == 2 && r.Method == "GET" {
			out, e := s.Service.ReleaseArtifact(ctx, p, project, parts[1])
			respond(w, 200, out, e)
			return true
		}
		if len(parts) == 4 && parts[2] == "blobs" && r.Method == "GET" {
			tx, e := s.Service.DB.BeginTx(ctx, nil)
			if e != nil {
				writeError(w, e)
				return true
			}
			defer tx.Rollback()
			b, e := s.Service.releaseArtifactBlobTx(ctx, tx, p, project, parts[1], parts[3])
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
			out, total, next, e := s.Service.ReleaseCandidates(ctx, p, project, limit, cursor)
			respond(w, 200, map[string]any{"items": out, "total": total, "next_cursor": next}, e)
			return true
		}
		if len(parts) == 1 && r.Method == "POST" {
			out, e := s.Service.CreateReleaseCandidate(ctx, p, project, m)
			respond(w, 201, out, e)
			return true
		}
		if len(parts) == 2 && r.Method == "GET" {
			out, e := s.Service.ReleaseCandidate(ctx, p, project, parts[1])
			respond(w, 200, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	return false
}
