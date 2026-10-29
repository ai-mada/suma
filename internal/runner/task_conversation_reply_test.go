package runner

import (
	"strings"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestTaskConversationReplyRequiresFrozenReadOnlyGateway(t *testing.T) {
	base := testModelBinding(t, "https://provider.example")
	binding, err := mc.NewBinding(base.Deployment, mc.Parameters{MaxSteps: 1, MaxOutputTokens: 8192}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Input: "agent-jsonl", ReadOnly: true, TaskConversationReply: true, ModelExecutionProtocol: mc.Protocol}
	profile := tc.Profile{CommandID: "recommend", Adapter: spec.Adapter, Protocol: spec.Protocol, ReadOnly: true, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "1.0.0", ModelExecutionProtocol: mc.Protocol, TaskConversationReply: &tc.TaskConversationReplyCapability{Protocol: tc.TaskConversationReplyProtocol}}
	a := Assignment{Type: "agent", Config: map[string]any{"purpose": "task_conversation_reply", "goal": strings.Repeat("x", 64000), "max_steps": 1, "max_output_tokens": 8192, "model_execution": modelBindingValue(t, binding)}}
	request, err := taskConversationReplyRequest(a, spec, profile)
	if err != nil || request["max_steps"] != 1 || request["max_tokens"] != 8192 {
		t.Fatalf("frozen one-call request: %v", err)
	}
	if tc.ValidateProfile(profile, true) != nil || !tc.SupportsTaskConversationReplyProfile(profile) {
		t.Fatal("dedicated profile was rejected")
	}
	for _, name := range []string{"model_execution", "max_steps", "goal"} {
		old := a.Config[name]
		delete(a.Config, name)
		if _, err := taskConversationReplyRequest(a, spec, profile); err == nil {
			t.Fatalf("missing %s accepted", name)
		}
		a.Config[name] = old
	}
	a.Config["max_output_tokens"] = 8191
	if _, err := taskConversationReplyRequest(a, spec, profile); err == nil {
		t.Fatal("binding scalar drift accepted")
	}
	a.Config["max_output_tokens"] = 8192
	a.Config["goal"] = strings.Repeat("x", 64001)
	if _, err := taskConversationReplyRequest(a, spec, profile); err == nil {
		t.Fatal("oversized frozen source accepted")
	}
	withWorkspace := tc.CloneProfile(profile)
	withWorkspace.WorkspaceToolsVersion = NativeWorkspaceVersion
	if tc.ValidateProfile(withWorkspace, true) == nil {
		t.Fatal("task reply acquired workspace tools")
	}
	withProvider := spec
	withProvider.EnvFrom = []string{"VENDOR_KEY"}
	if _, err := localTaskConversationReplyCapability(withProvider); err == nil {
		t.Fatal("task reply accepted provider environment")
	}
	withoutGateway := spec
	withoutGateway.ModelExecutionProtocol = ""
	if validateModelExecutionSpec(withoutGateway) == nil {
		t.Fatal("unbound task reply acquired execution")
	}
	// The new capability must keep exact value identity through independently
	// decoded inventory and may not be combined with a different model purpose.
	cloned := tc.CloneProfile(profile)
	if !tc.EqualProfile(cloned, profile) {
		t.Fatal("cloned reply capability changed")
	}
	cloned.TaskConversationReply.Protocol = "other"
	if tc.EqualProfile(cloned, profile) || profile.TaskConversationReply.Protocol != tc.TaskConversationReplyProtocol {
		t.Fatal("reply profile pointer escaped clone/value comparison")
	}
	mixed := tc.CloneProfile(profile)
	mixed.DispatchRecommendation = &tc.DispatchRecommendationCapability{Protocol: tc.DispatchRecommendationProtocol}
	if tc.ValidateProfile(mixed, true) == nil {
		t.Fatal("multiple read-only purposes accepted in one profile")
	}

}
