package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/ai-mada/suma/internal/strictjson"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type toolReplacementCursor struct {
	Tenant    string `json:"tenant"`
	Project   string `json:"project"`
	Actor     string `json:"actor"`
	Profile   string `json:"profile"`
	Direction string `json:"direction"`
	Limit     int    `json:"limit"`
	Created   string `json:"created"`
	ID        string `json:"id"`
}

func normalizeToolReplacementQuery(q ToolProfileReplacementListQuery) (ToolProfileReplacementListQuery, error) {
	if q.Direction == "" {
		q.Direction = "outgoing"
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Direction != "outgoing" && q.Direction != "incoming" || q.Limit < 1 || q.Limit > 100 || len(q.Cursor) > 2048 {
		return q, toolReplacementInvalid()
	}
	return q, nil
}
func (s *Service) GetToolProfileReplacement(ctx context.Context, actor Actor, project, id string) (out ToolProfileReplacement, err error) {
	return s.getToolProfileReplacement(ctx, actor, project, id, func() error { return nil })
}
func (s *Service) getToolProfileReplacement(ctx context.Context, actor Actor, project, id string, check func() error) (out ToolProfileReplacement, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		if e := check(); e != nil {
			return e
		}
		if !toolcontract.ValidCommandID(id) {
			return toolReplacementInvalid()
		}
		var e error
		out, e = s.toolProfileReplacementTx(auth.ctx, auth.tx, auth.target.tenantID, project, id)
		return e
	})
	return
}
func (s *Service) ListToolProfileReplacements(ctx context.Context, actor Actor, project, profile string, q ToolProfileReplacementListQuery) (ToolProfileReplacementList, error) {
	return s.listToolProfileReplacements(ctx, actor, project, profile, func() (ToolProfileReplacementListQuery, error) { return q, nil })
}
func (s *Service) listToolProfileReplacements(ctx context.Context, actor Actor, project, profile string, parse func() (ToolProfileReplacementListQuery, error)) (out ToolProfileReplacementList, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: profile}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		q, e := parse()
		if e != nil {
			return e
		}
		q, e = normalizeToolReplacementQuery(q)
		if e != nil {
			return e
		}
		if !toolcontract.ValidCommandID(profile) {
			return toolReplacementInvalid()
		}
		if _, e = s.toolProfileTx(auth.ctx, auth.tx, auth.currentPrincipal, project, profile); e != nil {
			return e
		}
		out = ToolProfileReplacementList{ProjectID: project, ProfileID: profile, Direction: q.Direction, Items: []ToolProfileReplacement{}}
		basis := toolReplacementCursor{Tenant: auth.target.tenantID, Project: project, Actor: auth.currentPrincipal.ID, Profile: profile, Direction: q.Direction, Limit: q.Limit}
		cursor := basis
		if q.Cursor != "" {
			raw, e := base64.RawURLEncoding.DecodeString(q.Cursor)
			if e != nil || strictjson.Decode(raw, &cursor, 2048) != nil {
				return toolReplacementInvalid()
			}
			stamp, e := time.Parse(replacementStamp, cursor.Created)
			if e != nil || stamp.UTC().Format(replacementStamp) != cursor.Created || !toolcontract.ValidCommandID(cursor.ID) || cursor.Tenant != basis.Tenant || cursor.Project != basis.Project || cursor.Actor != basis.Actor || cursor.Profile != basis.Profile || cursor.Direction != basis.Direction || cursor.Limit != basis.Limit {
				return toolReplacementInvalid()
			}
		}
		column := "original_id"
		if q.Direction == "incoming" {
			column = "replacement_id"
		}
		where := `tenant_id=? AND project_id=? AND ` + column + `=?`
		args := []any{auth.target.tenantID, project, profile}
		if q.Cursor != "" {
			where += ` AND (created_at<? OR(created_at=? AND id<?))`
			args = append(args, cursor.Created, cursor.Created, cursor.ID)
		}
		args = append(args, q.Limit+1)
		rows, e := auth.tx.QueryContext(auth.ctx, s.q(`SELECT id FROM suma_tool_profile_replacements WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), args...)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		last := basis
		framing, e := json.Marshal(out)
		if e != nil {
			return e
		}
		used := len(framing)
		for _, id := range ids {
			if len(out.Items) == q.Limit {
				out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(last)))
				break
			}
			item, e := s.toolProfileReplacementTx(auth.ctx, auth.tx, auth.target.tenantID, project, id)
			if e != nil {
				return e
			}
			if q.Direction == "outgoing" && item.Original.ID != profile || q.Direction == "incoming" && item.Replacement.ID != profile {
				return toolReplacementChanged()
			}
			raw, e := json.Marshal(item)
			if e != nil {
				return e
			}
			// A full snapshot may be large. Stop before the first unreturned
			// candidate and keep the cursor on the last returned one; never skip it.
			additional := len(raw)
			if len(out.Items) > 0 {
				additional++
			}
			if used+additional > (8<<20)-2048 {
				if len(out.Items) == 0 {
					return toolReplacementChanged()
				}
				out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(last)))
				break
			}
			out.Items = append(out.Items, item)
			used += additional
			last.Created, last.ID = item.CreatedAt, item.ID
		}
		return nil
	})
	return
}
