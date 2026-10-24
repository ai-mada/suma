package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func (s *Server) serveSite(site string, w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	var environment string
	if err := s.db.QueryRow(`SELECT id FROM environments WHERE site_id=?`, site).Scan(&environment); err != nil {
		http.Error(w, "No active revision", 503)
		return
	}
	active, err := s.active(environment)
	if err != nil || active.Generation == 0 || active.Directory == "" {
		http.Error(w, "No active revision", 503)
		return
	}
	op, err := s.load(active.OperationID)
	if err != nil || op.Manifest == nil || op.Request.ArtifactSHA256 != active.Digest || op.RevisionDirectory != active.Directory {
		http.Error(w, "Active revision unavailable", 503)
		return
	}
	w.Header().Set("X-Suma-Generation", strconv.FormatInt(active.Generation, 10))
	w.Header().Set("X-Suma-Artifact-SHA256", active.Digest)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/.suma-release" {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"generation": active.Generation, "artifact_sha256": active.Digest})
		}
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" || strings.HasSuffix(name, "/") {
		name += "index.html"
	}
	if !releasecontract.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	var expected *releasecontract.ArtifactFile
	for i := range op.Manifest.Files {
		if op.Manifest.Files[i].Path == name {
			expected = &op.Manifest.Files[i]
			break
		}
	}
	if expected == nil {
		http.NotFound(w, r)
		return
	}
	body, err := safeFile(active.Directory, filepath.Join(active.Directory, filepath.FromSlash(name)), expected.SizeBytes, expected.SHA256)
	if err != nil {
		http.Error(w, "Active content verification failed", 503)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == "GET" {
		_, _ = w.Write(body)
	}
}

func (s *Server) health(ctx context.Context, op *operation) error {
	policy := op.Grant.Plan.Health
	op.Observation.Health = releasecontract.HealthObservation{State: "failed", Generation: op.Request.Generation, ArtifactSHA256: op.Request.ArtifactSHA256, Checks: []releasecontract.HealthCheckResult{}}
	for _, check := range policy.Checks {
		var expected *releasecontract.ArtifactFile
		for i := range op.Manifest.Files {
			if op.Manifest.Files[i].Path == check.ExpectedFile {
				expected = &op.Manifest.Files[i]
				break
			}
		}
		if expected == nil || expected.SHA256 != check.ExpectedSHA256 {
			return fmt.Errorf("health check does not bind actual artifact file")
		}
		passed := false
		var last releasecontract.HealthCheckResult
		for attempt := 0; attempt < policy.Attempts; attempt++ {
			requestCtx, cancel := context.WithTimeout(ctx, time.Duration(policy.TimeoutMS)*time.Millisecond)
			req, _ := http.NewRequestWithContext(requestCtx, "GET", s.SiteURL(op.Grant.Plan.SiteID)+check.Path, nil)
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			resp, err := client.Do(req)
			if err == nil {
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, expected.SizeBytes+1))
				_ = resp.Body.Close()
				last = releasecontract.HealthCheckResult{Path: check.Path, StatusCode: resp.StatusCode, BodySHA256: hashBytes(body), ObservedAt: now()}
				passed = readErr == nil && resp.StatusCode == check.ExpectedStatus && int64(len(body)) == expected.SizeBytes && last.BodySHA256 == check.ExpectedSHA256 && resp.Header.Get("X-Suma-Generation") == strconv.FormatInt(op.Request.Generation, 10) && resp.Header.Get("X-Suma-Artifact-SHA256") == op.Request.ArtifactSHA256
			}
			cancel()
			if passed {
				break
			}
			if ctx.Err() != nil {
				break
			}
		}
		if last.ObservedAt != "" {
			op.Observation.Health.Checks = append(op.Observation.Health.Checks, last)
		}
		if !passed {
			return fmt.Errorf("actual HTTP health check failed")
		}
	}
	// The reserved marker observes the same active-pointer snapshot as content.
	markerCtx, cancel := context.WithTimeout(ctx, time.Duration(policy.TimeoutMS)*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(markerCtx, "GET", s.SiteURL(op.Grant.Plan.SiteID)+"/.suma-release", nil)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("active marker health unavailable")
	}
	defer resp.Body.Close()
	var marker struct {
		Generation     int64  `json:"generation"`
		ArtifactSHA256 string `json:"artifact_sha256"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&marker) != nil || marker.Generation != op.Request.Generation || marker.ArtifactSHA256 != op.Request.ArtifactSHA256 {
		return fmt.Errorf("active marker health differs")
	}
	op.Observation.Health.State = "passed"
	return nil
}
