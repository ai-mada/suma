package releasecontract

import (
	"strings"
	"testing"
)

func TestApplicationReleaseSetPlanReferenceBindsExactMember(t *testing.T) {
	ref := &ApplicationReleaseSetRef{ReleaseSetID: "set124", ReleaseSetSHA256: strings.Repeat("a", 64), ApplicationID: "app124", ApplicationVersionID: "version124", ApplicationVersionSHA256: strings.Repeat("b", 64)}
	if ValidateApplicationReleaseSetRef(ref) != nil {
		t.Fatal("valid frozen reference rejected")
	}
	plan := ReleasePlanContent{ApplicationReleaseSet: ref}
	before, _ := CanonicalDigest(plan)
	ref.ApplicationVersionSHA256 = strings.Repeat("c", 64)
	after, _ := CanonicalDigest(plan)
	if before == after {
		t.Fatal("application version change did not invalidate original plan digest")
	}
	ref.ReleaseSetSHA256 = ""
	if ValidateApplicationReleaseSetRef(ref) == nil {
		t.Fatal("partial member reference allowed")
	}
	ref.ReleaseSetSHA256 = strings.Repeat("a", 64)
	ref.ApplicationID = "../other"
	if ValidateApplicationReleaseSetRef(ref) == nil {
		t.Fatal("unbounded identity allowed")
	}
}
