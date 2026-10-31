package platform

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

func validateKnowledgeDraft(request ProjectKnowledgeWriteRequest) error {
	if !knowledgeKind(request.Kind) || !knowledgeText(request.Title, 200) || strings.TrimSpace(request.Title) == "" || !knowledgeText(request.Summary, 1000) || !knowledgeText(request.Content, 16000) || strings.TrimSpace(request.Content) == "" {
		return invalid("知识需要有效类型、标题及不超过16000字节的正文")
	}
	if request.ExpectedVersion < 0 || request.ExpectedVersion > 2147483646 {
		return invalid("知识版本无效")
	}
	return nil
}

// The transaction owner admits the actual current credential before reading an
// operation receipt. Domain callbacks can only append revisions through proof.
func (s *Service) knowledgeCommand(ctx context.Context, actor Actor, project, id string, purpose commandPurpose, key string, body any, change func(context.Context, *authorizedCommand) (ProjectKnowledgeDetail, error)) (out ProjectKnowledgeDetail, err error) {
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purpose)
	if err != nil {
		return out, err
	}
	ctx = auth.ctx
	hash, replay, err := s.commandReceipt(ctx, auth, key, body)
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		if err = s.projectKnowledgeReceiptEligibilityTx(ctx, auth, project, &out); err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		return out, tx.Commit()
	}
	out, err = change(ctx, auth)
	if err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, key, hash, out); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	if err = tx.Commit(); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	return out, nil
}

func assertKnowledgeWrite(auth *authorizedCommand, project string) error {
	if err := assertProjectRead(auth, project); err != nil {
		return err
	}
	if auth.purpose != purposeProjectWrite && auth.purpose != purposeBrowserProjectApprove {
		return forbidden()
	}
	return nil
}
func (s *Service) insertKnowledgeRevisionTx(ctx context.Context, auth *authorizedCommand, project, id string, revision ProjectKnowledgeRevision) error {
	if err := assertKnowledgeWrite(auth, project); err != nil {
		return err
	}
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_knowledge_revisions(tenant_id,project_id,knowledge_id,revision,snapshot) VALUES(?,?,?,?,?)`), auth.target.tenantID, project, id, revision.Revision, stringify(revision))
	return err
}
func knowledgeRevision(request ProjectKnowledgeWriteRequest, sources []ProjectKnowledgeSource, method string, auth *authorizedCommand) ProjectKnowledgeRevision {
	r := ProjectKnowledgeRevision{Revision: 1, State: "draft", Kind: request.Kind, Title: request.Title, Summary: request.Summary, Content: request.Content, Sources: sources, Method: method, CreatedBy: auth.currentPrincipal.ID, CreatedAt: auth.checkedAt.Format(time.RFC3339Nano)}
	r.SHA256 = knowledgeRevisionHash(r)
	return r
}
func (s *Service) knowledgeCreateTx(ctx context.Context, auth *authorizedCommand, project string, request ProjectKnowledgeWriteRequest, sources []ProjectKnowledgeSource, method string) (ProjectKnowledgeDetail, error) {
	if err := assertKnowledgeWrite(auth, project); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	if request.ExpectedVersion != 0 {
		return ProjectKnowledgeDetail{}, invalid("新知识的预期版本必须为0")
	}
	id := newID()
	revision := knowledgeRevision(request, sources, method, auth)
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_knowledge(tenant_id,project_id,id,version,kind,state,latest_revision,published_revision,title,summary,created_at,updated_at) VALUES(?,?,?,1,?,'draft',1,0,?,?,?,?)`), auth.target.tenantID, project, id, request.Kind, request.Title, request.Summary, revision.CreatedAt, revision.CreatedAt)
	if err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	if err = s.insertKnowledgeRevisionTx(ctx, auth, project, id, revision); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	if err = s.commandActivity(ctx, auth, "knowledge.created", id, revision.Title); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	return s.knowledgeDetailTx(ctx, auth, project, id)
}
func (s *Service) CreateProjectKnowledge(ctx context.Context, actor Actor, project string, request ProjectKnowledgeWriteRequest) (ProjectKnowledgeDetail, error) {
	if request.ProjectID != project {
		return ProjectKnowledgeDetail{}, invalid("知识项目与当前范围不一致")
	}
	if err := validateKnowledgeDraft(request); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	return s.knowledgeCommand(ctx, actor, project, "", purposeProjectWrite, request.IdempotencyKey, map[string]any{"knowledge_create": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectKnowledgeDetail, error) {
		sources, err := s.resolveKnowledgeSourcesTx(ctx, auth, project, request.Sources)
		if err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		return s.knowledgeCreateTx(ctx, auth, project, request, sources, "manual")
	})
}
func (s *Service) ExtractProjectKnowledge(ctx context.Context, actor Actor, project string, request ProjectKnowledgeExtractionRequest) (ProjectKnowledgeDetail, error) {
	if request.ProjectID != project || !knowledgeKind(request.Kind) || len(request.Sources) < 1 {
		return ProjectKnowledgeDetail{}, invalid("提取草稿需要当前项目、有效类型及至少一个来源")
	}
	return s.knowledgeCommand(ctx, actor, project, "", purposeProjectWrite, request.IdempotencyKey, map[string]any{"knowledge_extract": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectKnowledgeDetail, error) {
		sources, err := s.resolveKnowledgeSourcesTx(ctx, auth, project, request.Sources)
		if err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		title := request.Title
		if title == "" {
			title = sources[0].Title
		}
		quotes := []string{}
		for _, source := range sources {
			quotes = append(quotes, source.Quote)
		}
		draft := ProjectKnowledgeWriteRequest{ProjectID: project, Kind: request.Kind, Title: title, Summary: request.Summary, Content: strings.Join(quotes, "\n\n")}
		if err = validateKnowledgeDraft(draft); err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		return s.knowledgeCreateTx(ctx, auth, project, draft, sources, "extractive")
	})
}
func (s *Service) UpdateProjectKnowledge(ctx context.Context, actor Actor, project, id string, request ProjectKnowledgeWriteRequest) (ProjectKnowledgeDetail, error) {
	if request.ProjectID != project || request.ExpectedVersion < 1 {
		return ProjectKnowledgeDetail{}, invalid("修改知识需要当前项目及预期版本")
	}
	if err := validateKnowledgeDraft(request); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	return s.knowledgeCommand(ctx, actor, project, id, purposeProjectWrite, request.IdempotencyKey, map[string]any{"knowledge_update": id, "request": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectKnowledgeDetail, error) {
		current, err := s.knowledgeDetailTx(ctx, auth, project, id)
		if err != nil {
			return current, err
		}
		if current.Version != request.ExpectedVersion {
			return current, conflict("知识已变化，请刷新后核对草稿")
		}
		if current.Kind != request.Kind {
			return current, invalid("知识创建后不能修改类型")
		}
		sources, err := s.resolveKnowledgeSourcesTx(ctx, auth, project, request.Sources)
		if err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		revision := knowledgeRevision(request, sources, "manual", auth)
		revision.BasedOnRevision = current.LatestRevision
		revision.Revision = current.LatestRevision + 1
		state := "draft"
		if current.PublishedRevision > 0 {
			state = "published"
		}
		if err = s.advanceKnowledgeTx(ctx, auth, project, current, revision, state, current.PublishedRevision); err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		if err = s.commandActivity(ctx, auth, "knowledge.draft_updated", id, revision.Title); err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		return s.currentKnowledgeDetailTx(ctx, auth, project, id)
	})
}
func (s *Service) advanceKnowledgeTx(ctx context.Context, auth *authorizedCommand, project string, current ProjectKnowledgeDetail, revision ProjectKnowledgeRevision, state string, published int) error {
	if err := assertKnowledgeWrite(auth, project); err != nil {
		return err
	}
	if current.Version >= 2147483646 || current.LatestRevision >= 2147483646 {
		return conflict("知识修订数量已达到上限")
	}
	if err := s.insertKnowledgeRevisionTx(ctx, auth, project, current.ID, revision); err != nil {
		return err
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_project_knowledge SET version=version+1,state=?,latest_revision=?,published_revision=?,title=?,summary=?,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), state, revision.Revision, published, revision.Title, revision.Summary, revision.CreatedAt, auth.target.tenantID, project, current.ID, current.Version)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return conflict("知识已变化，请刷新后核对草稿")
	}
	return nil
}
func (s *Service) currentKnowledgeDetailTx(ctx context.Context, auth *authorizedCommand, project, id string) (ProjectKnowledgeDetail, error) {
	detail, err := s.knowledgeDetailTx(ctx, auth, project, id)
	if err != nil {
		return detail, err
	}
	items := []ProjectKnowledgeDetail{detail}
	if err = s.knowledgeEligibilityTx(ctx, auth, project, items); err != nil {
		return ProjectKnowledgeDetail{}, err
	}
	return items[0], nil
}
func (s *Service) CommandProjectKnowledge(ctx context.Context, actor Actor, project, id string, request ProjectKnowledgeCommandRequest) (ProjectKnowledgeDetail, error) {
	if request.ProjectID != project || request.ExpectedVersion < 1 || request.ExpectedVersion > 2147483646 || request.Action != "publish" && request.Action != "retire" {
		return ProjectKnowledgeDetail{}, invalid("知识命令需要发布或退役及当前版本")
	}
	return s.knowledgeCommand(ctx, actor, project, id, purposeBrowserProjectApprove, request.IdempotencyKey, map[string]any{"knowledge_command": id, "request": request}, func(ctx context.Context, auth *authorizedCommand) (ProjectKnowledgeDetail, error) {
		current, err := s.knowledgeDetailTx(ctx, auth, project, id)
		if err != nil {
			return current, err
		}
		if current.Version != request.ExpectedVersion {
			return current, conflict("知识已变化，请刷新后核对草稿")
		}
		revision := current.Latest
		state := "published"
		published := current.LatestRevision + 1
		if request.Action == "publish" {
			if current.Latest.State != "draft" {
				return current, conflict("只有当前草稿可以发布")
			}
			records, e := s.knowledgeSourceRecordsTx(ctx, auth, project, revision.Sources)
			if e != nil {
				return ProjectKnowledgeDetail{}, e
			}
			states, e := s.knowledgeSourcesCurrentTx(ctx, auth.tx, auth.target.tenantID, project, revision.Sources, records, true)
			if e != nil {
				return ProjectKnowledgeDetail{}, e
			}
			for _, sourceCurrent := range states {
				if !sourceCurrent {
					return current, conflict("知识来源已变化或不可访问，请重新选择")
				}
			}
		} else {
			if current.State == "retired" {
				return current, conflict("知识已经退役")
			}
			state = "retired"
			published = 0
		}
		revision.BasedOnRevision = current.LatestRevision
		revision.Revision = current.LatestRevision + 1
		revision.State = state
		revision.CreatedBy = auth.currentPrincipal.ID
		revision.CreatedAt = auth.checkedAt.Format(time.RFC3339Nano)
		if err = s.advanceKnowledgeTx(ctx, auth, project, current, revision, state, published); err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		if err = s.commandActivity(ctx, auth, "knowledge."+request.Action, id, revision.Title); err != nil {
			return ProjectKnowledgeDetail{}, err
		}
		return s.currentKnowledgeDetailTx(ctx, auth, project, id)
	})
}

// A receipt remains immutable historical evidence. Only this returned eligibility
// projection is refreshed, bound to the receipt's own approved revision/hash;
// it never mixes a newer published body into an older operation response.
func (s *Service) projectKnowledgeReceiptEligibilityTx(ctx context.Context, auth *authorizedCommand, project string, out *ProjectKnowledgeDetail) error {
	if out.Published == nil {
		return nil
	}
	current, err := s.knowledgeDetailTx(ctx, auth, project, out.ID)
	if err != nil {
		return err
	}
	out.PublishedEligible = false
	if current.State == "retired" {
		out.Eligibility = "retired"
		return nil
	}
	if current.Published == nil || current.PublishedRevision != out.Published.Revision || current.Published.SHA256 != out.Published.SHA256 {
		out.Eligibility = "publication_changed"
		return nil
	}
	items := []ProjectKnowledgeDetail{*out}
	if err = s.knowledgeEligibilityTx(ctx, auth, project, items); err != nil {
		return err
	}
	out.PublishedEligible = items[0].PublishedEligible
	out.Eligibility = items[0].Eligibility
	return nil
}
