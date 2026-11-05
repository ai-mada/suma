package repositoryprovider

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func (s *Server) sourceExportHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.SourceExportTokenEnv == "" || !token(r, s.cfg.SourceExportTokenEnv) {
		writeError(w, http.StatusForbidden, "repository_source_export_forbidden")
		return
	}
	transport := http.NewResponseController(w)
	_ = transport.SetReadDeadline(time.Now().Add(5 * time.Second))
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var request ri.RepositorySourceExportRequest
	if r.Method != http.MethodPost || r.URL.ForceQuery || r.URL.RawQuery != "" || decode(r, &request) != nil {
		writeError(w, http.StatusBadRequest, "repository_source_export_invalid")
		return
	}
	requestSHA, err := ri.RepositorySourceExportDigest(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "repository_source_export_invalid")
		return
	}
	if request.TenantID != s.cfg.TenantID || request.ProjectID != s.cfg.ProjectID || request.ProviderID != s.cfg.ProviderID {
		writeError(w, http.StatusForbidden, "repository_source_export_forbidden")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	// Bound full-object I/O without serializing prepare, apply or reconciliation.
	// The fixed header and final reread detect concurrent ref movement.
	if !s.exportMu.TryLock() {
		writeError(w, http.StatusTooManyRequests, "repository_source_export_busy")
		return
	}
	defer s.exportMu.Unlock()
	if s.ctx.Err() != nil {
		writeError(w, http.StatusServiceUnavailable, "repository_source_export_unavailable")
		return
	}
	data, err := s.exportSource(ctx, request)
	if err != nil || ctx.Err() != nil {
		writeError(w, http.StatusConflict, "repository_source_export_unavailable")
		return
	}
	// Keep the single-export slot through transmission to bound buffered bytes.
	_ = transport.SetWriteDeadline(time.Now().Add(10 * time.Second))
	w.Header().Set("Content-Type", "application/x-git-bundle")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("X-Suma-Bundle-SHA256", hashBytes(data))
	w.Header().Set("X-Suma-Source-Request-SHA256", requestSHA)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type sourceExportObservation struct {
	source, base ri.RepositoryTargetObservation
}

func (s *Server) observeSourceExport(ctx context.Context, q ri.RepositorySourceExportRequest, baseAlias string) (sourceExportObservation, error) {
	var result sourceExportObservation
	source, err := s.Target(ctx, q.TargetAlias)
	if err != nil || !source.Available || source.Ref != q.TargetRef || source.ConfigSHA256 != q.TargetConfigSHA256 ||
		source.HeadCommit != q.ExpectedHead || source.FenceOID != q.ExpectedFenceOID || source.Generation != q.ExpectedGeneration {
		return result, fmt.Errorf("registered source changed")
	}
	base, err := s.Target(ctx, baseAlias)
	if err != nil || !base.Available || base.HeadCommit != q.ExpectedBase || base.ObjectFormat != source.ObjectFormat {
		return result, fmt.Errorf("designated baseline changed")
	}
	commitLength := sha1.Size * 2
	if source.ObjectFormat == "sha256" {
		commitLength = sha256.Size * 2
	}
	if len(q.ExpectedHead) != commitLength || len(q.ExpectedBase) != commitLength {
		return result, fmt.Errorf("source object format changed")
	}
	// Compare immutable evidence and ref state across the Git operation.
	source.ObservedAt, base.ObservedAt = "", ""
	return sourceExportObservation{source: source, base: base}, nil
}

func (s *Server) exportSource(ctx context.Context, q ri.RepositorySourceExportRequest) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t := s.targets[q.TargetAlias]
	if t == nil || t.config.DeliveryBaseAlias == "" {
		return nil, fmt.Errorf("source export is not configured")
	}
	before, err := s.observeSourceExport(ctx, q, t.config.DeliveryBaseAlias)
	if err != nil {
		return nil, err
	}
	if _, err = s.git(ctx, t, nil, 4096, "merge-base", "--is-ancestor", q.ExpectedBase, q.ExpectedHead); err != nil {
		return nil, fmt.Errorf("source does not descend from the designated baseline")
	}
	// Git requires a named ref to create a bundle. The existing candidate
	// contract accepts one HEAD, so normalize the checked ref label only.
	data, err := s.git(ctx, t, nil, int(cc.MaxBundleBytes), "-c", "pack.threads=1", "-c", "pack.windowMemory=16m", "bundle", "create", "--quiet", "-", q.TargetRef, "^"+q.ExpectedBase)
	if err != nil {
		return nil, err
	}
	offset, err := bundleHeaderRef(data, q.ExpectedBase, q.ExpectedHead, q.TargetRef)
	if err != nil {
		return nil, err
	}
	if !validExportPack(data[offset:], before.source.ObjectFormat) {
		return nil, fmt.Errorf("invalid source bundle pack")
	}
	start := offset - len(q.TargetRef) - len("\n\n")
	const advertised = "HEAD\n\n"
	copy(data[start:], advertised)
	data = data[:start+len(advertised)+copy(data[start+len(advertised):], data[offset:])]
	if _, err = s.git(ctx, t, bytes.NewReader(data), 8192, "bundle", "verify", "-"); err != nil {
		return nil, fmt.Errorf("source bundle verification failed")
	}
	after, err := s.observeSourceExport(ctx, q, t.config.DeliveryBaseAlias)
	if err != nil || before != after {
		return nil, fmt.Errorf("registered source or baseline changed during export")
	}
	return data, nil
}

// Verify the generated pack envelope without importing objects into the
// administrator repository. A recipient still verifies the objects on import.
func validExportPack(data []byte, format string) bool {
	const headerSize = 12
	trailerSize := sha1.Size
	if format == "sha256" {
		trailerSize = sha256.Size
	}
	if len(data) <= headerSize+trailerSize || string(data[:4]) != "PACK" || binary.BigEndian.Uint32(data[8:headerSize]) == 0 {
		return false
	}
	version := binary.BigEndian.Uint32(data[4:8])
	if version != 2 && version != 3 {
		return false
	}
	pack, trailer := data[:len(data)-trailerSize], data[len(data)-trailerSize:]
	if format == "sha256" {
		digest := sha256.Sum256(pack)
		return bytes.Equal(digest[:], trailer)
	}
	digest := sha1.Sum(pack)
	return bytes.Equal(digest[:], trailer)
}
