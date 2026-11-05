package platform

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const notificationOrganizationInboxChanged = "Accessible inbox projects changed; refresh the inbox"

// Removing the canonical UTC suffix leaves a lexically sortable timestamp.
// For an instant on the whole second the shorter prefix sorts before every
// fractional instant in that second. This also preserves nanosecond order.
const notificationOrganizationInboxTimeOrder = `REPLACE(n.created_at,'Z','')`

type notificationOrganizationInboxCursor struct {
	Version       int    `json:"version"`
	Organization  string `json:"organization"`
	User          string `json:"user"`
	Filter        string `json:"filter"`
	Limit         int    `json:"limit"`
	ScopeRevision string `json:"scope_revision"`
	CreatedAt     string `json:"created_at"`
	ID            string `json:"id"`
}

func (s *Service) NotificationOrganizationInbox(ctx context.Context, actor Actor, filter string, limit int, cursor string) (out NotificationOrganizationInboxPage, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if filter == "" {
		filter = "inbox"
	}
	if limit == 0 {
		limit = 20
	}
	if !validNotificationInboxRead(filter, limit, cursor) {
		return out, invalid(notificationInboxInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.notificationOrganizationInboxTx(bounded, auth, filter, limit, cursor)
		return err
	})
	if err != nil {
		return NotificationOrganizationInboxPage{}, err
	}
	return out, nil
}

func (s *Service) notificationOrganizationInboxTx(ctx context.Context, auth *authorizedCommand, filter string, limit int, cursor string) (out NotificationOrganizationInboxPage, err error) {
	target := commandTarget{tenantID: auth.currentPrincipal.TenantID, projectID: organizationScope}
	if err = auth.assertTx(auth.tx, target, purposeOrganizationRead); err != nil {
		return out, err
	}

	// The authority transaction holds the organization lock used by project
	// grants and project lifecycle writes. Counts, page rows, and source reads
	// all use this same current scope. Match the project-read roles exactly.
	visibleWhere := ` WHERE p.tenant_id=?`
	visibleArgs := []any{target.tenantID}
	if !orgAdmin(auth.currentPrincipal) {
		role := "m.role"
		if s.Dialect == "oceanbase" {
			// MySQL-compatible collations may fold case or ignore trailing
			// spaces, while validProjectRole compares the exact stored value.
			role = "BINARY m.role"
		}
		visibleWhere += ` AND EXISTS (SELECT 1 FROM suma_project_members m WHERE m.project_id=p.id AND m.user_id=? AND ` + role + ` IN ('admin','developer','reviewer','viewer'))`
		visibleArgs = append(visibleArgs, auth.currentPrincipal.ID)
	}
	revision, err := s.notificationOrganizationInboxRevisionTx(ctx, auth, visibleWhere, visibleArgs)
	if err != nil {
		return out, err
	}
	scope := notificationOrganizationInboxCursor{
		Version: 1, Organization: target.tenantID, User: auth.currentPrincipal.ID, Filter: filter, Limit: limit, ScopeRevision: revision,
	}
	where := visibleWhere + ` AND n.recipient_user_id=?`
	args := append(visibleArgs, auth.currentPrincipal.ID)
	var before notificationOrganizationInboxCursor
	if cursor != "" {
		if before, err = parseNotificationOrganizationInboxCursor(cursor, scope); err != nil {
			return out, err
		}
	}

	out = NotificationOrganizationInboxPage{
		OrganizationID: target.tenantID, UserID: auth.currentPrincipal.ID, Limit: limit, ScopeRevision: revision,
		AsOf: auth.checkedAt.UTC().Format(time.RFC3339Nano),
	}
	out.Counts, err = s.notificationInboxCountsTx(ctx, auth, where, args)
	if err != nil {
		return out, err
	}
	condition, total := notificationInboxFilter(filter, out.Counts)
	where += condition
	out.Total = total
	if cursor != "" {
		instant := strings.TrimSuffix(before.CreatedAt, "Z")
		where += " AND (" + notificationOrganizationInboxTimeOrder + "<? OR (" + notificationOrganizationInboxTimeOrder + "=? AND n.id<?))"
		args = append(args, instant, instant, before.ID)
	}
	out.Items, err = s.notificationInboxItemsTx(ctx, auth, where, args, notificationOrganizationInboxTimeOrder+" DESC,n.id DESC", limit)
	if err != nil {
		return out, err
	}
	for _, row := range out.Items {
		if !notificationOrganizationInboxInstant(row.Notification.CreatedAt) {
			return out, conflict("The notification source is invalid")
		}
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1].Notification
		scope.CreatedAt, scope.ID = last.CreatedAt, last.ID
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	return out, nil
}

func (s *Service) notificationOrganizationInboxRevisionTx(ctx context.Context, auth *authorizedCommand, where string, args []any) (string, error) {
	rows, err := auth.tx.QueryContext(ctx, s.q("SELECT p.id FROM suma_projects p"+where+" ORDER BY p.id"), args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	hash := sha256.New()
	hash.Write([]byte("notification-inbox-organization:v1\n" + strconv.Quote(auth.target.tenantID) + "\n" + strconv.Quote(auth.currentPrincipal.ID)))
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return "", err
		}
		hash.Write([]byte("\n" + strconv.Quote(id)))
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func notificationOrganizationInboxInstant(raw string) bool {
	value, err := time.Parse(time.RFC3339Nano, raw)
	return err == nil && value.UTC().Format(time.RFC3339Nano) == raw
}

func parseNotificationOrganizationInboxCursor(cursor string, scope notificationOrganizationInboxCursor) (before notificationOrganizationInboxCursor, err error) {
	raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if decodeErr != nil || json.Unmarshal(raw, &before) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(before))) != cursor {
		return before, invalid(notificationInboxInvalid)
	}
	hash, decodeErr := hex.DecodeString(before.ScopeRevision)
	if decodeErr != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != before.ScopeRevision ||
		!notificationOrganizationInboxInstant(before.CreatedAt) || !handoffID(before.ID, false) {
		return before, invalid(notificationInboxInvalid)
	}
	identity := before
	identity.CreatedAt, identity.ID, identity.ScopeRevision = "", "", scope.ScopeRevision
	if identity != scope {
		return before, invalid(notificationInboxInvalid)
	}
	if before.ScopeRevision != scope.ScopeRevision {
		return before, conflict(notificationOrganizationInboxChanged)
	}
	return before, nil
}
