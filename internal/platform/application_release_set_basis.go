package platform

import (
	"context"
	"database/sql"
	"sort"
)

func applicationReleaseSetInputValid(project string, q ApplicationReleaseSetPreviewRequest) bool {
	if q.ProjectID != project || !handoffID(project, false) || !testPlanText(q.Label, 64, true) || !testPlanText(q.Reason, 2000, true) || q.Members == nil || len(q.Members) < 1 || len(q.Members) > 16 {
		return false
	}
	apps := map[string]bool{}
	for _, m := range q.Members {
		if !handoffID(m.ApplicationID, false) || !handoffID(m.ApplicationVersionID, false) || !contentDigest.MatchString(m.ApplicationVersionSHA256) || !handoffID(m.CandidateID, false) || apps[m.ApplicationID] {
			return false
		}
		apps[m.ApplicationID] = true
	}
	return true
}
func applicationReleaseSetBlock(code, app, ref string) ApplicationReleaseSetBlocker {
	return ApplicationReleaseSetBlocker{Code: code, ApplicationID: app, RefID: ref}
}
func applicationReleaseSetDependencyClosure(versions map[string]ApplicationCodeVersion, inputs map[string]ApplicationReleaseSetMemberInput, acknowledged bool) (dependencies []ApplicationReleaseSetDependency, cycles [][]string, blockers []ApplicationReleaseSetBlocker) {
	dependencies = []ApplicationReleaseSetDependency{}
	blockers = []ApplicationReleaseSetBlocker{}
	nodes := []ApplicationGraphNode{}
	edges := []ApplicationGraphEdge{}
	ids := []string{}
	for id := range inputs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		nodes = append(nodes, ApplicationGraphNode{ApplicationID: id})
		v, ok := versions[id]
		if !ok {
			continue
		}
		for _, d := range v.Dependencies {
			dependencies = append(dependencies, ApplicationReleaseSetDependency{ApplicationID: id, ApplicationVersionID: v.ID, TargetApplicationID: d.TargetApplicationID, TargetVersionID: d.TargetVersionID, TargetVersionSHA256: d.TargetVersionSHA256, Compatibility: d.Compatibility})
			edges = append(edges, ApplicationGraphEdge{SourceApplicationID: id, TargetApplicationID: d.TargetApplicationID})
			target, ok := inputs[d.TargetApplicationID]
			if !ok {
				blockers = append(blockers, applicationReleaseSetBlock("release_set_dependency_missing", id, d.TargetApplicationID))
			} else if target.ApplicationVersionID != d.TargetVersionID || target.ApplicationVersionSHA256 != d.TargetVersionSHA256 {
				blockers = append(blockers, applicationReleaseSetBlock("release_set_dependency_conflict", id, d.TargetApplicationID))
			}
			if d.Compatibility == "incompatible" {
				blockers = append(blockers, applicationReleaseSetBlock("release_set_dependency_incompatible", id, d.TargetApplicationID))
			} else if d.Compatibility != "compatible" && !acknowledged {
				blockers = append(blockers, applicationReleaseSetBlock("release_set_compatibility_unverified", id, d.TargetApplicationID))
			}
		}
	}
	sort.Slice(dependencies, func(i, j int) bool { return stringify(dependencies[i]) < stringify(dependencies[j]) })
	cycles = applicationGraphCycles(nodes, edges)
	return
}
func (s *Service) applicationReleaseSetPreviewTx(ctx context.Context, tx *sql.Tx, p Principal, project string, q ApplicationReleaseSetPreviewRequest) (out ApplicationReleaseSetPreview, err error) {
	out = ApplicationReleaseSetPreview{Set: ApplicationReleaseSet{ProjectID: project, Label: q.Label, Reason: q.Reason, Members: []ApplicationReleaseSetMember{}, Dependencies: []ApplicationReleaseSetDependency{}, Cycles: [][]string{}, UnverifiedCompatibilityAcknowledged: q.AcknowledgeUnverifiedCompatibility}, CanRegister: true, Blockers: []ApplicationReleaseSetBlocker{}}
	versions := map[string]ApplicationCodeVersion{}
	inputs := map[string]ApplicationReleaseSetMemberInput{}
	candidates, artifacts := map[string]bool{}, map[string]bool{}
	block := func(code, app, ref string) {
		out.Blockers = append(out.Blockers, applicationReleaseSetBlock(code, app, ref))
		out.CanRegister = false
	}
	ordered := append([]ApplicationReleaseSetMemberInput{}, q.Members...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ApplicationID < ordered[j].ApplicationID })
	for _, input := range ordered {
		inputs[input.ApplicationID] = input
		m := ApplicationReleaseSetMember{ApplicationReleaseSetMemberInput: input, SourceTasks: []ReleaseTaskBasis{}, CandidateTasks: []ReleaseTaskBasis{}}
		v, e := s.applicationVersionReadTx(ctx, tx, p, project, input.ApplicationID, input.ApplicationVersionID)
		if e != nil {
			if !expertTeamExpectedError(e) {
				return out, e
			}
			block("release_set_application_changed", input.ApplicationID, input.ApplicationVersionID)
		} else {
			m.ApplicationName = v.Application.Name
			m.ApplicationVersionLabel = v.Label
			if v.SnapshotSHA256 != input.ApplicationVersionSHA256 {
				block("release_set_application_changed", input.ApplicationID, v.ID)
			} else {
				versions[input.ApplicationID] = v
			}
			view, e := s.applicationVersionViewReadTx(ctx, tx, p, v)
			if e != nil {
				if !expertTeamExpectedError(e) {
					return out, e
				}
				block("release_set_quality_stale", input.ApplicationID, v.ID)
			} else if !view.BasisCurrent {
				block("release_set_quality_stale", input.ApplicationID, v.ID)
			}
			origin := ""
			sourceTasks := map[string]ReleaseTaskBasis{}
			for _, source := range v.Sources {
				key := digest(stringify([]string{source.Scope.TargetID, source.Scope.ProviderID, source.Scope.TargetAlias, source.Scope.TargetRef, source.Scope.ConfigSHA256}))
				if origin == "" {
					origin = key
					m.TargetID = source.Scope.TargetID
					m.SourceCommit = source.Commit
				} else if origin != key || m.SourceCommit != source.Commit {
					block("release_set_source_unsupported", input.ApplicationID, source.Scope.ID)
				}
				if prior, ok := sourceTasks[source.TaskBasis.ID]; ok && prior != source.TaskBasis {
					block("release_set_quality_stale", input.ApplicationID, source.TaskBasis.ID)
				}
				sourceTasks[source.TaskBasis.ID] = source.TaskBasis
			}
			if len(v.Sources) == 0 || m.SourceCommit == "" {
				block("release_set_source_unsupported", input.ApplicationID, v.ID)
			}
			for _, b := range sourceTasks {
				m.SourceTasks = append(m.SourceTasks, b)
			}
			sort.Slice(m.SourceTasks, func(i, j int) bool { return m.SourceTasks[i].ID < m.SourceTasks[j].ID })
			if !v.DependencyCoverageComplete && !q.AcknowledgeUnverifiedCompatibility {
				block("release_set_compatibility_unverified", input.ApplicationID, v.ID)
			}
		}
		candidate, e := s.validateCurrentReleaseCandidateTx(ctx, tx, p, project, input.CandidateID)
		if e != nil {
			if !expertTeamExpectedError(e) {
				return out, e
			}
			block("release_set_candidate_stale", input.ApplicationID, input.CandidateID)
		} else {
			m.CandidateName = candidate.Name
			m.CandidateSHA256 = digest(stringify(candidate))
			m.ArtifactID = candidate.ArtifactID
			m.ArtifactSHA256 = candidate.ArtifactSHA256
			m.BuildProof = candidate.BuildProof
			m.PackageTestProof = candidate.PackageTestProof
			m.CandidateTasks = append([]ReleaseTaskBasis{}, candidate.Tasks...)
			// The candidate's own accepted build delivery is independent of the
			// original code-source tasks. Both sets are preserved without rebinding.
			build, e := s.currentArtifactProofTx(ctx, tx, p, project, candidate.BuildAttestationID)
			if e != nil {
				return out, e
			}
			test, e := s.currentArtifactProofTx(ctx, tx, p, project, candidate.PackageTestAttestationID)
			if e != nil {
				return out, e
			}
			if stringify(build) != stringify(candidate.BuildProof) || stringify(test) != stringify(candidate.PackageTestProof) {
				block("release_set_candidate_stale", input.ApplicationID, candidate.ID)
			}
			if candidate.SourceCommit != m.SourceCommit || build.ApplicationBuild != nil && !applicationBuildMatchesVersion(build.ApplicationBuild, v) {
				block("release_set_source_mismatch", input.ApplicationID, candidate.ID)
			}
			if build.Assignment.WorkerID == "" || build.Assignment.WorkerID == test.Assignment.WorkerID || build.ArtifactID != test.ArtifactID || build.ManifestSHA256 != test.ManifestSHA256 || build.SourceCommit != test.SourceCommit {
				block("release_set_independent_test_required", input.ApplicationID, candidate.ID)
			}
			if candidates[candidate.ID] || artifacts[candidate.ArtifactID] {
				block("release_set_reused_candidate", input.ApplicationID, candidate.ID)
			}
			candidates[candidate.ID] = true
			artifacts[candidate.ArtifactID] = true
		}
		out.Set.Members = append(out.Set.Members, m)
	}
	dependencies, cycles, dependencyBlockers := applicationReleaseSetDependencyClosure(versions, inputs, q.AcknowledgeUnverifiedCompatibility)
	out.Set.Dependencies, out.Set.Cycles = dependencies, cycles
	out.Blockers = append(out.Blockers, dependencyBlockers...)
	if len(dependencyBlockers) > 0 {
		out.CanRegister = false
	}
	out.Set.DependencyClosureSHA256 = digest(stringify([]any{out.Set.Dependencies, out.Set.Cycles}))
	out.Set.SnapshotSHA256 = applicationReleaseSetDigest(out.Set)
	out.PreviewSHA256 = digest(stringify([]any{"application-release-set-preview-v1", out.Set.SnapshotSHA256}))
	return
}

// Recompute current facts without changing the registered historical snapshot.
func (s *Service) applicationReleaseSetBasisTx(ctx context.Context, tx *sql.Tx, p Principal, set ApplicationReleaseSet) ([]ApplicationReleaseSetBlocker, error) {
	inputs := []ApplicationReleaseSetMemberInput{}
	for _, m := range set.Members {
		inputs = append(inputs, m.ApplicationReleaseSetMemberInput)
	}
	current, err := s.applicationReleaseSetPreviewTx(ctx, tx, p, set.ProjectID, ApplicationReleaseSetPreviewRequest{ProjectID: set.ProjectID, Label: set.Label, Members: inputs, AcknowledgeUnverifiedCompatibility: set.UnverifiedCompatibilityAcknowledged, Reason: set.Reason})
	if err != nil {
		return nil, err
	}
	if current.CanRegister && applicationReleaseSetDraftDigest(current.Set) != applicationReleaseSetDraftDigest(set) {
		current.Blockers = append(current.Blockers, applicationReleaseSetBlock("release_set_quality_stale", "", set.ID))
	}
	return current.Blockers, nil
}
