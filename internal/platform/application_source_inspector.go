package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// ApplicationSourceInspectorConfig pins a trusted, bundled parser and Node.
// Source manifests remain untrusted data. There is no caller-selected program,
// shell, project cwd, environment, URL or installation step in this contract.
type ApplicationSourceInspectorConfig struct {
	node, entry, nodeSHA, entrySHA string
	parserSHA                      string
}

const (
	applicationSourceUnavailable = "Application source inspection is unavailable"
	applicationSourceChanged     = "The configured application source inspector changed"
	applicationSourceInvalid     = "The application source report failed its fixed contract"
	applicationSourceLimit       = "The application source exceeds the inspection allowance"
)

func applicationSourceError(status int, code, message string) error {
	return &APIError{Status: status, Code: "application_source_" + code, Message: message}
}

// These options are process-local deployment configuration, never business
// form input. An absent pair disables source inspection; a partial pair is an
// invalid deployment instead of silently falling back to another executable.
func LoadApplicationSourceInspector(getenv func(string) string) (ApplicationSourceInspectorConfig, error) {
	var cfg ApplicationSourceInspectorConfig
	cfg.node, cfg.entry = getenv("SUMA_SOURCE_INSPECTOR_NODE"), getenv("SUMA_SOURCE_INSPECTOR_ENTRY")
	if cfg.node == "" && cfg.entry == "" {
		return cfg, nil
	}
	if cfg.node == "" || cfg.entry == "" || !strings.HasSuffix(cfg.entry, ".mjs") {
		return cfg, fmt.Errorf("source inspection requires both a fixed Node executable and a bundled .mjs entry")
	}
	// Resolve administrator-selected aliases once, then retain only exact real
	// files. Later alias retargeting cannot change an already loaded program.
	var err error
	cfg.node, err = filepath.Abs(cfg.node)
	if err != nil {
		return ApplicationSourceInspectorConfig{}, err
	}
	cfg.entry, err = filepath.Abs(cfg.entry)
	if err != nil {
		return ApplicationSourceInspectorConfig{}, err
	}
	cfg.node, err = filepath.EvalSymlinks(cfg.node)
	if err != nil {
		return ApplicationSourceInspectorConfig{}, err
	}
	cfg.entry, err = filepath.EvalSymlinks(cfg.entry)
	if err != nil {
		return ApplicationSourceInspectorConfig{}, err
	}
	cfg.nodeSHA, err = applicationInspectorFileSHA(cfg.node, 512<<20)
	if err != nil {
		return ApplicationSourceInspectorConfig{}, err
	}
	cfg.entrySHA, err = applicationInspectorFileSHA(cfg.entry, 16<<20)
	if err != nil {
		return ApplicationSourceInspectorConfig{}, err
	}
	cfg.parserSHA = digest(stringify([]string{"suma-source-inspector-program-v1", cfg.nodeSHA, cfg.entrySHA}))
	return cfg, nil
}

func applicationInspectorFileSHA(path string, max int64) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("source inspector executable paths must be canonical absolute files")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > max {
		return "", fmt.Errorf("source inspector executable is unavailable or exceeds its fixed bound")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(resolved, path) {
		return "", fmt.Errorf("source inspector executable cannot traverse a link")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", fmt.Errorf("source inspector executable identity changed")
	}
	// Existing digest helpers use canonical UTF-8 text; executable bytes need
	// their own lossless stream hash instead of conversion through a string.
	return applicationInspectorStreamSHA(f, max)
}

func (s *Service) readApplicationSource(ctx context.Context, request ri.RepositorySourceInspectionRequest) (ri.ProviderSourceInspection, error) {
	var out ri.ProviderSourceInspection
	if ri.ValidateRepositorySourceInspectionRequest(request) != nil {
		return out, applicationSourceError(400, "invalid", applicationSourceInvalid)
	}
	cfg, err := s.repositoryProvider(request.TenantID, request.ProjectID, request.ProviderID, request.TargetAlias)
	if err != nil {
		return out, err
	}
	if len(cfg.SourceReadToken) < 32 || digest(cfg.SourceReadToken) == cfg.TokenHash || cfg.SourceReadToken == cfg.ReconcileToken {
		return out, applicationSourceError(503, "unavailable", applicationSourceUnavailable)
	}
	if err = repositoryProviderRequestHTTP(ctx, cfg, cfg.SourceReadToken, ri.RepositorySourceMaxTransportBytes, "POST", "/source-inspections", request, &out); err != nil {
		return ri.ProviderSourceInspection{}, err
	}
	if ri.ValidateProviderSourceInspection(request, out) != nil {
		return ri.ProviderSourceInspection{}, applicationSourceError(502, "invalid", applicationSourceInvalid)
	}
	return out, nil
}

type applicationInspectorBuffer struct {
	bytes.Buffer
	max int
}

func (b *applicationInspectorBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, fmt.Errorf("source inspector output exceeds fixed allowance")
	}
	return b.Buffer.Write(p)
}

func (s *Service) runApplicationSourceInspector(ctx context.Context, sources []ri.ProviderSourceInspection) (ri.SourceDependencyReport, error) {
	var report ri.SourceDependencyReport
	cfg := s.ApplicationSourceInspector
	if cfg.node == "" || cfg.entry == "" {
		return report, applicationSourceError(503, "unavailable", applicationSourceUnavailable)
	}
	nodeSHA, e1 := applicationInspectorFileSHA(cfg.node, 512<<20)
	entrySHA, e2 := applicationInspectorFileSHA(cfg.entry, 16<<20)
	if e1 != nil || e2 != nil || nodeSHA != cfg.nodeSHA || entrySHA != cfg.entrySHA {
		return report, applicationSourceError(409, "program_changed", applicationSourceChanged)
	}
	count, size := 0, int64(0)
	for _, source := range sources {
		count += len(source.Manifests)
		size += source.Coverage.ContentBytes
	}
	if len(sources) < 1 || len(sources) > 8 || count > ri.RepositorySourceMaxManifests || size > ri.RepositorySourceMaxContentBytes {
		return report, applicationSourceError(413, "limit", applicationSourceLimit)
	}
	input, err := json.Marshal(struct {
		SchemaVersion int                           `json:"schema_version"`
		Sources       []ri.ProviderSourceInspection `json:"sources"`
	}{1, sources})
	if err != nil || len(input) > ri.RepositorySourceMaxTransportBytes {
		return report, applicationSourceError(413, "limit", applicationSourceLimit)
	}
	if s.DataDir == "" {
		return report, applicationSourceError(503, "unavailable", applicationSourceUnavailable)
	}
	base, err := filepath.Abs(s.DataDir)
	if err != nil {
		return report, applicationSourceError(503, "unavailable", applicationSourceUnavailable)
	}
	cwd := filepath.Join(base, "source-inspector")
	if err = os.MkdirAll(cwd, 0700); err != nil {
		return report, applicationSourceError(503, "unavailable", applicationSourceUnavailable)
	}
	resolved, err := filepath.EvalSymlinks(cwd)
	if err != nil || !strings.EqualFold(resolved, cwd) {
		return report, applicationSourceError(503, "unavailable", applicationSourceUnavailable)
	}
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, cfg.node, "--no-addons", "--disable-proto=throw", "--max-old-space-size=128", cfg.entry)
	command.Dir = cwd
	command.Stdin = bytes.NewReader(input)
	command.WaitDelay = time.Second
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := os.Getenv(key); value != "" {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	stdout, stderr := &applicationInspectorBuffer{max: ri.RepositorySourceMaxReportBytes}, &applicationInspectorBuffer{max: 8192}
	command.Stdout, command.Stderr = stdout, stderr
	if err = command.Run(); err != nil {
		return report, applicationSourceError(502, "invalid", applicationSourceInvalid)
	}
	raw := stdout.Bytes()
	if RejectDuplicateReleaseJSON(raw) != nil {
		return report, applicationSourceError(502, "invalid", applicationSourceInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF {
		return ri.SourceDependencyReport{}, applicationSourceError(502, "invalid", applicationSourceInvalid)
	}
	if err = validateApplicationDependencyReport(sources, report); err != nil {
		return ri.SourceDependencyReport{}, applicationSourceError(502, "invalid", applicationSourceInvalid)
	}
	return report, nil
}
