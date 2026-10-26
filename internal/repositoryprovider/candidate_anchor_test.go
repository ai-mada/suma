package repositoryprovider

import (
	"context"
	"strings"
	"testing"

	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func TestRepositoryGitInheritedCandidateUsesBundleAnchor(t *testing.T) {
	f := newGitFixture(t)
	prepare := f.request(t, "prepare", "inherited-prepare", "")
	prepare.Source.BundleBaseCommit = f.base
	prepare.Source.BaseCommit = f.head // selected M == HEAD: the new task made no delta.
	f.executions[prepare.OperationID] = prepare.RepositoryExecution
	code, observed := providerCall(t, f.s, "POST", "/operations", prepare, false)
	if code != 200 || observed.State != "prepared" || observed.Preview == nil || !strings.Contains(observed.Preview.Diff, "+candidate") {
		t.Fatalf("B..M inherited preview was lost: %d %+v", code, observed)
	}
	apply := f.request(t, "apply", "inherited-apply", observed.Preview.PreparedSHA256)
	apply.Source = prepare.Source
	f.executions[apply.OperationID] = apply.RepositoryExecution
	code, applied := providerCall(t, f.s, "POST", "/operations", apply, false)
	sha, _ := rc.RepositoryRequestDigest(apply)
	if code != 200 || applied.State != "applied" || rc.RepositoryValidateObservation(applied, apply.RepositoryExecution, sha) != nil {
		t.Fatalf("inherited exact candidate did not apply: %d %+v", code, applied)
	}
	target, err := f.s.Target(context.Background(), "main")
	if err != nil || target.HeadCommit != f.head || f.effects != 1 {
		t.Fatalf("target did not receive exact M: %+v %v", target, err)
	}
	// The pack anchor alone cannot claim a selected base which its head does not
	// descend from. A false selected M is rejected even when HEAD already exists.
	bad := prepare
	bad.Source.BaseCommit = strings.Repeat("f", len(f.base))
	if err = f.s.importBundle(context.Background(), f.s.targets["main"], &operation{Request: rc.RepositoryNonSecretRequest{RepositoryExecution: bad.RepositoryExecution, Assignment: bad.Assignment}}); err == nil {
		t.Fatal("unproved selected base accepted behind a valid bundle anchor")
	}
}
