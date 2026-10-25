package platform

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestWorkflowCatalogUsesManagedRegistryAndPreservesExactRunDefinition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w := dag.Workflow{ID: "managed-user-check", Name: "普通用户流程", Version: 1, Nodes: []dag.Node{{ID: "work", Name: "工作", Type: "command", Command: []string{"allowed-command"}, TimeoutSeconds: 30}}}
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, w); err != nil {
		t.Fatal(err)
	}
	authority := f.s.engineAuthority(ctx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectApprove)
	defer authority.close()
	run, err := f.e.CreateManagedRunBound(authority.ctx, f.user.TenantID, f.project, newID(), newID(), w, authority.before, func(*sql.Tx, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	items, err := f.e.Workflows(ctx, f.user.TenantID, f.project)
	if err != nil || len(items) != 1 || items[0].ID != w.ID {
		t.Fatalf("catalog mixed protected and user templates: %v %v", items, err)
	}
	definition, err := f.e.GetWorkflow(ctx, f.user.TenantID, f.project, run.WorkflowID, run.WorkflowVersion)
	if err != nil || definition.ID != run.WorkflowID || len(definition.Nodes) != 1 {
		t.Fatalf("exact managed execution diagram unavailable: %v %v", definition, err)
	}
}
