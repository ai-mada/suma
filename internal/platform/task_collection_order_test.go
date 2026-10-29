package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"testing"
)

func TestTaskCollectionExactUTCOrderAcrossSingleItemPages(t *testing.T) {
	h := newTaskCollectionFixture(t, 0)
	f := h.f
	entries := []struct{ id, at, status string }{
		{"whole", "2026-10-29T00:00:00Z", "waiting_vendor"},
		{"b-equal", "2026-10-29T00:00:00.1Z", "waiting_vendor"},
		{"a-equal", "2026-10-29T00:00:00.100Z", "todo"},
		{"later", "2026-10-29T00:00:00.1001Z", "todo"},
		{"small", "2026-10-29T00:00:00.01Z", "waiting_vendor"},
		{"nano-low", "2026-10-29T00:00:00.100100001Z", "waiting_vendor"},
		{"nano-high", "2026-10-29T00:00:00.100100002Z", "todo"},
	}
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, entry := range entries {
		r := recordFrom(f.user, f.project, map[string]any{"title": entry.id, "status": entry.status, "owner_id": h.owner.ID})
		r.ID, r.UpdatedAt = entry.id, entry.at
		r.CreatedAt = entry.at
		if err = f.s.insertRecord(context.Background(), tx, r, "tasks"); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, sortMode := range []string{"updated_desc", "created_desc"} {
		for _, test := range []struct {
			status   string
			expected []string
		}{
			{"", []string{"nano-high", "nano-low", "later", "a-equal", "b-equal", "small", "whole"}},
			{"todo", []string{"nano-high", "later", "a-equal"}},
		} {
			query := "/task-workspace?project_id=" + f.project + "&owner_id=" + h.owner.ID + "&limit=1&sort=" + sortMode
			if test.status != "" {
				query += "&status=" + url.QueryEscape(test.status)
			}
			cursor := ""
			ids := []string{}
			for {
				path := query
				if cursor != "" {
					path += "&cursor=" + url.QueryEscape(cursor)
				}
				page := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", path, nil, 200))
				if page.Total != len(test.expected) || len(page.Items) != 1 {
					t.Fatalf("incorrect exact-order page: %+v", page)
				}
				item := page.Items[0]
				ids = append(ids, item.ID)
				for _, entry := range entries {
					if entry.id == item.ID && (item.UpdatedAt != entry.at || item.CreatedAt != entry.at) {
						t.Fatalf("source timestamp rewritten: %+v", item)
					}
				}
				cursor = page.NextCursor
				if cursor == "" {
					break
				}
				raw, decodeErr := base64.RawURLEncoding.DecodeString(cursor)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				var token map[string]any
				if err = json.Unmarshal(raw, &token); err != nil {
					t.Fatal(err)
				}
				if _, oldField := token["updated"]; oldField {
					t.Fatal("cursor retained raw timestamp ordering")
				}
				encodedKey, ok := token["value"].(string)
				keyBytes, keyErr := base64.RawURLEncoding.DecodeString(encodedKey)
				key := string(keyBytes)
				if !ok || keyErr != nil || len(key) != 30 {
					t.Fatalf("cursor lacks fixed UTC nanosecond key: %#v", token)
				}
			}
			if !reflect.DeepEqual(ids, test.expected) {
				t.Fatalf("status %q chronological order: got %v want %v", test.status, ids, test.expected)
			}
		}
	}
	first := taskCollectionDecode[TaskWorkspacePage](t, f.call("GET", "/task-workspace?project_id="+f.project+"&limit=1", nil, 200))
	raw, err := base64.RawURLEncoding.DecodeString(first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	var token map[string]any
	if err = json.Unmarshal(raw, &token); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"2026-10-29T00:00:00.1Z", "2026-10-29T00:00:00.100000000+00:00", "2026-10-29T00:00:00.1000000000Z"} {
		token["value"] = base64.RawURLEncoding.EncodeToString([]byte(key))
		wire, marshalErr := json.Marshal(token)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		// Preserve the struct's exact JSON field ordering so the timestamp
		// validation, rather than merely object ordering, rejects the cursor.
		var candidate taskWorkspaceCursor
		if err = json.Unmarshal(wire, &candidate); err != nil {
			t.Fatal(err)
		}
		cursor := base64.RawURLEncoding.EncodeToString([]byte(stringify(candidate)))
		_, err = f.s.ListTaskWorkspace(context.Background(), f.actor, f.project, TaskWorkspaceQuery{Limit: 1, Cursor: cursor})
		requireAuthorityStatus(t, err, 400)
	}
}

func TestRecordOrderKeyAcceptsExactUTCPrecisionWithoutRewritingSource(t *testing.T) {
	for _, test := range []struct{ source, key string }{
		{"2026-10-29T00:00:00Z", "2026-10-29T00:00:00.000000000Z"},
		{"2026-10-29T00:00:00.1Z", "2026-10-29T00:00:00.100000000Z"},
		{"2026-10-29T00:00:00.100Z", "2026-10-29T00:00:00.100000000Z"},
		{"2026-10-29T00:00:00.100100001Z", "2026-10-29T00:00:00.100100001Z"},
	} {
		key, err := recordOrderKey(test.source)
		if err != nil || key != test.key {
			t.Fatalf("exact key %q: %q %v", test.source, key, err)
		}
	}
	for _, source := range []string{"", "2026-10-29T00:00:00+00:00", "2026-10-29T00:00:00.1000000001Z", "2026-02-30T00:00:00Z", "2026-10-29T0:00:00Z", "2026-10-29T00:00:00,1Z"} {
		_, err := recordOrderKey(source)
		var apiError *APIError
		if !errors.As(err, &apiError) || apiError.Status != 409 || apiError.Code != "task_timestamp_invalid" {
			t.Fatalf("unsupported source silently normalized %q: %v", source, err)
		}
	}
}

func TestTaskCollectionInvalidSourceFailsWithoutOmittingCountOrLookahead(t *testing.T) {
	h := newTaskCollectionFixture(t, 2)
	f := h.f
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET updated_at='2026-10-29T00:00:01Z' WHERE id=?`, h.tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"2026-10-29T00:00:00.1+00:00", "2026-10-29T00:00:00.1001000010Z", "invalid-source"} {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET updated_at=?,created_at=? WHERE id=?`, source, source, h.tasks[1].ID); err != nil {
			t.Fatal(err)
		}
		summary, err := f.s.TaskCollectionSummary(context.Background(), f.actor, f.project, TaskWorkspaceQuery{}, TaskCollectionGroupOptions{GroupBy: "status"})
		if err != nil || summary.Total != 2 {
			t.Fatalf("invalid source was silently excluded: %+v %v", summary, err)
		}
		for _, sortMode := range []string{"updated_desc", "created_desc"} {
			out := f.call("GET", "/task-workspace?project_id="+f.project+"&limit=1&sort="+sortMode, nil, 409)
			failure := out["error"].(map[string]any)
			if failure["code"] != "task_timestamp_invalid" {
				t.Fatalf("invalid source treated as caller query: %#v", out)
			}
		}
		var stored string
		if err = f.s.DB.QueryRow(`SELECT updated_at FROM suma_tasks WHERE id=?`, h.tasks[1].ID).Scan(&stored); err != nil || stored != source {
			t.Fatalf("source timestamp changed: %q %v", stored, err)
		}
	}
}
