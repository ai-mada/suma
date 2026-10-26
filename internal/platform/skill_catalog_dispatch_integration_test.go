package platform

import (
	"strconv"
	"strings"
	"testing"
)

// Exercise the actual catalog and knowledge HTTP commands before dispatch. An
// adopted skill is an independent target-project draft, never new tool authority.
func TestSkillCatalogDispatchRequiresTargetPublicationAndFreezesAdoption(t *testing.T) {
	f, token, request, _ := taskDispatchFixture(t)
	sourceProject := f.call("POST", "/projects", map[string]any{"name": "Reviewed skill source", "idempotency_key": newID()}, 201)["id"].(string)
	sourceBody := func(body map[string]any) map[string]any {
		body["project_id"] = sourceProject
		body["idempotency_key"] = newID()
		return body
	}
	doc := f.call("POST", "/documents", sourceBody(map[string]any{"title": "Private source title", "description": "Private source metadata", "data": map[string]any{"content": taskKnowledgeQuote}}), 201)
	source := f.call("POST", "/project-knowledge/extractions", sourceBody(map[string]any{"kind": "skill", "title": "Reviewed tenant boundary skill", "sources": []any{taskKnowledgeSource(doc)}}), 201)
	source = taskKnowledgeCommand(f, source, "publish")
	sourceRevision := source["published"].(map[string]any)
	catalog := f.call("POST", "/skill-catalog", sourceBody(map[string]any{
		"knowledge_id": source["id"], "expected_knowledge_version": source["version"],
		"published_revision": source["published_revision"], "published_sha256": source["published_sha256"],
		"expected_version": 0, "domain": "engineering", "scenarios": []string{"review"}, "tags": []string{"tenant"},
		"reason": "Share only reviewed skill text", "acknowledge_sharing": true,
	}), 201)
	catalogPath := "/skill-catalog/" + catalog["id"].(string)
	shared := f.call("GET", catalogPath+"?project_id="+f.project, nil, 200)
	if shared["content"] != sourceRevision["content"] || strings.Contains(stringify(shared), "Private source") || shared["source_current"] != true {
		t.Fatal("organization view did not preserve only approved shared text", shared)
	}
	adoption := f.body(map[string]any{"expected_version": shared["version"], "origin_sha256": source["published_sha256"], "reason": "Review independently in target project", "acknowledge_draft": true})
	draft := f.call("POST", catalogPath+"/adoptions", adoption, 201)
	if replay := f.call("POST", catalogPath+"/adoptions", adoption, 201); replay["id"] != draft["id"] {
		t.Fatal("same adoption operation created another draft")
	}
	if draft["project_id"] != f.project || draft["id"] == source["id"] || draft["kind"] != "skill" || draft["state"] != "draft" || draft["published_revision"] != float64(0) || draft["published"] != nil {
		t.Fatal("adoption bypassed the target draft/publication boundary", draft)
	}
	latest := draft["latest"].(map[string]any)
	if latest["content"] != sourceRevision["content"] || len(latest["sources"].([]any)) != 0 {
		t.Fatal("adoption changed shared text or copied private source evidence", latest)
	}
	lineage := draft["catalog_adoption"].(map[string]any)
	origin := lineage["origin"].(map[string]any)
	if lineage["catalog_id"] != shared["id"] || lineage["catalog_version"] != shared["version"] || lineage["project_id"] != f.project || lineage["knowledge_id"] != draft["id"] || lineage["revision"] != latest["revision"] || lineage["sha256"] != latest["sha256"] || origin["project_id"] != sourceProject || origin["knowledge_id"] != source["id"] || origin["revision"] != source["published_revision"] || origin["sha256"] != source["published_sha256"] {
		t.Fatal("adoption lost its exact catalog/source/target provenance", lineage)
	}
	request["knowledge"] = []any{map[string]any{"id": draft["id"], "revision": latest["revision"], "sha256": latest["sha256"]}}
	before := taskKnowledgeEffectCounts(t, f)
	f.call("POST", "/task-dispatch-plans", request, 409)
	if after := taskKnowledgeEffectCounts(t, f); after != before {
		t.Fatalf("unpublished adoption created dispatch effects: before=%+v after=%+v", before, after)
	}
	published := taskKnowledgeCommand(f, draft, "publish")
	if published["published_eligible"] != true || published["catalog_adoption"] == nil || stringify(published["catalog_adoption"]) != stringify(lineage) {
		t.Fatal("normal target publication lost the original adoption", published)
	}
	request["knowledge"] = []any{taskKnowledgeReference(published)}
	request["idempotency_key"] = newID()
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	approved := taskKnowledgeSnapshot(t, plan["context"].(map[string]any))
	if approved["content"] != sourceRevision["content"] || approved["revision"] != published["published_revision"] || approved["sha256"] != published["published_sha256"] || stringify(approved["catalog_adoption"]) != stringify(lineage) || len(approved["sources"].([]any)) != 0 {
		t.Fatal("dispatch preview lost the approved target body/hash/lineage", approved)
	}
	goal, goalContext := taskKnowledgeGoal(t, plan["workflow"].(map[string]any))
	if stringify(taskKnowledgeSnapshot(t, goalContext)) != stringify(approved) {
		t.Fatal("actual agent goal differs from the reviewed target snapshot")
	}

	// Hiding the source blocks new adoption. It cannot revoke the independent
	// target publication or silently rewrite a plan already based on that copy.
	f.call("PATCH", "/documents/"+doc["id"].(string), sourceBody(map[string]any{"expected_version": doc["version"], "status": "archived"}), 200)
	staleSource := f.call("GET", catalogPath+"?project_id="+f.project, nil, 200)
	if staleSource["source_current"] != false || staleSource["can_adopt"] != false {
		t.Fatal("hidden original source still allows new adoption", staleSource)
	}
	f.call("POST", catalogPath+"/adoptions", f.body(map[string]any{"expected_version": shared["version"], "origin_sha256": source["published_sha256"], "reason": "Must not adopt hidden source", "acknowledge_draft": true}), 409)
	retired := f.call("POST", catalogPath+"/commands", sourceBody(map[string]any{"action": "retire", "expected_version": shared["version"], "reason": "Stop future catalog adoption"}), 200)
	if retired["state"] != "retired" {
		t.Fatal("catalog retirement did not complete")
	}
	current := f.call("GET", "/project-knowledge/"+published["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["published_eligible"] != true || current["published_sha256"] != published["published_sha256"] || stringify(current["catalog_adoption"]) != stringify(lineage) {
		t.Fatal("source/catalog changes altered the independent target publication", current)
	}
	reread := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if reread["valid"] != true || stringify(taskKnowledgeSnapshot(t, reread["context"].(map[string]any))) != stringify(approved) {
		t.Fatal("source/catalog changes rewrote or revoked the fixed target plan", reread)
	}
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	workflowPath := "/workflows/" + run["workflow_id"].(string) + "?project_id=" + f.project + "&version=" + strconv.Itoa(int(run["workflow_version"].(float64)))
	definition := f.call("GET", workflowPath, nil, 200)
	runningGoal, runningContext := taskKnowledgeGoal(t, definition)
	if runningGoal != goal || stringify(taskKnowledgeSnapshot(t, runningContext)) != stringify(approved) {
		t.Fatal("started workflow changed the reviewed adoption snapshot")
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "tool-worker", "capabilities": []string{"command:native-agent"}}), 200)
	assignment, ok := claim["assignment"].(map[string]any)
	if !ok || assignment["config"].(map[string]any)["goal"] != goal {
		t.Fatal("real runner assignment did not receive the exact approved goal", claim)
	}
	if assignment["command"].([]any)[0] != "native-agent" {
		t.Fatal("skill adoption changed the fixed execution capability", assignment)
	}
}
