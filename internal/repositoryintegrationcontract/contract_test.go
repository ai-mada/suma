package repositoryintegrationcontract

import (
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"strings"
	"testing"
)

func contractRequest() RepositoryRequest {
	a := cc.CandidateAssignmentRef{RunID: "run", NodeID: "node", Attempt: 1, Fence: 1, WorkerID: "worker"}
	return RepositoryRequest{RepositoryExecution: RepositoryExecution{Protocol: RepositoryProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "provider", TargetID: "target", TargetAlias: "main", TargetRef: "refs/heads/main", TargetConfigSHA256: strings.Repeat("a", 64), PlanID: "plan", PlanVersion: 1, Phase: "prepare", OperationID: "operation-0001", ExpectedTargetHead: strings.Repeat("a", 40), Generation: 1, Source: cc.CandidateBinding{SchemaVersion: 1, State: "sealed", TransferID: "transfer", ProjectID: "project", Source: a, BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("c", 64), BundleSHA256: strings.Repeat("d", 64), SizeBytes: 1}}, Assignment: a, AuthorizationTicket: "secret"}
}
func TestRepositoryDigestAndReceiptBindings(t *testing.T) {
	q := contractRequest()
	h, e := RepositoryRequestDigest(q)
	if e != nil {
		t.Fatal(e)
	}
	q.AuthorizationTicket = "different"
	q.Assignment.LeaseUntil = "later"
	q.Assignment.DispatchState = "active"
	same, e := RepositoryRequestDigest(q)
	if e != nil || h != same {
		t.Fatal("ephemeral authorization changed request digest")
	}
	q.Assignment.WorkerID = "different"
	different, _ := RepositoryRequestDigest(q)
	if h == different {
		t.Fatal("worker omitted from request binding")
	}
	q = contractRequest()
	q.Phase = "apply"
	q.PreparedSHA256 = strings.Repeat("f", 64)
	h, _ = RepositoryRequestDigest(q)
	r := RepositoryReceipt{Protocol: RepositoryProtocol, OperationID: q.OperationID, RequestSHA256: h, TargetID: q.TargetID, TargetAlias: q.TargetAlias, TargetRef: q.TargetRef, TargetConfigSHA256: q.TargetConfigSHA256, BeforeCommit: q.ExpectedTargetHead, ResultCommit: q.Source.HeadCommit, SourceBundleSHA256: q.Source.BundleSHA256, PreparedSHA256: q.PreparedSHA256, Generation: q.Generation, EffectGrantID: "effect", State: "applied"}
	if e = RepositoryValidateReceipt(r, q.RepositoryExecution, h); e != nil {
		t.Fatal(e)
	}
	for _, format := range []string{"sha1", "sha256"} {
		oid, e := RepositoryReceiptOID(r, format)
		if e != nil || len(oid) != map[string]int{"sha1": 40, "sha256": 64}[format] {
			t.Fatal("receipt OID")
		}
	}
	r.BeforeCommit = q.Source.HeadCommit
	if RepositoryValidateReceipt(r, q.RepositoryExecution, h) == nil {
		t.Fatal("wrong before accepted")
	}
}
