package platform

import (
	"net/url"
	"testing"
)

func TestWorkspaceGitHTTPExactReadScope(t *testing.T) {
	f, _ := workspaceFixture(t)
	path := "/workspace/git?project_id=" + url.QueryEscape(f.project)
	out := f.call("GET", path, nil, 200)
	if out["project_id"] != f.project || out["available"] != false || out["files"] == nil {
		t.Fatal(out)
	}
	for _, suffix := range []string{"&project_id=" + f.project, "&unknown=1", "&source=index", "&project_id="} {
		f.call("GET", path+suffix, nil, 400)
	}
	for _, suffix := range []string{"", "&path=main.go&source=other", "&path=main.go&path=other.go", "&path="} {
		f.call("GET", "/workspace/git/diff?project_id="+f.project+suffix, nil, 400)
	}
	f.call("GET", "/workspace/git?project_id=unowned", nil, 404)
}
