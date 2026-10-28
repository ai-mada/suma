package platform

import (
	"context"
	"encoding/json"
	"sort"
)

// A replay restores the original immutable code version, then computes current
// read facts. It performs no provider IO, parsing, insertion or pointer advance.
func (s *Service) applicationRegisterReplayTx(ctx context.Context, auth *authorizedCommand, project, appID, raw string) (out ApplicationVersionView, err error) {
	var receipt ApplicationVersionView
	if err = json.Unmarshal([]byte(raw), &receipt); err != nil {
		return
	}
	v, err := s.applicationVersionTx(ctx, auth, project, appID, receipt.Version.ID)
	if err != nil {
		return out, err
	}
	if receipt.Version.ApplicationID != appID || receipt.Version.ProjectID != project || receipt.Version.SnapshotSHA256 != v.SnapshotSHA256 || receipt.Version.RegisteredPreviewSHA256 != v.RegisteredPreviewSHA256 {
		return out, conflict(applicationChanged)
	}
	return s.applicationVersionViewTx(ctx, auth, v)
}

// Observations remain parser facts. Human selected immutable dependency versions
// form an explicit overlay and never rewrite an original manifest declaration.
func (s *Service) applicationDependenciesTx(ctx context.Context, auth *authorizedCommand, project string, sources []ApplicationVersionSource, inputs []ApplicationDependencyInput) (observed []ApplicationDependencyObservation, deps []ApplicationDependency, complete bool, err error) {
	observed = []ApplicationDependencyObservation{}
	deps = []ApplicationDependency{}
	complete = true
	names := map[string][]string{}
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT a.id,v.payload FROM suma_project_applications a JOIN suma_application_versions v ON v.tenant_id=a.tenant_id AND v.project_id=a.project_id AND v.application_id=a.id AND v.id=a.current_version_id WHERE a.tenant_id=? AND a.project_id=? AND a.state='active' ORDER BY a.id LIMIT 101`), auth.target.tenantID, project)
	if e != nil {
		err = e
		return
	}
	bytes, count := 0, 0
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			break
		}
		count++
		bytes += len(raw)
		if count > 100 || bytes > 4<<20 {
			complete = false
			break
		}
		var v ApplicationCodeVersion
		if json.Unmarshal([]byte(raw), &v) != nil || v.ApplicationID != id || v.ProjectID != project || applicationVersionDigest(v) != v.SnapshotSHA256 {
			err = conflict(applicationChanged)
			break
		}
		seen := map[string]bool{}
		for _, source := range v.Sources {
			for _, m := range source.Inspection.Manifests {
				key := m.Ecosystem + ":" + m.PackageName
				if m.PackageName != "" && !seen[key] {
					names[key] = append(names[key], id)
					seen[key] = true
				}
			}
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return
	}
	for _, source := range sources {
		if !source.Inspection.Coverage.Complete {
			complete = false
		}
		for _, m := range source.Inspection.Manifests {
			if !m.Complete {
				complete = false
			}
			for _, d := range m.Dependencies {
				ids := []string{}
				state := "unknown"
				if !d.RangeRedacted && d.RangeState == "valid" {
					ids = append(ids, names[m.Ecosystem+":"+d.Name]...)
					switch len(ids) {
					case 0:
						state = "external"
					case 1:
						state = "matched"
					default:
						state = "ambiguous"
					}
				}
				sort.Strings(ids)
				o := ApplicationDependencyObservation{ScopeID: source.Scope.ID, ManifestPath: m.Path, ManifestSHA256: m.SHA256, Parser: m.Parser, ParserVersion: m.ParserVersion, Ecosystem: m.Ecosystem, Name: d.Name, DeclaredRange: d.DeclaredRange, Kind: d.Kind, MatchState: state, RangeState: d.RangeState, Compatibility: d.Compatibility, RangeRedacted: d.RangeRedacted, DeclaredRangeSHA256: d.DeclaredRangeSHA256, CandidateApplicationIDs: ids}
				o.ID = digest(stringify(o))
				observed = append(observed, o)
			}
		}
	}
	sort.Slice(observed, func(i, j int) bool { return observed[i].ID < observed[j].ID })
	byID := map[string]ApplicationDependencyObservation{}
	for _, o := range observed {
		byID[o.ID] = o
	}
	covered := map[string]bool{}
	for _, input := range inputs {
		target, e := s.projectApplicationTx(ctx, auth, project, input.TargetApplicationID)
		if e != nil {
			err = e
			return
		}
		if target.State != "active" {
			err = conflict(applicationChanged)
			return
		}
		v, e := s.applicationVersionTx(ctx, auth, project, target.ID, input.TargetVersionID)
		if e != nil {
			err = e
			return
		}
		if v.SnapshotSHA256 != input.TargetVersionSHA256 {
			err = conflict(applicationChanged)
			return
		}
		d := ApplicationDependency{ApplicationDependencyInput: input, Compatibility: "unverified"}
		if input.ObservationID != "" {
			o, ok := byID[input.ObservationID]
			if !ok || covered[o.ID] {
				err = invalid(applicationInvalid)
				return
			}
			d.ObservedDeclaredRange = o.DeclaredRange
			covered[o.ID] = true
		}
		// No local comparison guesses ecosystem semantics. Exact references are
		// valid human declarations; source range compatibility stays unverified.
		deps = append(deps, d)
	}
	for _, o := range observed {
		if !covered[o.ID] {
			complete = false
		}
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].TargetApplicationID < deps[j].TargetApplicationID })
	return
}
func (s *Service) applicationBuildPreviewTx(ctx context.Context, auth *authorizedCommand, a ProjectApplication, basis []applicationSourceBasis, sources []ApplicationVersionSource, q ApplicationVersionPreviewRequest) (out ApplicationVersionPreview, err error) {
	observations, deps, complete, err := s.applicationDependenciesTx(ctx, auth, a.ProjectID, sources, q.Dependencies)
	if err != nil {
		return out, err
	}
	v := ApplicationCodeVersion{ProjectID: a.ProjectID, ApplicationID: a.ID, Label: q.Label, Application: a, Sources: sources, Observations: observations, Dependencies: deps, DependencyCoverageComplete: complete, DependencyGapsAcknowledged: q.AcknowledgeDependencyGaps, Reason: q.Reason}
	v.SnapshotSHA256 = applicationVersionDigest(v)
	out = ApplicationVersionPreview{Version: v, CanRegister: true, Blockers: []string{}}
	out.PreviewSHA256 = digest(stringify([]any{"application-preview-v1", v.SnapshotSHA256, applicationBasisDigest(a, basis)}))
	if !complete && !q.AcknowledgeDependencyGaps {
		out.CanRegister = false
		out.Blockers = append(out.Blockers, "application_dependency_gaps")
	}
	return
}

func (s *Service) PreviewApplicationVersion(ctx context.Context, actor Actor, project, id string, q ApplicationVersionPreviewRequest) (out ApplicationVersionPreview, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(id, false) || !applicationPreviewValid(project, q) {
		return out, invalid(applicationInvalid)
	}
	bounded, auth, close, err := s.applicationRead(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	a, basis, err := s.applicationPreviewBasisTx(bounded, auth, project, id, q)
	close()
	if err != nil {
		return out, err
	}
	sources, err := s.applicationInspect(ctx, basis)
	if err != nil {
		return out, err
	}
	bounded, auth, close, err = s.applicationRead(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	defer close()
	current, currentBasis, err := s.applicationPreviewBasisTx(bounded, auth, project, id, q)
	if err != nil {
		return out, err
	}
	if applicationBasisDigest(a, basis) != applicationBasisDigest(current, currentBasis) {
		return out, conflict(applicationChanged)
	}
	return s.applicationBuildPreviewTx(bounded, auth, current, currentBasis, sources, q)
}
func (s *Service) RegisterApplicationVersion(ctx context.Context, actor Actor, project, id string, q ApplicationVersionRegisterRequest) (out ApplicationVersionView, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if !handoffID(id, false) || !applicationPreviewValid(project, q.ApplicationVersionPreviewRequest) || !planningKey(q.IdempotencyKey) || !contentDigest.MatchString(q.ExpectedPreviewSHA256) {
		return out, invalid(applicationInvalid)
	}
	// Restore the original command only after rechecking today's authorization.
	// A lost response therefore cannot repeat provider IO or register a second row.
	bounded, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	request := map[string]any{"application_register": id, "request": q}
	hash, replay, err := s.commandReceipt(bounded, auth, q.IdempotencyKey, request)
	if err != nil {
		close()
		return out, err
	}
	if replay {
		out, err = s.applicationRegisterReplayTx(bounded, auth, project, id, hash)
		close()
		return
	}
	a, basis, err := s.applicationPreviewBasisTx(bounded, auth, project, id, q.ApplicationVersionPreviewRequest)
	close()
	if err != nil {
		return out, err
	}
	sources, err := s.applicationInspect(ctx, basis)
	if err != nil {
		return out, err
	}
	bounded, auth, close, err = s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err = s.commandReceipt(bounded, auth, q.IdempotencyKey, request)
	if err != nil {
		return out, err
	}
	if replay {
		return s.applicationRegisterReplayTx(bounded, auth, project, id, hash)
	}
	current, currentBasis, err := s.applicationPreviewBasisTx(bounded, auth, project, id, q.ApplicationVersionPreviewRequest)
	if err != nil {
		return out, err
	}
	if applicationBasisDigest(a, basis) != applicationBasisDigest(current, currentBasis) {
		return out, conflict(applicationChanged)
	}
	preview, err := s.applicationBuildPreviewTx(bounded, auth, current, currentBasis, sources, q.ApplicationVersionPreviewRequest)
	if err != nil {
		return out, err
	}
	if !preview.CanRegister {
		return out, conflict(applicationDependencies)
	}
	if preview.PreviewSHA256 != q.ExpectedPreviewSHA256 {
		return out, conflict(applicationChanged)
	}
	var count, sequence int
	if err = auth.tx.QueryRowContext(bounded, s.q(`SELECT COUNT(*) FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND application_id=? AND label_key=?`), auth.target.tenantID, project, id, digest(q.Label)).Scan(&count); err != nil {
		return
	}
	if count != 0 {
		return out, conflict(applicationChanged)
	}
	if err = auth.tx.QueryRowContext(bounded, s.q(`SELECT COALESCE(MAX(sequence),0) FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND application_id=?`), auth.target.tenantID, project, id).Scan(&sequence); err != nil {
		return
	}
	v := preview.Version
	v.ID = newID()
	v.Sequence = sequence + 1
	v.RegisteredBy = auth.currentPrincipal.ID
	v.RegisteredAt = now()
	v.RegisteredPreviewSHA256 = preview.PreviewSHA256
	v.SnapshotSHA256 = applicationVersionDigest(v)
	if _, err = auth.tx.ExecContext(bounded, s.q(`INSERT INTO suma_application_versions(id,tenant_id,project_id,application_id,sequence,label,label_key,snapshot_sha256,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?)`), v.ID, auth.target.tenantID, project, id, v.Sequence, v.Label, digest(v.Label), v.SnapshotSHA256, v.RegisteredAt, stringify(v)); err != nil {
		return
	}
	if err = s.saveApplicationGraphProjectionTx(bounded, auth, v); err != nil {
		return
	}
	previous := current.Version
	current.Version++
	current.CurrentRegisteredVersionID = v.ID
	current.UpdatedAt = v.RegisteredAt
	if err = s.saveProjectApplicationTx(bounded, auth, current, previous); err != nil {
		return
	}
	out = ApplicationVersionView{Version: v, BasisCurrent: true, Blockers: []string{}}
	if err = s.commandActivity(bounded, auth, "application.code_version_registered", v.ID, q.Reason); err != nil {
		return
	}
	if err = s.saveCommandReceipt(bounded, auth, q.IdempotencyKey, hash, out); err != nil {
		return
	}
	err = auth.tx.Commit()
	return
}
