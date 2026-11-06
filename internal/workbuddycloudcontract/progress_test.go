package workbuddycloudcontract

import (
	"strings"
	"testing"
)

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

func TestDecodeCloudProgressDeliveryRequiresCurrentPlatformHandoff(t *testing.T) {
	const saved = `{"project_id":"project","run_id":"run","node_id":"node","run_version":1,"run_state":"running","node_state":"waiting","node_reason":"cloud.delivery_required","attempt":1,"submission_state":"task_known","provider_status":"running","observed_at":"2026-11-06T00:00:00Z","observation_failed":true}`
	const link = `{"run_id":"run","node_id":"node","task_id":"task","task_version":1,"run_relation":"current"}`
	for _, tc := range []struct {
		name, original, replacement string
	}{
		{name: "saved observation can be stale"},
		{name: "paused run", original: `"run_state":"running"`, replacement: `"run_state":"paused"`},
		{name: "finished run", original: `"run_state":"running"`, replacement: `"run_state":"succeeded"`},
		{name: "different node state", original: `"node_state":"waiting"`, replacement: `"node_state":"running"`},
		{name: "different wait reason", original: `"node_reason":"cloud.delivery_required"`, replacement: `"node_reason":"cloud.waiting_input"`},
		{name: "no attempt", original: `"attempt":1`, replacement: `"attempt":0`},
		{name: "unconfirmed submission", original: `"submission_state":"task_known","provider_status":"running","observed_at":"2026-11-06T00:00:00Z","observation_failed":true`, replacement: `"submission_state":"unknown","observation_failed":false`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := saved
			if tc.original != "" {
				item = strings.Replace(saved, tc.original, tc.replacement, 1)
			}
			page, err := DecodeCloudNodeProgressPage([]byte(`{"project_id":"project","filter":"delivery","items":[` + item + `],"task_links":[` + link + `]}`))
			if tc.original == "" {
				if err != nil || page.Filter != CloudProgressDeliveryFilter || len(page.Items) != 1 {
					t.Fatalf("valid delivery fact did not survive decode: %+v, %v", page, err)
				}
			} else if err == nil {
				t.Fatal("accepted row outside the delivery scope")
			}
		})
	}
	for _, links := range []string{"", strings.Replace(link, `"current"`, `"changed"`, 1), strings.Replace(link, `"current"`, `"unbound"`, 1)} {
		if _, err := DecodeCloudNodeProgressPage([]byte(`{"project_id":"project","filter":"delivery","items":[` + saved + `],"task_links":[` + links + `]}`)); err == nil {
			t.Fatal("accepted delivery fact without a current formal task link")
		}
	}
}

func TestDecodeCloudProgressInputRequiresSavedWaitingObservationAndCurrentLink(t *testing.T) {
	const saved = `{"project_id":"project","run_id":"run","node_id":"node","run_version":1,"run_state":"running","node_state":"waiting","node_reason":"cloud.waiting_input","attempt":1,"submission_state":"task_known","provider_status":"waiting_input","observed_at":"2026-11-06T00:00:00Z","observation_failed":false}`
	const link = `{"run_id":"run","node_id":"node","task_id":"task","task_version":1,"run_relation":"current"}`
	for _, tc := range []struct {
		name, item, link string
		valid            bool
	}{
		{name: "current wait with matching saved observation", item: saved, link: link, valid: true},
		{name: "wait marker without matching observation", item: strings.Replace(saved, `"provider_status":"waiting_input"`, `"provider_status":"running"`, 1), link: link},
		{name: "observation without matching wait marker", item: strings.Replace(saved, `"node_reason":"cloud.waiting_input"`, `"node_reason":"cloud.observation_unconfirmed"`, 1), link: link},
		{name: "failed refresh", item: strings.Replace(saved, `"observation_failed":false`, `"observation_failed":true`, 1), link: link},
		{name: "paused original run", item: strings.Replace(saved, `"run_state":"running"`, `"run_state":"paused"`, 1), link: link},
		{name: "no formal task link", item: saved},
		{name: "task rebound to another run", item: saved, link: strings.Replace(link, `"current"`, `"changed"`, 1)},
		{name: "task unbound from this run", item: saved, link: strings.Replace(link, `"current"`, `"unbound"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := DecodeCloudNodeProgressPage([]byte(`{"project_id":"project","filter":"input","items":[` + tc.item + `],"task_links":[` + tc.link + `]}`))
			if (err == nil) != tc.valid || tc.valid && (page.Filter != CloudProgressInputFilter || len(page.Items) != 1 || len(page.TaskLinks) != 1) {
				t.Fatalf("unexpected input page validation: %+v, %v", page, err)
			}
		})
	}
}
