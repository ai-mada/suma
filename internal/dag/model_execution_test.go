package dag

import (
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/modelcontract"
)

func TestBoundModelCompileRejectsAutomaticRetry(t *testing.T) {
	d := modelcontract.Deployment{ID: "bound-model", Revision: 1, Protocol: modelcontract.ProviderProtocol, Model: "bound-model", EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("a", 64), ContextWindow: 8192}
	d.SHA256 = modelcontract.DeploymentDigest(d)
	b, err := modelcontract.NewBinding(d, modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := toolNode()
	n.Idempotent = true
	n.MaxRetries = 1
	n.Config["model_execution"] = b
	n.Config["max_steps"] = 2
	n.Config["max_output_tokens"] = 100
	if err = Compile(Workflow{Nodes: []Node{n}}); err == nil {
		t.Fatal("bound model accepted a workflow with automatic retries")
	}
}
