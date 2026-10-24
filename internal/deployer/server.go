// Package deployer implements the administrator-configured local static provider.
package deployer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
	_ "modernc.org/sqlite"
)

type SiteConfig struct {
	Address string `json:"address"`
}
type Config struct {
	ProviderID        string                `json:"provider_id"`
	Root              string                `json:"root"`
	ControlURL        string                `json:"control_url"`
	ControlTokenEnv   string                `json:"control_token_env"`
	ReconcileTokenEnv string                `json:"reconcile_token_env"`
	Workers           map[string]string     `json:"workers"`
	Sites             map[string]SiteConfig `json:"sites"`
	ManagementAddress string                `json:"management_address"`
	afterActivate     func()
	afterConsume      func()
}
type Server struct {
	cfg              Config
	db               *sql.DB
	ctx              context.Context
	cancel           context.CancelFunc
	unlock           func() error
	mu               sync.Mutex
	working          map[string]context.CancelFunc
	environmentLocks sync.Map
	sites            map[string]*http.Server
	addresses        map[string]string
	wg               sync.WaitGroup
}
type operation struct {
	Request           releasecontract.NonSecretDeploymentRequest    `json:"request"`
	Grant             *releasecontract.DeploymentAuthorizationGrant `json:"grant,omitempty"`
	EffectGrant       *releasecontract.DeploymentEffectGrant        `json:"effect_grant,omitempty"`
	Manifest          *releasecontract.ArtifactManifest             `json:"manifest,omitempty"`
	RevisionDirectory string                                        `json:"revision_directory,omitempty"`
	Observation       releasecontract.DeploymentObservation         `json:"observation"`
	ReportedVersion   int                                           `json:"reported_version"`
}
type activeRevision struct {
	Fence, Generation                          int64
	ArtifactID, Digest, Directory, OperationID string
}

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func now() string               { return time.Now().UTC().Format(time.RFC3339Nano) }
func terminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}
func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func New(cfg Config) (result *Server, err error) {
	if cfg.ProviderID == "" || len(cfg.Sites) == 0 || len(cfg.Workers) == 0 || os.Getenv(cfg.ControlTokenEnv) == "" || os.Getenv(cfg.ReconcileTokenEnv) == "" {
		return nil, fmt.Errorf("provider identity, sites, workers and independent credentials required")
	}
	u, e := url.Parse(cfg.ControlURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return nil, fmt.Errorf("provider requires fixed HTTPS or loopback control origin")
	}
	root, e := filepath.Abs(cfg.Root)
	if e != nil {
		return nil, e
	}
	if !strings.Contains(strings.ToLower(filepath.ToSlash(root)), "/.cache/") {
		return nil, fmt.Errorf("local provider root must be a dedicated child of an administrator .cache directory")
	}
	if e = safeDirectory(root); e != nil {
		return nil, e
	}
	cfg.Root = root
	unlock, e := lockProvider(root)
	if e != nil {
		return nil, fmt.Errorf("provider state already owned or unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{cfg: cfg, ctx: ctx, cancel: cancel, unlock: unlock, working: map[string]context.CancelFunc{}, sites: map[string]*http.Server{}, addresses: map[string]string{}}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	s.db, e = sql.Open("sqlite", filepath.Join(root, "provider.db"))
	if e != nil {
		return nil, e
	}
	s.db.SetMaxOpenConns(1)
	for _, ddl := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=FULL`, `CREATE TABLE IF NOT EXISTS operations(id TEXT PRIMARY KEY,record TEXT NOT NULL,cancel_requested INTEGER NOT NULL DEFAULT 0)`, `CREATE TABLE IF NOT EXISTS environments(id TEXT PRIMARY KEY,site_id TEXT NOT NULL,fence_generation INTEGER NOT NULL,active_generation INTEGER NOT NULL,artifact_id TEXT NOT NULL,artifact_sha256 TEXT NOT NULL,directory TEXT NOT NULL,operation_id TEXT NOT NULL)`, `CREATE UNIQUE INDEX IF NOT EXISTS provider_site_environment ON environments(site_id)`} {
		if _, e = s.db.Exec(ddl); e != nil {
			return nil, e
		}
	}
	for worker, env := range cfg.Workers {
		if worker == "" || os.Getenv(env) == "" {
			return nil, fmt.Errorf("worker transport identity is not configured")
		}
	}
	for id, site := range cfg.Sites {
		if id == "" || !loopbackAddress(site.Address) {
			return nil, fmt.Errorf("sites require explicit loopback address")
		}
		listener, e := net.Listen("tcp", site.Address)
		if e != nil {
			return nil, fmt.Errorf("site listener unavailable")
		}
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.serveSite(id, w, r) }), ReadHeaderTimeout: 5 * time.Second}
		s.sites[id] = server
		s.addresses[id] = "http://" + listener.Addr().String()
		s.wg.Add(1)
		go func() { defer s.wg.Done(); _ = server.Serve(listener) }()
	}
	rows, e := s.db.Query(`SELECT record FROM operations`)
	if e != nil {
		return nil, e
	}
	pending := []operation{}
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			_ = rows.Close()
			return nil, e
		}
		var op operation
		if e = json.Unmarshal([]byte(raw), &op); e != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("provider operation state corrupt")
		}
		if !terminal(op.Observation.State) {
			pending = append(pending, op)
		}
	}
	e = rows.Err()
	_ = rows.Close()
	if e != nil {
		return nil, e
	}
	for _, op := range pending {
		if op.Grant == nil {
			grant, inspectErr := s.inspectAuthorization(s.ctx, &op)
			if inspectErr != nil {
				state, code := "reconciling", "authorization_recovery_unknown"
				if controlNotFound(inspectErr) {
					state, code = "received", "authorization_required"
				}
				op.Observation.Error = &releasecontract.DeploymentError{Code: code, Message: code}
				if e = s.transition(&op, state); e != nil {
					return nil, e
				}
				continue
			}
			op.Grant = &grant
			op.Observation.Error = nil
			if e = s.transition(&op, "authorized"); e != nil {
				return nil, e
			}
		}
		s.start(op, "")
	}
	s.wg.Add(1)
	go s.reportLoop()
	return s, nil
}
func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	for _, cancel := range s.working {
		cancel()
	}
	s.mu.Unlock()
	for _, site := range s.sites {
		_ = site.Close()
	}
	s.wg.Wait()
	var err error
	if s.db != nil {
		err = s.db.Close()
	}
	if s.unlock != nil {
		unlockErr := s.unlock()
		s.unlock = nil
		if err == nil {
			err = unlockErr
		}
	}
	return err
}
func (s *Server) SiteURL(id string) string { return s.addresses[id] }
func (s *Server) load(id string) (operation, error) {
	var op operation
	var raw string
	err := s.db.QueryRow(`SELECT record FROM operations WHERE id=?`, id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &op)
	}
	return op, err
}
func (s *Server) save(op *operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, err := s.load(op.Request.OperationID); err == nil && prior.ReportedVersion > op.ReportedVersion {
		op.ReportedVersion = prior.ReportedVersion
	}
	raw, err := json.Marshal(op)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE operations SET record=? WHERE id=?`, string(raw), op.Request.OperationID)
	return err
}
func (s *Server) transition(op *operation, state string) error {
	op.Observation.State = state
	op.Observation.Version++
	op.Observation.EvidenceSHA256, _ = releasecontract.ObservationDigest(op.Observation)
	return s.save(op)
}
func (s *Server) active(environment string) (activeRevision, error) {
	var a activeRevision
	err := s.db.QueryRow(`SELECT fence_generation,active_generation,artifact_id,artifact_sha256,directory,operation_id FROM environments WHERE id=?`, environment).Scan(&a.Fence, &a.Generation, &a.ArtifactID, &a.Digest, &a.Directory, &a.OperationID)
	return a, err
}
