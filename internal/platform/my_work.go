package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/store"
)

const myWorkInvalid = "Invalid personal work query or cursor"

type myWorkCursor struct {
	Tenant          string `json:"tenant"`
	Project         string `json:"project"`
	User            string `json:"user"`
	Resource        string `json:"resource"`
	ResourceVersion int    `json:"resource_version"`
	Q               string `json:"q"`
	Status          string `json:"status"`
	Limit           int    `json:"limit"`
	Updated         string `json:"updated"`
	ID              string `json:"id"`
}

func (s *Service) InitializeMyWork(ctx context.Context) error {
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tasks_owner_updates", "suma_tasks", "tenant_id", "project_id", "owner_id", "updated_at", "id")
}

func myWorkHuman(actor Actor) bool {
	return actor.kind == actorBrowser || actor.kind == actorProjectAPI
}

func normalizeMyWorkOptions(options MyWorkOptions) (MyWorkOptions, error) {
	if options.Limit == 0 {
		options.Limit = 20
	}
	if options.Status == "" {
		options.Status = "all"
	}
	switch options.Status {
	case "all", "draft", "todo", "in_progress", "in_review", "accepted", "blocked", "cancelled":
	default:
		return options, invalid(myWorkInvalid)
	}
	if options.Limit < 1 || options.Limit > 50 || len(options.Cursor) > 4096 || !utf8.ValidString(options.Q) || utf8.RuneCountInString(options.Q) > 200 || strings.ContainsRune(options.Q, 0) {
		return options, invalid(myWorkInvalid)
	}
	return options, nil
}

// MyWork uses the same current authority transaction for identity, complete
// counts and bounded rows. Moving tasks between live pages requires refresh;
// neither a cursor nor a successful run is a snapshot or human acceptance.
func (s *Service) MyWork(ctx context.Context, actor Actor, project string, options MyWorkOptions) (out MyWorkPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) {
		return out, invalid(myWorkInvalid)
	}
	options, err = normalizeMyWorkOptions(options)
	if err != nil {
		return out, err
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.myWorkTx(bounded, auth, project, options)
		return err
	})
	if err != nil {
		return MyWorkPage{}, err
	}
	return out, nil
}

func (s *Service) myWorkTx(ctx context.Context, auth *authorizedCommand, project string, options MyWorkOptions) (MyWorkPage, error) {
	out := MyWorkPage{ProjectID: project, Items: []Record{}, StatusCounts: []MyWorkStatusCount{}, Limit: options.Limit, ResourceStatus: "unbound"}
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	if !myWorkHuman(auth.actor) {
		return out, forbidden()
	}
	out.UserID = auth.currentPrincipal.ID
	out.AsOf = auth.checkedAt.UTC().Format(time.RFC3339Nano)
	tenant := auth.target.tenantID
	var resource string
	err := auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT resource_id FROM suma_human_resources WHERE tenant_id=? AND project_id=? AND user_id=?`)), tenant, project, out.UserID).Scan(&resource)
	if errors.Is(err, sql.ErrNoRows) {
		if options.Cursor != "" {
			return out, invalid(myWorkInvalid)
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.ResourceID, out.ResourceStatus = resource, "inactive"
	member, err := scanRecord(auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT `+recordColumns+` FROM suma_members WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, resource))
	if errors.Is(err, sql.ErrNoRows) {
		if options.Cursor != "" {
			return out, invalid(myWorkInvalid)
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.ResourceName = member.Title
	user, _ := member.Data["user_id"].(string)
	if member.Kind != "human" || member.Status != "active" || user != out.UserID {
		if options.Cursor != "" {
			return out, invalid(myWorkInvalid)
		}
		return out, nil
	}
	out.ResourceStatus = "active"
	scope := myWorkCursor{Tenant: tenant, Project: project, User: out.UserID, Resource: resource, ResourceVersion: member.Version, Q: options.Q, Status: options.Status, Limit: options.Limit}
	var after myWorkCursor
	if options.Cursor != "" {
		raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(options.Cursor)
		if decodeErr != nil || json.Unmarshal(raw, &after) != nil || !handoffID(after.ID, false) || !validReleaseTime(after.Updated) || base64.RawURLEncoding.EncodeToString([]byte(stringify(after))) != options.Cursor {
			return out, invalid(myWorkInvalid)
		}
		identity := after
		identity.Updated, identity.ID = "", ""
		if identity != scope {
			return out, invalid(myWorkInvalid)
		}
	}
	where := ` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND owner_id=? AND status<>'archived'`
	args := []any{tenant, project, resource}
	if options.Q != "" {
		// Escape wildcards and use the database's own LOWER on both operands.
		// This preserves literal Unicode text under SQLite's native case rules.
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(options.Q) + "%"
		where += ` AND (LOWER(title) LIKE LOWER(?) ESCAPE '!' OR LOWER(description) LIKE LOWER(?) ESCAPE '!')`
		args = append(args, pattern, pattern)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT status,COUNT(*)`+where+` GROUP BY status ORDER BY status ASC`), args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var count MyWorkStatusCount
		if err = rows.Scan(&count.Status, &count.Total); err != nil {
			rows.Close()
			return out, err
		}
		out.StatusCounts = append(out.StatusCounts, count)
		if options.Status == "all" || count.Status == options.Status {
			out.Total += count.Total
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return out, err
	}
	if err = rows.Close(); err != nil {
		return out, err
	}
	if options.Status != "all" {
		where += ` AND status=?`
		args = append(args, options.Status)
	}
	if options.Cursor != "" {
		where += ` AND (updated_at<? OR (updated_at=? AND id>?))`
		args = append(args, after.Updated, after.Updated, after.ID)
	}
	args = append(args, options.Limit+1)
	rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT `+recordColumns+where+` ORDER BY updated_at DESC,id ASC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanRecord(rows)
		if scanErr != nil {
			return out, scanErr
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > options.Limit {
		out.Items = out.Items[:options.Limit]
		last := out.Items[options.Limit-1]
		scope.Updated, scope.ID = last.UpdatedAt, last.ID
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	return out, nil
}
