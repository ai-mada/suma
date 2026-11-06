package workbuddycloudcontract

import "testing"

func TestDecodeCloudProgressTaskScopeRequiresExactEchoAndLinks(t *testing.T) {
	const item = `{"project_id":"project","run_id":"run","node_id":"node","run_version":1,"run_state":"running","node_state":"running","attempt":1,"submission_state":"prepared","observation_failed":false}`
	const link = `{"run_id":"run","node_id":"node","task_id":"task","task_version":1,"run_relation":"current"}`
	for _, tc := range []struct {
		name     string
		raw      string
		wantTask string
		invalid  bool
	}{
		{name: "taskless omitted", raw: `{"project_id":"project","items":[]}`},
		{name: "taskless null", raw: `{"project_id":"project","items":[],"task_id":null}`, invalid: true},
		{name: "taskless empty", raw: `{"project_id":"project","items":[],"task_id":""}`, invalid: true},
		{name: "scoped empty continuation", raw: `{"project_id":"project","items":[],"task_id":"task","next_cursor":"YQ"}`, wantTask: "task"},
		{name: "scoped matching link", raw: `{"project_id":"project","items":[` + item + `],"task_id":"task","task_links":[` + link + `]}`, wantTask: "task"},
		{name: "scoped missing link", raw: `{"project_id":"project","items":[` + item + `],"task_id":"task"}`, invalid: true},
		{name: "scoped different task link", raw: `{"project_id":"project","items":[` + item + `],"task_id":"other","task_links":[` + link + `]}`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := DecodeCloudNodeProgressPage([]byte(tc.raw))
			if (err != nil) != tc.invalid || !tc.invalid && page.TaskID != tc.wantTask {
				t.Fatalf("unexpected scoped decode: task %q, error %v", page.TaskID, err)
			}
		})
	}
}
