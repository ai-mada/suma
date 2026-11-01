package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type continuationCursor struct {
	Scope    string `json:"scope"`
	Node     string `json:"node"`
	Transfer string `json:"transfer"`
}

func (s *Service) ListTaskDevelopmentContinuationOptions(ctx context.Context, actor Actor, project, task string, limit int, before string) (out TaskDevelopmentContinuationOptions, err error) {
	out = TaskDevelopmentContinuationOptions{ProjectID: project, TaskID: task, Items: []TaskDevelopmentContinuationOption{}, Complete: true}
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	out.Limit = limit
	if !handoffID(project, false) || !handoffID(task, false) || limit < 1 || limit > 100 || len(before) > 4096 {
		return out, invalid(continuationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	t, err := s.getRecordTx(ctx, auth, project, "tasks", task)
	if err != nil {
		return out, err
	}
	out.TaskVersion, out.SourceTaskRunID = t.Version, t.RunID
	if t.RunID != "" {
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT state,version FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, t.RunID).Scan(&out.SourceRunState, &out.SourceRunVersion)
		if err != nil {
			return out, err
		}
	}
	scope := digest(stringify([]any{auth.currentPrincipal.ID, auth.target.tenantID, project, task, t.Version, t.RunID, out.SourceRunVersion, limit}))
	var cursor continuationCursor
	if before != "" {
		raw, e := base64.RawURLEncoding.DecodeString(before)
		if e != nil || woc.Decode(raw, &cursor) != nil || cursor.Scope != scope || !diagnosisID(cursor.Node, 150) || !handoffID(cursor.Transfer, false) {
			return out, invalid(continuationInvalid)
		}
	}
	switch {
	case t.Status == "accepted":
		out.Reason = "task_accepted"
	case t.RunID == "":
		out.Reason = "no_run"
	case !continuationTerminal(out.SourceRunState):
		out.Reason = "run_not_terminal"
	}
	if out.Reason != "" {
		return out, nil
	}
	if _, err = s.continuationProvenanceTx(ctx, auth.tx, auth.target.tenantID, project, task, t.RunID); err != nil {
		return out, err
	}
	// Read every known current producer to count the full eligible set. SQL
	// uses original indexed scope/assignment columns; payloads remain authority.
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT c.binding,n.spec FROM suma_candidate_transfers c JOIN dag_nodes n ON n.run_id=c.run_id AND n.id=c.node_id AND n.attempt=c.attempt AND n.fence=c.fence AND n.worker_id=c.worker_id JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE c.tenant_id=? AND c.project_id=? AND c.run_id=? AND n.state='succeeded' AND a.state='succeeded' ORDER BY c.node_id,c.id`), auth.target.tenantID, project, t.RunID)
	if err != nil {
		return out, err
	}
	var candidates []TaskDevelopmentContinuationOption
	for rows.Next() {
		var raw, specRaw string
		var b cc.Binding
		var spec dag.Node
		if err = rows.Scan(&raw, &specRaw); err != nil {
			rows.Close()
			return out, err
		}
		if json.Unmarshal([]byte(raw), &b) != nil || json.Unmarshal([]byte(specRaw), &spec) != nil || !b.ValidSealed() || spec.Type != "agent" && spec.Type != "command" {
			continue
		}
		candidates = append(candidates, TaskDevelopmentContinuationOption{Selection: dc.DevelopmentContinuationSource{RunID: t.RunID, NodeID: b.Source.NodeID, Attempt: b.Source.Attempt, Fence: b.Source.Fence, ExpectedRunVersion: out.SourceRunVersion}, Candidate: b, NodeName: spec.Name})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, item := range candidates {
		if err = s.candidateSourceTx(ctx, auth.tx, candidateSubject{tenant: auth.target.tenantID, project: project}, item.Candidate); err != nil {
			var api *APIError
			if errors.As(err, &api) && api.Status >= 400 && api.Status < 500 {
				continue
			}
			return out, err
		}
		out.Total++
		if item.Selection.NodeID < cursor.Node || item.Selection.NodeID == cursor.Node && item.Candidate.TransferID <= cursor.Transfer {
			continue
		}
		if len(out.Items) <= limit {
			out.Items = append(out.Items, item)
		}
	}
	if len(out.Items) > limit {
		out.Complete = false
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		raw, _ := json.Marshal(continuationCursor{Scope: scope, Node: last.Selection.NodeID, Transfer: last.Candidate.TransferID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if out.Total == 0 {
		out.Reason = "no_sealed_candidate"
	}
	return out, nil
}

func (s *Server) taskDevelopmentContinuationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) != 3 || parts[0] != "tasks" || parts[2] != "development-continuation-options" {
		return false
	}
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(continuationInvalid)) }
	if r.Method != http.MethodGet || !handoffID(parts[1], false) || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q["project_id"]) != 1 || !handoffID(q.Get("project_id"), false) {
		bad()
		return true
	}
	for key, values := range q {
		if len(values) != 1 || key != "project_id" && key != "limit" && key != "before" {
			bad()
			return true
		}
	}
	limit := 0
	if values, present := q["limit"]; present {
		limit, err = strconv.Atoi(values[0])
		if err != nil || limit < 1 || limit > 100 {
			bad()
			return true
		}
	}
	if strings.TrimSpace(q.Get("before")) != q.Get("before") || len(q.Get("before")) > 4096 {
		bad()
		return true
	}
	project := q.Get("project_id")
	if scope == "api" && (boundProject == "" || boundProject != project) {
		writeError(w, forbidden())
		return true
	}
	out, err := s.Service.ListTaskDevelopmentContinuationOptions(r.Context(), actor, project, parts[1], limit, q.Get("before"))
	respond(w, http.StatusOK, out, err)
	return true
}
