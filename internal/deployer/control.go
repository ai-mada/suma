package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func secret(env string) string { return os.Getenv(env) }

type controlHTTPError struct{ status int }

func (e controlHTTPError) Error() string { return fmt.Sprintf("control request HTTP %d", e.status) }
func controlNotFound(err error) bool {
	var status controlHTTPError
	return errors.As(err, &status) && status.status == 404
}

func (s *Server) inspectAuthorization(ctx context.Context, op *operation) (releasecontract.DeploymentAuthorizationGrant, error) {
	var grant releasecontract.DeploymentAuthorizationGrant
	err := s.control(ctx, "POST", "deployment-authorizations/inspect", releasecontract.InspectDeploymentAuthorization{OperationID: op.Request.OperationID, Assignment: op.Request.Assignment, RequestSHA256: op.Observation.RequestSHA256}, &grant)
	if err == nil && !s.validGrant(op, grant) {
		err = fmt.Errorf("historical authorization binding differs")
	}
	return grant, err
}
func (s *Server) control(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.cfg.ControlURL, "/")+"/api/v1/provider/"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret(s.cfg.ControlTokenEnv))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("control request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return controlHTTPError{resp.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	}
	return nil
}
func (s *Server) controlBlob(ctx context.Context, op *operation, sha string, size int64) ([]byte, error) {
	path := "artifacts/" + url.PathEscape(op.Request.ArtifactID) + "/blobs/" + sha + "?operation_id=" + url.QueryEscape(op.Request.OperationID)
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.cfg.ControlURL, "/")+"/api/v1/provider/"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret(s.cfg.ControlTokenEnv))
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artifact download unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("artifact download rejected")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, size+1))
	if err != nil || int64(len(b)) != size || hashBytes(b) != sha {
		return nil, fmt.Errorf("artifact actual bytes differ from manifest")
	}
	return b, nil
}
func (s *Server) reportLoop() {
	defer s.wg.Done()
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
		}
		rows, err := s.db.Query(`SELECT record FROM operations`)
		if err != nil {
			continue
		}
		records := []operation{}
		for rows.Next() {
			var raw string
			if rows.Scan(&raw) != nil {
				continue
			}
			var op operation
			if json.Unmarshal([]byte(raw), &op) == nil && op.Grant != nil && op.ReportedVersion < op.Observation.Version {
				records = append(records, op)
			}
		}
		_ = rows.Close()
		for _, op := range records {
			s.report(&op)
		}
	}
}
func (s *Server) report(op *operation) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()
	if s.control(ctx, "POST", "deployment-observations", op.Observation, nil) == nil {
		s.mu.Lock()
		latest, err := s.load(op.Request.OperationID)
		if err == nil {
			if latest.ReportedVersion < op.Observation.Version {
				latest.ReportedVersion = op.Observation.Version
			}
			raw, _ := json.Marshal(latest)
			_, _ = s.db.Exec(`UPDATE operations SET record=? WHERE id=?`, string(raw), latest.Request.OperationID)
		}
		s.mu.Unlock()
	}
}
