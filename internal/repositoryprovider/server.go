// Package repositoryprovider implements a fixed, local, administrator-owned Git
// provider. Its durable intent is separate from the atomic Git effect receipt.
package repositoryprovider

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type TargetConfig struct {
	BareRepository    string `json:"bare_repository"`
	Ref               string `json:"ref"`
	DeliveryBaseAlias string `json:"delivery_base_alias,omitempty"`
}
type Config struct {
	ProviderID           string                  `json:"provider_id"`
	TenantID             string                  `json:"tenant_id"`
	ProjectID            string                  `json:"project_id"`
	ControlURL           string                  `json:"control_url"`
	ControlTokenEnv      string                  `json:"control_token_env"`
	RunnerTokenEnv       string                  `json:"runner_token_env"`
	ReconcileTokenEnv    string                  `json:"reconcile_token_env"`
	SourceReadTokenEnv   string                  `json:"source_read_token_env,omitempty"`
	SourceExportTokenEnv string                  `json:"source_export_token_env,omitempty"`
	GitExecutable        string                  `json:"git_executable"`
	Targets              map[string]TargetConfig `json:"targets"`
	StateDir             string                  `json:"-"`
}
type target struct {
	config                                  TargetConfig
	root                                    *os.Root
	identity                                os.FileInfo
	format, configSHA, gitSHA, rawConfigSHA string
}
type operation struct {
	Request         rc.RepositoryNonSecretRequest    `json:"request"`
	Grant           *rc.RepositoryAuthorizationGrant `json:"grant,omitempty"`
	Effect          *rc.RepositoryEffectGrant        `json:"effect,omitempty"`
	Observation     rc.RepositoryObservation         `json:"observation"`
	ReportedVersion int                              `json:"reported_version"`
}
type Server struct {
	cfg      Config
	db       *sql.DB
	mu       sync.Mutex
	exportMu sync.Mutex
	targets  map[string]*target
	unlocks  []func() error
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	afterGit func()
}

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func now() string               { return time.Now().UTC().Format(time.RFC3339Nano) }
func safeDirectory(path string, create bool) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("provider paths must be absolute")
	}
	path = filepath.Clean(path)
	if create {
		if e := os.MkdirAll(path, 0700); e != nil {
			return "", e
		}
	}
	for p := path; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("provider directory is unavailable or linked")
		}
		real, e := filepath.EvalSymlinks(p)
		if e != nil || !strings.EqualFold(real, p) {
			return "", fmt.Errorf("provider path traverses a link")
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return path, nil
}
func New(cfg Config) (result *Server, err error) {
	if cfg.ProviderID == "" || cfg.TenantID == "" || cfg.ProjectID == "" || len(cfg.Targets) < 1 || len(cfg.Targets) > 64 {
		return nil, fmt.Errorf("repository provider identity and fixed targets required")
	}
	u, e := url.Parse(cfg.ControlURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return nil, fmt.Errorf("repository provider control must be a fixed secure origin")
	}
	seen := map[string]bool{}
	values := map[string]bool{}
	for _, name := range []string{cfg.ControlTokenEnv, cfg.RunnerTokenEnv, cfg.ReconcileTokenEnv} {
		v := os.Getenv(name)
		if name == "" || seen[name] || len(v) < 32 || values[v] {
			return nil, fmt.Errorf("three distinct repository credentials of at least 32 bytes required")
		}
		seen[name] = true
		values[v] = true
	}
	// Source content has a separate opt-in read capability. A Runner or a
	// reconciliation credential must not gain arbitrary repository content IO.
	for _, env := range []string{cfg.SourceReadTokenEnv, cfg.SourceExportTokenEnv} {
		if env == "" {
			continue
		}
		value := os.Getenv(env)
		if seen[env] || len(value) < 32 || values[value] || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("source access requires independent credentials of at least 32 bytes")
		}
		seen[env], values[value] = true, true
	}
	if !filepath.IsAbs(cfg.GitExecutable) {
		return nil, fmt.Errorf("fixed absolute Git executable required")
	}
	st, e := os.Lstat(cfg.GitExecutable)
	real, e2 := filepath.EvalSymlinks(cfg.GitExecutable)
	if e != nil || e2 != nil || !st.Mode().IsRegular() || !strings.EqualFold(real, cfg.GitExecutable) {
		return nil, fmt.Errorf("Git executable is unavailable or linked")
	}
	dir, e := safeDirectory(cfg.StateDir, true)
	if e != nil {
		return nil, e
	}
	cfg.StateDir = dir
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{cfg: cfg, ctx: ctx, cancel: cancel, targets: map[string]*target{}}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	unlock, e := lockProvider(dir)
	if e != nil {
		return nil, fmt.Errorf("repository provider state already owned")
	}
	s.unlocks = append(s.unlocks, unlock)
	for _, name := range []string{"repository-provider.db", "repository-provider.db-wal", "repository-provider.db-shm"} {
		if st, err := os.Lstat(filepath.Join(dir, name)); err == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
			return nil, fmt.Errorf("repository state file linked or unavailable")
		}
	}
	s.db, e = sql.Open("sqlite", filepath.Join(dir, "repository-provider.db"))
	if e != nil {
		return nil, e
	}
	s.db.SetMaxOpenConns(1)
	for _, ddl := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=FULL`, `CREATE TABLE IF NOT EXISTS operations(id TEXT PRIMARY KEY,record TEXT NOT NULL)`} {
		if _, e = s.db.Exec(ddl); e != nil {
			return nil, e
		}
	}
	physical := map[string]bool{}
	repoLocks := map[string]bool{}
	for alias, c := range cfg.Targets {
		if alias == "" || len(alias) > 100 || strings.ContainsAny(alias, "/\\\x00\r\n") || !rc.RepositoryValidRef(c.Ref) {
			return nil, fmt.Errorf("invalid repository target alias or ref")
		}
		repo, e := safeDirectory(c.BareRepository, false)
		if e != nil {
			return nil, e
		}
		c.BareRepository = repo
		key := repo + "\x00" + c.Ref
		if os.PathSeparator == '\\' {
			key = strings.ToLower(key)
		}
		if physical[key] {
			return nil, fmt.Errorf("duplicate physical repository target")
		}
		physical[key] = true
		if !repoLocks[repo] {
			unlock, e = lockProvider(repo)
			if e != nil {
				return nil, fmt.Errorf("bare repository already owned")
			}
			s.unlocks = append(s.unlocks, unlock)
			repoLocks[repo] = true
		}
		root, e := os.OpenRoot(repo)
		if e != nil {
			return nil, e
		}
		identity, e := os.Stat(repo)
		if e != nil {
			root.Close()
			return nil, e
		}
		t := &target{config: c, root: root, identity: identity}
		s.targets[alias] = t
		git, e := os.ReadFile(cfg.GitExecutable)
		if e != nil {
			return nil, e
		}
		t.gitSHA = hashBytes(git)
		if e = s.validateTarget(ctx, t, true); e != nil {
			return nil, e
		}
		config, e := t.root.ReadFile("config")
		if e != nil {
			return nil, e
		}
		t.rawConfigSHA = hashBytes(config)
		t.configSHA, _ = rc.RepositoryCanonicalDigest(struct{ Protocol, Provider, Tenant, Project, Alias, Ref, Repository, GitSHA, ConfigSHA, Format string }{rc.RepositoryProtocol, cfg.ProviderID, cfg.TenantID, cfg.ProjectID, alias, c.Ref, repo, t.gitSHA, t.rawConfigSHA, t.format})
	}
	for _, t := range s.targets {
		alias := t.config.DeliveryBaseAlias
		if alias == "" {
			continue
		}
		base := s.targets[alias]
		if cfg.SourceExportTokenEnv == "" || base == nil || base.config.DeliveryBaseAlias != "" || base.config.Ref == t.config.Ref || !os.SameFile(t.identity, base.identity) {
			return nil, fmt.Errorf("source export requires a distinct baseline target in the same bare repository and an independent credential")
		}
		// The source registration must change when its designated baseline
		// changes, even if both branches temporarily point to the same commit.
		t.configSHA, _ = rc.RepositoryCanonicalDigest(struct{ TargetConfigSHA256, DeliveryBaseAlias, DeliveryBaseConfigSHA256 string }{t.configSHA, alias, base.configSHA})
	}
	s.wg.Add(1)
	go s.reportLoop()
	return s, nil
}
func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.exportMu.Lock()
	defer s.exportMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.targets {
		t.root.Close()
	}
	var err error
	if s.db != nil {
		err = s.db.Close()
	}
	for i := len(s.unlocks) - 1; i >= 0; i-- {
		s.unlocks[i]()
	}
	return err
}
func (s *Server) load(id string) (operation, error) {
	var raw string
	err := s.db.QueryRow(`SELECT record FROM operations WHERE id=?`, id).Scan(&raw)
	var op operation
	if err == nil {
		err = json.Unmarshal([]byte(raw), &op)
	}
	return op, err
}
func (s *Server) save(op *operation) error {
	b, e := json.Marshal(op)
	if e != nil {
		return e
	}
	_, e = s.db.Exec(`INSERT INTO operations(id,record)VALUES(?,?) ON CONFLICT(id)DO UPDATE SET record=excluded.record`, op.Request.OperationID, string(b))
	return e
}
func (s *Server) transition(op *operation, state, reason string) error {
	op.Observation.State = state
	op.Observation.Reason = reason
	op.Observation.Version++
	op.Observation.ObservedAt = now()
	op.Observation.ObservationSHA256, _ = rc.RepositoryObservationDigest(op.Observation)
	return s.save(op)
}
func (s *Server) Target(ctx context.Context, alias string) (rc.RepositoryTargetObservation, error) {
	t := s.targets[alias]
	o := rc.RepositoryTargetObservation{Alias: alias, ObservedAt: now()}
	if t == nil {
		return o, fmt.Errorf("repository target is not configured")
	}
	o.Ref = t.config.Ref
	o.ConfigSHA256 = t.configSHA
	o.ObjectFormat = t.format
	if err := s.validateTarget(ctx, t, false); err != nil {
		o.Reason = "repository target configuration changed"
		return o, err
	}
	head, e := s.git(ctx, t, nil, 4096, "rev-parse", "--verify", t.config.Ref+"^{commit}")
	if e != nil {
		return o, e
	}
	o.HeadCommit = strings.TrimSpace(string(head))
	o.FenceOID, o.Generation, e = s.fence(ctx, t)
	if e != nil {
		return o, e
	}
	o.Available = true
	return o, nil
}
