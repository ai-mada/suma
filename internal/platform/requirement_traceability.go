package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// GetRequirementTraceability reads one current, authorized requirement and its
// complete published proposal in a single project-coordinated transaction.
func (s *Service) GetRequirementTraceability(ctx context.Context, actor Actor, project, requirement string) (RequirementTraceability, error) {
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, requirement, purposeProjectRead)
	if err != nil {
		return RequirementTraceability{}, err
	}
	defer close()
	if actor.kind != actorBrowser && actor.kind != actorProjectAPI {
		return RequirementTraceability{}, forbidden()
	}
	out, err := s.requirementTraceabilityTx(ctx, auth, project, requirement)
	if err != nil {
		return RequirementTraceability{}, err
	}
	if err = auth.assertTx(auth.tx, auth.target, auth.purpose); err != nil {
		return RequirementTraceability{}, err
	}
	return out, nil
}

func (s *Service) requirementTraceabilityTx(ctx context.Context, auth *authorizedCommand, project, requirement string) (out RequirementTraceability, err error) {
	source, err := s.getRecordTx(ctx, auth, project, "requirements", requirement)
	if err != nil {
		return out, err
	}
	aggregate, err := s.designAggregateTx(ctx, auth, project, requirement)
	if err != nil {
		return out, err
	}
	if len(source.AcceptanceCriteria) > 100 {
		return out, fmt.Errorf("requirement criteria exceed the current record contract")
	}
	out = RequirementTraceability{ProjectID: project, RequirementID: source.ID, RequirementVersion: source.Version, RequirementStatus: source.Status, DesignVersion: aggregate.Version, PublishedRevision: aggregate.Published, Eligibility: "none", Criteria: []RequirementTraceabilityCriterion{}, Tasks: []RequirementTraceabilityTask{}}
	for i, criterion := range source.AcceptanceCriteria {
		out.Criteria = append(out.Criteria, RequirementTraceabilityCriterion{Index: i + 1, Text: criterion, TaskKeys: []string{}})
	}
	var published RequirementDesignRevision
	if aggregate.Published > 0 {
		published, err = s.designRevisionTx(ctx, auth, project, requirement, aggregate.Published)
		if err != nil {
			return out, err
		}
		if published.State != "published" {
			return out, fmt.Errorf("requirement design publication is inconsistent")
		}
		out.DesignSHA256 = published.SHA256
	}
	// Covers belongs to the immutable source revision. An old index must never
	// be applied to a newly edited criterion at the same list position.
	switch {
	case source.Status == "archived" || source.Status == "deleted":
		out.Eligibility = "source_unavailable"
	case aggregate.State == "retired":
		out.Eligibility = "retired"
	case aggregate.Published == 0:
		return out, nil
	case !designSourceCurrent(source, published.Requirement):
		out.Eligibility = "requirement_changed"
	default:
		out.Eligibility = "current"
	}
	if out.Eligibility != "current" {
		return out, nil
	}
	if err = validateDesignDraft(published.Content, published.Tasks); err != nil {
		return out, fmt.Errorf("published requirement design is invalid: %w", err)
	}
	if len(designDiagnostics(published)) != 0 {
		return out, fmt.Errorf("published requirement design is incomplete")
	}
	links, err := s.designLinksTx(ctx, auth, project, requirement)
	if err != nil {
		return out, err
	}
	byKey := make(map[string]designTaskLink, len(links))
	for _, link := range links {
		byKey[link.Key] = link
	}
	for _, proposal := range published.Tasks {
		link, found := byKey[proposal.Key]
		task, e := s.requirementTraceabilityTaskTx(ctx, auth, project, requirement, proposal, link, found)
		if e != nil {
			return out, e
		}
		out.Tasks = append(out.Tasks, task)
		for _, index := range proposal.Covers {
			out.Criteria[index-1].TaskKeys = append(out.Criteria[index-1].TaskKeys, proposal.Key)
		}
	}
	return out, nil
}

func (s *Service) requirementTraceabilityTaskTx(ctx context.Context, auth *authorizedCommand, project, requirement string, proposal RequirementDesignTask, link designTaskLink, found bool) (out RequirementTraceabilityTask, err error) {
	out = RequirementTraceabilityTask{Key: proposal.Key, Title: proposal.Title, Covers: append([]int{}, proposal.Covers...), BindingState: "not_adopted", RunBasisState: "not_dispatched"}
	if !found {
		return out, nil
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", link.TaskID)
	if err != nil {
		if traceabilityBusinessAbsence(err) {
			out.BindingState, out.RunBasisState = "source_unavailable", "unknown"
			return out, nil
		}
		return out, err
	}
	out.TaskID, out.TaskVersion, out.TaskStatus = task.ID, task.Version, task.Status
	view, err := s.taskDesignContextViewTx(ctx, auth, project, task.ID)
	if err != nil {
		return out, err
	}
	out.BindingState = view.Eligibility
	if !view.Bound || view.Context == nil || view.Context.RequirementID != requirement || view.Context.TaskKey != proposal.Key {
		out.BindingState = "source_unavailable"
	}
	if task.RunID == "" {
		return out, nil
	}
	var run TaskDeliveryRun
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, task.RunID).Scan(&run.RunID, &run.Version, &run.State)
	if errors.Is(err, sql.ErrNoRows) {
		out.RunBasisState = "unknown"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	basis, ok := task.Data["run_task_basis"].(string)
	switch {
	case !ok || basis == "":
		out.RunBasisState = "unknown"
	case basis != taskBasis(task):
		out.RunBasisState = "changed"
	default:
		out.RunBasisState = "current"
	}
	if run.Nodes, run.Tests, err = s.taskDeliveryNodesTx(ctx, auth, run.RunID); err != nil {
		return out, err
	}
	out.Run = &run
	if out.BindingState != "current" || out.RunBasisState != "current" {
		return out, nil
	}
	if err = s.checkTaskRequirementDesignRunTx(ctx, auth, project, task.ID, run.RunID, true); err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 400 || api.Status == 404 || api.Status == 409) {
			out.RunBasisState = "changed"
			return out, nil
		}
		return out, err
	}
	out.FormalAcceptance, _, err = s.expertTeamFormalAcceptanceTx(ctx, auth.tx, auth.target.tenantID, project, task.ID, run.RunID)
	return out, err
}

func traceabilityBusinessAbsence(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Status == 404
}

func (s *Server) requirementTraceabilityHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) != 3 || parts[0] != "requirements" || parts[2] != "traceability" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 || len(values) != 1 || len(values["project_id"]) != 1 || values.Get("project_id") == "" || values.Get("project_id") != project {
		writeError(w, invalid("Invalid requirement traceability query"))
		return true
	}
	out, err := s.Service.GetRequirementTraceability(r.Context(), actor, project, parts[1])
	respond(w, http.StatusOK, out, err)
	return true
}
