package platform

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

const taskKnowledgeQuote = `Preserve tenant boundaries. 原文 <scope> "quoted" stays unchanged.`

// Knowledge is created and reviewed through the same HTTP entry points as the
// workbench. No fixture mutates SQL to fabricate publication or approval.
func taskKnowledgePublished(t *testing.T, f *fixture) (map[string]any, map[string]any) {
	t.Helper()
	doc := f.call("POST", "/documents", f.body(map[string]any{"title": "Reviewed architecture source", "description": "Architecture decision record.", "data": map[string]any{"content": taskKnowledgeQuote}}), 201)
	draft := f.call("POST", "/project-knowledge/extractions", f.body(map[string]any{"kind": "memory", "title": "Tenant boundary memory", "sources": []any{taskKnowledgeSource(doc)}}), 201)
	published := taskKnowledgeCommand(f, draft, "publish")
	return doc, published
}

func taskKnowledgeSource(doc map[string]any) map[string]any {
	return map[string]any{"kind": "documents", "id": doc["id"], "version": doc["version"], "quote": taskKnowledgeQuote}
}

func taskKnowledgeCommand(f *fixture, current map[string]any, action string) map[string]any {
	f.t.Helper()
	return f.call("POST", "/project-knowledge/"+current["id"].(string)+"/commands", map[string]any{"project_id": current["project_id"], "action": action, "expected_version": current["version"], "idempotency_key": newID()}, 200)
}

func taskKnowledgeReference(published map[string]any) map[string]any {
	return map[string]any{"id": published["id"], "revision": published["published_revision"], "sha256": published["published_sha256"]}
}

func taskKnowledgeEdit(f *fixture, current, doc map[string]any, content string) map[string]any {
	f.t.Helper()
	return f.call("PATCH", "/project-knowledge/"+current["id"].(string), f.body(map[string]any{"kind": "memory", "title": "A newer editable draft", "summary": "Not adopted until reviewed", "content": content, "sources": []any{taskKnowledgeSource(doc)}, "expected_version": current["version"]}), 200)
}

// Decode the exact task JSON sent to the agent. JSON escaping may change the
// serialized appearance of quotes and markup, but never the original text.
func taskKnowledgeGoal(t *testing.T, workflow map[string]any) (string, map[string]any) {
	t.Helper()
	for _, raw := range workflow["nodes"].([]any) {
		node := raw.(map[string]any)
		if node["id"] != "develop" {
			continue
		}
		goal := node["config"].(map[string]any)["goal"].(string)
		_, fixed, ok := strings.Cut(goal, "<task_context_json>\n")
		if !ok {
			t.Fatal("fixed task boundary missing")
		}
		body, _, ok := strings.Cut(fixed, "\n</task_context_json>")
		if !ok {
			t.Fatal("fixed task boundary is incomplete")
		}
		var context map[string]any
		if err := json.Unmarshal([]byte(body), &context); err != nil {
			t.Fatal(err)
		}
		return goal, context
	}
	t.Fatal("managed developer node missing")
	return "", nil
}

func taskKnowledgeSnapshot(t *testing.T, context map[string]any) map[string]any {
	t.Helper()
	raw, ok := context["knowledge"].([]any)
	if !ok || len(raw) != 1 {
		t.Fatalf("expected exactly one selected approved snapshot: %#v", context)
	}
	return raw[0].(map[string]any)
}

type taskKnowledgeEffects struct{ runs, plans, receipts, activities int }

func taskKnowledgeEffectCounts(t *testing.T, f *fixture) taskKnowledgeEffects {
	t.Helper()
	var out taskKnowledgeEffects
	for table, target := range map[string]*int{"dag_runs": &out.runs, "suma_task_dispatch_plans": &out.plans, "suma_operations": &out.receipts, "suma_activity": &out.activities} {
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestTaskKnowledgeDispatchKeepsPublishedSnapshotWhileDraftChanges(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	doc, published := taskKnowledgePublished(t, f)
	request["knowledge"] = []any{taskKnowledgeReference(published)}
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	approved := taskKnowledgeSnapshot(t, plan["context"].(map[string]any))
	if approved["content"] != taskKnowledgeQuote || approved["revision"] != published["published_revision"] || approved["sha256"] != published["published_sha256"] {
		t.Fatal("preview lost reviewed body/version/hash", approved)
	}
	source := approved["sources"].([]any)[0].(map[string]any)
	if source["id"] != doc["id"] || source["version"] != doc["version"] || len(source["sha256"].(string)) != 64 {
		t.Fatal("preview lost source provenance", source)
	}
	evidence := published["published"].(map[string]any)["sources"].([]any)[0].(map[string]any)
	if evidence["quote"] != taskKnowledgeQuote {
		t.Fatal("immutable revision lost original quote", evidence)
	}
	if _, repeated := source["quote"]; repeated {
		t.Fatal("consumed context repeats review quote", source)
	}
	previewGoal, goalContext := taskKnowledgeGoal(t, plan["workflow"].(map[string]any))
	if strings.Count(previewGoal, stringify(taskKnowledgeQuote)) != 1 {
		t.Fatal("approved text is duplicated or missing in the actual agent goal")
	}
	if stringify(taskKnowledgeSnapshot(t, goalContext)) != stringify(approved) {
		t.Fatal("agent goal differs from reviewed task context")
	}
	original := stringify(approved)

	const newDraft = "UNAPPROVED draft must never drift into a ready plan or running goal."
	edited := taskKnowledgeEdit(f, published, doc, newDraft)
	if edited["has_draft"] != true || edited["published_revision"] != published["published_revision"] {
		t.Fatal("draft edit replaced publication", edited)
	}
	reread := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if reread["valid"] != true || stringify(taskKnowledgeSnapshot(t, reread["context"].(map[string]any))) != original {
		t.Fatal("draft edit invalidated or rewrote old approved plan", reread)
	}
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	workflowPath := "/workflows/" + run["workflow_id"].(string) + "?project_id=" + f.project + "&version=" + strconv.Itoa(int(run["workflow_version"].(float64)))
	definition := f.call("GET", workflowPath, nil, 200)
	goal, runningContext := taskKnowledgeGoal(t, definition)
	if stringify(taskKnowledgeSnapshot(t, runningContext)) != original || strings.Contains(goal, newDraft) {
		t.Fatal("running definition used the unapproved draft", runningContext)
	}
	taskKnowledgeCommand(f, edited, "retire")
	retiredRead := f.call("GET", workflowPath, nil, 200)
	retiredGoal, _ := taskKnowledgeGoal(t, retiredRead)
	if retiredGoal != goal {
		t.Fatal("retirement rewrote a previously accepted run definition")
	}
}

func TestTaskKnowledgeDispatchChangedBasisCannotStartOrWriteReceipt(t *testing.T) {
	for _, change := range []string{"source_changed", "retired", "republished"} {
		t.Run(change, func(t *testing.T) {
			f, _, request, task := taskDispatchFixture(t)
			doc, published := taskKnowledgePublished(t, f)
			request["knowledge"] = []any{taskKnowledgeReference(published)}
			plan := f.call("POST", "/task-dispatch-plans", request, 201)
			original := stringify(plan["context"])
			switch change {
			case "source_changed":
				f.call("PATCH", "/documents/"+doc["id"].(string), f.body(map[string]any{"expected_version": doc["version"], "description": "The reviewed source changed.", "data": map[string]any{"content": "A different engineering policy."}}), 200)
			case "retired":
				taskKnowledgeCommand(f, published, "retire")
			case "republished":
				updated := taskKnowledgeEdit(f, published, doc, "A newly reviewed replacement, explicitly published.")
				taskKnowledgeCommand(f, updated, "publish")
			}
			before := taskKnowledgeEffectCounts(t, f)
			f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
			after := taskKnowledgeEffectCounts(t, f)
			if before != after || after.runs != 0 {
				t.Fatalf("rejected knowledge basis wrote effects: before=%+v after=%+v", before, after)
			}
			stale := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
			if stale["valid"] != false || stringify(stale["context"]) != original {
				t.Fatal("stale preview did not retain its immutable original context", stale)
			}
			current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
			if current["version"] != task["version"] || current["run_id"] != nil {
				t.Fatal("rejected knowledge basis bound the task", current)
			}
		})
	}
}

func TestTaskKnowledgeDispatchRejectsWrongHashScopeAndGoalBudget(t *testing.T) {
	f, _, base, _ := taskDispatchFixture(t)
	_, published := taskKnowledgePublished(t, f)
	preview := func(refs []any, want int) {
		t.Helper()
		request := map[string]any{}
		for key, value := range base {
			request[key] = value
		}
		request["idempotency_key"] = newID()
		request["knowledge"] = refs
		before := taskKnowledgeEffectCounts(t, f)
		f.call("POST", "/task-dispatch-plans", request, want)
		if after := taskKnowledgeEffectCounts(t, f); after != before {
			t.Fatalf("invalid context wrote effects: before=%+v after=%+v", before, after)
		}
	}
	bad := taskKnowledgeReference(published)
	bad["sha256"] = strings.Repeat("0", 64)
	preview([]any{bad}, 409)

	other := f.call("POST", "/projects", map[string]any{"name": "Separate knowledge project", "idempotency_key": newID()}, 201)
	foreign := f.call("POST", "/project-knowledge", map[string]any{"project_id": other["id"], "kind": "spec", "title": "Other project's reviewed spec", "summary": "", "content": "Must not become context for the first project.", "sources": []any{}, "expected_version": 0, "idempotency_key": newID()}, 201)
	foreign = taskKnowledgeCommand(f, foreign, "publish")
	preview([]any{taskKnowledgeReference(foreign)}, 404)

	// A maximal extractive item fits the complete goal when provenance is
	// metadata, while the full original quote remains review/history evidence.
	longBody := strings.Repeat("y", 16000)
	longDoc := f.call("POST", "/documents", f.body(map[string]any{"title": "Bounded extractive source", "data": map[string]any{"content": longBody}}), 201)
	extracted := f.call("POST", "/project-knowledge/extractions", f.body(map[string]any{"kind": "spec", "sources": []any{map[string]any{"kind": "documents", "id": longDoc["id"], "version": longDoc["version"], "quote": ""}}}), 201)
	maximal := taskKnowledgeCommand(f, extracted, "publish")
	maxRequest := map[string]any{}
	for key, value := range base {
		maxRequest[key] = value
	}
	maxRequest["idempotency_key"] = newID()
	maxRequest["knowledge"] = []any{taskKnowledgeReference(maximal)}
	maxPlan := f.call("POST", "/task-dispatch-plans", maxRequest, 201)
	maxGoal, _ := taskKnowledgeGoal(t, maxPlan["workflow"].(map[string]any))
	if len(maxGoal) > 32000 || strings.Count(maxGoal, longBody) != 1 {
		t.Fatal("maximal extractive knowledge was duplicated or exceeded the fixed goal budget")
	}

	large := []any{}
	for _, label := range []string{"First", "Second"} {
		draft := f.call("POST", "/project-knowledge", f.body(map[string]any{"kind": "spec", "title": label + " large reviewed spec", "summary": "", "content": strings.Repeat("x", 16000), "sources": []any{}, "expected_version": 0}), 201)
		large = append(large, taskKnowledgeReference(taskKnowledgeCommand(f, draft, "publish")))
	}
	// Each source fits the individual limit. Their complete task/requirement/
	// workflow envelope exceeds the fixed goal budget and must not be truncated.
	preview(large, 400)
}

func TestTaskKnowledgeAgentToolReturnsOnlyExplicitPublishedSnapshots(t *testing.T) {
	f, _, _, _ := taskDispatchFixture(t)
	doc, published := taskKnowledgePublished(t, f)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "agent", "label": "browser"}), 201)["token"].(string)
	refs := []any{taskKnowledgeReference(published)}
	read := func(want int) map[string]any {
		t.Helper()
		return f.bearer("POST", "/agent/tools/invoke", token, f.body(map[string]any{"tool": "project.knowledge", "arguments": map[string]any{"references": refs}}), want)
	}
	first := read(200)
	if len(first["items"].([]any)) != 1 || first["items"].([]any)[0].(map[string]any)["content"] != taskKnowledgeQuote {
		t.Fatal("agent did not receive exact selected publication", first)
	}
	agentSource := first["items"].([]any)[0].(map[string]any)["sources"].([]any)[0].(map[string]any)
	if agentSource["id"] != doc["id"] || agentSource["version"] != doc["version"] {
		t.Fatal("agent context lost provenance", agentSource)
	}
	if _, repeated := agentSource["quote"]; repeated {
		t.Fatal("agent context repeats the full review quote", agentSource)
	}
	const privateDraft = "PRIVATE_UNAPPROVED_DRAFT_NOT_FOR_AGENT"
	taskKnowledgeEdit(f, published, doc, privateDraft)
	reread := read(200)
	if stringify(reread) != stringify(first) || strings.Contains(stringify(reread), privateDraft) {
		t.Fatal("agent read leaked the newer draft", reread)
	}
	draft := f.call("POST", "/project-knowledge", f.body(map[string]any{"kind": "skill", "title": "Unreviewed only", "summary": "", "content": privateDraft, "sources": []any{}, "expected_version": 0}), 201)
	refs = []any{map[string]any{"id": draft["id"], "revision": draft["latest_revision"], "sha256": draft["latest"].(map[string]any)["sha256"]}}
	read(409)
	f.bearer("GET", "/project-knowledge?project_id="+f.project, token, nil, 403)
	f.bearer("POST", "/project-knowledge/"+published["id"].(string)+"/commands", token, f.body(map[string]any{"action": "publish", "expected_version": published["version"]}), 403)
	f.bearer("POST", "/agent/tools/invoke", token, map[string]any{"project_id": newID(), "tool": "project.knowledge", "arguments": map[string]any{"references": []any{taskKnowledgeReference(published)}}}, 403)
}
