package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/store"
)

const taskCollectionInvalid = "Invalid project task collection query or cursor"

type taskCollectionGroupCursor struct {
	Version int    `json:"version"`
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	User    string `json:"user"`
	Filter  string `json:"filter"`
	GroupBy string `json:"group_by"`
	Limit   int    `json:"limit"`
	Key     string `json:"key"`
}

func normalizeTaskCollectionGroupOptions(options TaskCollectionGroupOptions) (TaskCollectionGroupOptions, error) {
	if options.GroupBy == "" {
		options.GroupBy = "status"
	}
	if options.Limit == 0 {
		options.Limit = 20
	}
	if options.GroupBy != "status" && options.GroupBy != "owner" && options.GroupBy != "priority" && options.GroupBy != "iteration" || options.Limit < 1 || options.Limit > 100 || len(options.Cursor) > 4096 {
		return options, invalid(taskCollectionInvalid)
	}
	return options, nil
}

func validTaskCollectionGroupKey(key string) bool {
	return utf8.ValidString(key) && utf8.RuneCountInString(key) <= 200 && taskWorkspaceText(key)
}

func taskCollectionGroupKeyError() error {
	return &APIError{Status: 409, Code: "task_group_key_invalid", Message: "A task group has an invalid key"}
}

func (s *Service) InitializeTaskCollections(ctx context.Context) error {
	for _, index := range []struct {
		name    string
		columns []string
	}{
		{"suma_tasks_state_updates", []string{"tenant_id", "project_id", "status", "updated_at", "id"}},
		{"suma_tasks_requirement_updates", []string{"tenant_id", "project_id", "requirement_id", "updated_at", "id"}},
		{"suma_tasks_priority_updates", []string{"tenant_id", "project_id", "priority", "updated_at", "id"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, "suma_tasks", index.columns...); err != nil {
			return err
		}
	}
	return nil
}

// TaskCollectionSummary uses the canonical task query compiler. Grouping only
// changes the aggregate key; status, ownership and delivery facts remain data.
func (s *Service) TaskCollectionSummary(ctx context.Context, actor Actor, project string, input TaskWorkspaceQuery, options TaskCollectionGroupOptions) (out TaskCollectionSummary, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || input.Sort != "" || input.Cursor != "" || input.Limit != 0 {
		return out, invalid(taskCollectionInvalid)
	}
	q, err := normalizeTaskWorkspaceQuery(input)
	if err != nil {
		return out, err
	}
	options, err = normalizeTaskCollectionGroupOptions(options)
	if err != nil {
		return out, err
	}
	column := map[string]string{"status": "status", "owner": "owner_id", "priority": "priority", "iteration": "iteration_key"}[options.GroupBy]
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		if err := assertProjectRead(auth, project); err != nil {
			return err
		}
		if !myWorkHuman(auth.actor) {
			return forbidden()
		}
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out = TaskCollectionSummary{ProjectID: project, AsOf: auth.checkedAt.UTC().Format(time.RFC3339Nano), GroupBy: options.GroupBy, Groups: []TaskCollectionGroup{}, GroupsLimit: options.Limit}
		scope := taskCollectionGroupCursor{Version: 1, Tenant: auth.target.tenantID, Project: project, User: auth.currentPrincipal.ID, Filter: digest(stringify(q)), GroupBy: options.GroupBy, Limit: options.Limit}
		var after taskCollectionGroupCursor
		if options.Cursor != "" {
			raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(options.Cursor)
			if decodeErr != nil || json.Unmarshal(raw, &after) != nil || !validTaskCollectionGroupKey(after.Key) || base64.RawURLEncoding.EncodeToString([]byte(stringify(after))) != options.Cursor {
				return invalid(taskCollectionInvalid)
			}
			identity := after
			identity.Key = ""
			if identity != scope {
				return invalid(taskCollectionInvalid)
			}
		}
		where, args := taskWorkspaceWhere(auth.target.tenantID, project, q)
		if options.GroupBy == "iteration" {
			// Materialize the scalar relation as a derived column before grouping;
			// aggregate clauses must not depend on ungrouped outer task columns.
			where = ` FROM (SELECT ` + taskWorkspaceIterationKey + ` AS iteration_key` + where + `) task_iteration_groups`
		}
		if countErr := auth.tx.QueryRowContext(bounded, s.q(`SELECT COUNT(*),COUNT(DISTINCT `+column+`)`+where), args...).Scan(&out.Total, &out.GroupsTotal); countErr != nil {
			return countErr
		}
		query := `SELECT ` + column + `,COUNT(*)` + where + ` GROUP BY ` + column
		if options.Cursor != "" {
			query += ` HAVING ` + column + `>?`
			args = append(args, after.Key)
		}
		query += ` ORDER BY ` + column + ` ASC LIMIT ?`
		args = append(args, options.Limit+1)
		rows, readErr := auth.tx.QueryContext(bounded, s.q(query), args...)
		if readErr != nil {
			return readErr
		}
		defer rows.Close()
		for rows.Next() {
			var group TaskCollectionGroup
			if readErr = rows.Scan(&group.Key, &group.Total); readErr != nil {
				return readErr
			}
			if !validTaskCollectionGroupKey(group.Key) {
				return taskCollectionGroupKeyError()
			}
			out.Groups = append(out.Groups, group)
		}
		if readErr = rows.Err(); readErr != nil {
			return readErr
		}
		if len(out.Groups) > options.Limit {
			out.Groups = out.Groups[:options.Limit]
			scope.Key = out.Groups[options.Limit-1].Key
			out.NextGroupCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
			if len(out.NextGroupCursor) > 4096 {
				return taskCollectionGroupKeyError()
			}
		}
		return nil
	})
	if err != nil {
		return TaskCollectionSummary{}, err
	}
	return out, nil
}
