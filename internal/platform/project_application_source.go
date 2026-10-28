package platform

import (
	"context"
	"sort"
	"strings"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// Private read basis binds IO to the original applied operation. Mutable target
// versions protect only this double-read; they do not enter permanent ownership.
type applicationSourceBasis struct {
	Source        ApplicationVersionSource
	Request       ri.RepositorySourceInspectionRequest
	TargetVersion int
}

func applicationPreviewValid(project string, q ApplicationVersionPreviewRequest) bool {
	if q.ProjectID != project || q.ExpectedApplicationVersion < 1 || !testPlanText(q.Label, 64, true) || !testPlanText(q.Reason, 2000, true) || q.Sources == nil || len(q.Sources) < 1 || len(q.Sources) > 8 || q.Dependencies == nil || len(q.Dependencies) > 64 {
		return false
	}
	seen := map[string]bool{}
	for _, x := range q.Sources {
		if !handoffID(x.ScopeID, false) || !handoffID(x.IntegrationPlanID, false) || x.ExpectedIntegrationVersion < 1 || seen[x.ScopeID] {
			return false
		}
		seen[x.ScopeID] = true
	}
	seen = map[string]bool{}
	for _, d := range q.Dependencies {
		if !handoffID(d.TargetApplicationID, false) || !handoffID(d.TargetVersionID, false) || !contentDigest.MatchString(d.TargetVersionSHA256) || !testPlanText(d.ObservationID, 64, false) || !testPlanText(d.DeclaredRange, 500, false) || !testPlanText(d.Reason, 2000, true) || seen[d.TargetApplicationID] {
			return false
		}
		seen[d.TargetApplicationID] = true
	}
	return true
}
func (s *Service) applicationSourceBasisTx(ctx context.Context, auth *authorizedCommand, a ProjectApplication, input ApplicationVersionSourceInput) (out applicationSourceBasis, err error) {
	var scope ApplicationCodeScope
	for _, x := range a.CodeScopes {
		if x.ID == input.ScopeID {
			scope = x
		}
	}
	if scope.ID == "" {
		return out, conflict(applicationScope)
	}
	target, err := s.repositoryTargetTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, scope.TargetID)
	if err != nil {
		return out, err
	}
	if target.Status != "active" || target.ProviderID != scope.ProviderID || target.TargetAlias != scope.TargetAlias || target.Ref != scope.TargetRef || target.ConfigSHA256 != scope.ConfigSHA256 {
		return out, conflict(applicationScope)
	}
	cfg, err := s.repositoryProvider(auth.target.tenantID, a.ProjectID, scope.ProviderID, scope.TargetAlias)
	if err != nil {
		return out, err
	}
	plan, err := s.repositoryPlanTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, input.IntegrationPlanID)
	if err != nil {
		return out, err
	}
	if plan.ProjectID != a.ProjectID || plan.Version != input.ExpectedIntegrationVersion || plan.TargetID != scope.TargetID || plan.State != "integrated" || plan.OperationID == "" || plan.ApplyRunID == "" || plan.Observation == nil {
		return out, conflict(applicationSource)
	}
	op, err := s.repositoryOperationTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, plan.OperationID)
	if err != nil {
		return out, err
	}
	if op.Execution.Phase != "apply" || op.Execution.PlanID != plan.ID || op.Execution.TargetID != scope.TargetID || op.Execution.TargetConfigSHA256 != scope.ConfigSHA256 || op.Execution.ProviderID != scope.ProviderID || op.Execution.TargetAlias != scope.TargetAlias || op.Execution.TargetRef != scope.TargetRef || op.RunID != plan.ApplyRunID || op.NodeID != "apply" || op.Assignment == nil {
		return out, conflict(applicationSource)
	}
	o := *plan.Observation
	if o.State != "applied" || o.Phase != "apply" || o.Receipt == nil || o.Receipt.State != "applied" || o.Receipt.ResultCommit != plan.Basis.Source.HeadCommit {
		return out, conflict(applicationSource)
	}
	if err = s.validateRepositoryObservationTx(ctx, auth.tx, cfg, op, o); err != nil {
		return out, err
	}
	applied, found, err := s.codeCandidateNodeResultTx(ctx, auth, a.ProjectID, op.RunID, op.NodeID)
	if err != nil {
		return out, err
	}
	if !found || !codeCandidateResultIdentity(applied, op.RunID) || applied.Spec.Type != "external_workflow" || applied.Spec.Config["purpose"] != "repository_integration" {
		return out, conflict(applicationSource)
	}
	actual, err := s.repositoryResultObservationTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, op, applied.Result)
	if err != nil {
		return out, err
	}
	if actual == nil || actual.ObservationSHA256 != o.ObservationSHA256 || actual.ReceiptSHA256 != o.ReceiptSHA256 {
		return out, conflict(applicationSource)
	}
	if err = s.checkRepositoryBasisTx(ctx, auth, plan.Basis); err != nil {
		return out, err
	}
	valid, _, err := s.currentCodeCandidateEvidenceTx(ctx, auth, a.ProjectID, plan.Basis.RunID)
	if err != nil {
		return out, err
	}
	if !valid {
		return out, conflict(applicationSource)
	}
	out.Source = ApplicationVersionSource{Scope: scope, IntegrationPlanID: plan.ID, IntegrationPlanVersion: plan.Version, OperationID: op.Execution.OperationID, ReceiptSHA256: o.ReceiptSHA256, ReceiptOID: o.ReceiptOID, Commit: o.Receipt.ResultCommit, TaskBasis: plan.Basis.AcceptedTaskBasis, SourceNodeID: plan.Basis.SourceNodeID}
	out.Request = ri.RepositorySourceInspectionRequest{Protocol: ri.RepositorySourceInspectionProtocol, TenantID: auth.target.tenantID, ProjectID: a.ProjectID, ProviderID: scope.ProviderID, TargetID: scope.TargetID, TargetAlias: scope.TargetAlias, TargetRef: scope.TargetRef, TargetConfigSHA256: scope.ConfigSHA256, OperationID: op.Execution.OperationID, RequestSHA256: op.RequestSHA256, ReceiptSHA256: o.ReceiptSHA256, ExactCommit: o.Receipt.ResultCommit, RelativeRoots: []string{scope.RelativeRoot}}
	out.TargetVersion = target.Version
	return
}
func (s *Service) applicationPreviewBasisTx(ctx context.Context, auth *authorizedCommand, project, id string, q ApplicationVersionPreviewRequest) (a ProjectApplication, basis []applicationSourceBasis, err error) {
	a, err = s.projectApplicationTx(ctx, auth, project, id)
	if err != nil {
		return
	}
	if a.Version != q.ExpectedApplicationVersion {
		return a, nil, conflict(applicationChanged)
	}
	if a.State != "active" {
		return a, nil, conflict(applicationScope)
	}
	if _, err = s.applicationTeamTx(ctx, auth, project, a.OwnerTeamID); err != nil {
		return
	}
	if len(q.Sources) != len(a.CodeScopes) {
		return a, nil, invalid(applicationInvalid)
	}
	basis = []applicationSourceBasis{}
	for _, input := range q.Sources {
		b, e := s.applicationSourceBasisTx(ctx, auth, a, input)
		if e != nil {
			return a, nil, e
		}
		basis = append(basis, b)
	}
	sort.Slice(basis, func(i, j int) bool { return basis[i].Source.Scope.ID < basis[j].Source.Scope.ID })
	return
}
func applicationBasisDigest(a ProjectApplication, b []applicationSourceBasis) string {
	return digest(stringify([]any{a, b}))
}

// Inspection is outside database/authority transactions. Every request is fixed
// to a receipt commit; branch heads and provider timestamps are not a baseline.
func (s *Service) applicationInspect(ctx context.Context, basis []applicationSourceBasis) ([]ApplicationVersionSource, error) {
	raw := []ri.ProviderSourceInspection{}
	var bytes int64
	count := 0
	for _, b := range basis {
		x, err := s.readApplicationSource(ctx, b.Request)
		if err != nil {
			return nil, err
		}
		if err = ri.ValidateProviderSourceInspection(b.Request, x); err != nil {
			return nil, conflict(applicationInspection)
		}
		bytes += x.Coverage.ContentBytes
		count += x.Coverage.ManifestCount
		if bytes > ri.RepositorySourceMaxContentBytes || count > ri.RepositorySourceMaxManifests {
			return nil, invalid(applicationInspection)
		}
		raw = append(raw, x)
	}
	parsed, err := s.parseApplicationSources(ctx, raw)
	if err != nil {
		return nil, err
	}
	if len(parsed) != len(basis) {
		return nil, conflict(applicationInspection)
	}
	out := []ApplicationVersionSource{}
	used := map[int]bool{}
	for i, b := range basis {
		found := -1
		for j, p := range parsed {
			if !used[j] && p.SnapshotSHA256 == raw[i].SnapshotSHA256 && p.RelativeRoot == b.Source.Scope.RelativeRoot {
				found = j
				break
			}
		}
		if found < 0 {
			return nil, conflict(applicationInspection)
		}
		p := parsed[found]
		used[found] = true
		if p.Commit != b.Source.Commit || p.TreeOID != raw[i].TreeOID || !gitRevision.MatchString(p.ScopeTreeOID) || !contentDigest.MatchString(p.ParserSHA256) || p.Manifests == nil || len(p.Manifests) > ri.RepositorySourceMaxManifests || p.Coverage.ManifestCount != len(p.Manifests) || p.Coverage.MaxManifests != ri.RepositorySourceMaxManifests {
			return nil, conflict(applicationInspection)
		}
		expectedTree := ""
		for _, scope := range raw[i].Scopes {
			if scope.RelativeRoot == p.RelativeRoot {
				expectedTree = scope.TreeOID
			}
		}
		if expectedTree == "" || p.ScopeTreeOID != expectedTree {
			return nil, conflict(applicationInspection)
		}
		for _, m := range p.Manifests {
			valid := false
			for _, original := range raw[i].Manifests {
				if original.Path == m.Path && original.SHA256 == m.SHA256 && original.BlobOID == m.BlobOID {
					valid = true
					break
				}
			}
			if !valid || m.Dependencies == nil {
				return nil, conflict(applicationInspection)
			}
			for _, d := range m.Dependencies {
				if !contentDigest.MatchString(d.DeclaredRangeSHA256) || d.RangeRedacted && d.DeclaredRange != "" {
					return nil, conflict(applicationInspection)
				}
			}
		}
		source := b.Source
		source.Inspection = p
		out = append(out, source)
	}
	return out, nil
}
func (s *Service) applicationVersionViewTx(ctx context.Context, auth *authorizedCommand, v ApplicationCodeVersion) (out ApplicationVersionView, err error) {
	out = ApplicationVersionView{Version: v, BasisCurrent: true, Blockers: []string{}}
	app, err := s.projectApplicationTx(ctx, auth, v.ProjectID, v.ApplicationID)
	if err != nil {
		return out, err
	}
	if app.State != "active" {
		out.BasisCurrent = false
		out.Blockers = append(out.Blockers, "application_archived")
	}
	// Historical code remains immutable, while accountability and hierarchy are
	// today's governance facts. An archived team cannot remain falsely current
	// merely because the caller is restoring a previously successful receipt.
	governanceCurrent := app.OwnerTeamID == v.Application.OwnerTeamID && app.ParentApplicationID == v.Application.ParentApplicationID && app.Kind == v.Application.Kind
	if _, e := s.applicationTeamTx(ctx, auth, v.ProjectID, app.OwnerTeamID); e != nil {
		if !expertTeamExpectedError(e) {
			return out, e
		}
		governanceCurrent = false
	}
	if !governanceCurrent {
		out.BasisCurrent = false
		out.Blockers = append(out.Blockers, "application_governance_changed")
	}
	for _, source := range v.Sources {
		b, e := s.applicationSourceBasisTx(ctx, auth, v.Application, ApplicationVersionSourceInput{ScopeID: source.Scope.ID, IntegrationPlanID: source.IntegrationPlanID, ExpectedIntegrationVersion: source.IntegrationPlanVersion})
		if e != nil {
			if expertTeamExpectedError(e) {
				out.BasisCurrent = false
				out.Blockers = append(out.Blockers, "application_quality_stale")
				continue
			}
			return out, e
		}
		b.Source.Inspection = source.Inspection
		if stringify(b.Source) != stringify(source) {
			out.BasisCurrent = false
			out.Blockers = append(out.Blockers, "application_source_changed")
		}
	}
	// Catalog edits do not mutate registered historical ownership; expose drift.
	if stringify(app.CodeScopes) != stringify(v.Application.CodeScopes) {
		out.BasisCurrent = false
		out.Blockers = append(out.Blockers, "application_scope_changed")
	}
	return
}
func applicationWithinRoot(path, root string) bool {
	return root == "" || path == root || strings.HasPrefix(path, root+"/")
}
