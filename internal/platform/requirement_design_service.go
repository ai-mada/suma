package platform

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func assertDesignWrite(auth *authorizedCommand, project string) error {
	if e := assertProjectRead(auth, project); e != nil {
		return e
	}
	if auth.purpose != purposeProjectWrite && auth.purpose != purposeBrowserProjectApprove {
		return forbidden()
	}
	return assertProjectCommand(auth.tx, auth, project)
}

// Only actual BrowserProjectApprove admits release effects. All effect-bearing
// commands share Review's project guard before reading records or replaying a
// receipt, so current authorization and evidence cannot race a design change.
func (s *Service) designCommand(ctx context.Context, actor Actor, project, requirement string, purpose commandPurpose, key string, body any, change func(context.Context, *authorizedCommand) (RequirementDesignView, error)) (out RequirementDesignView, err error) {
	ctx, tx, cleanup, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return out, e
	}
	defer cleanup()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: requirement}, purpose)
	if e != nil {
		return out, e
	}
	ctx = auth.ctx
	if purpose == purposeBrowserProjectApprove {
		if e = dag.GuardProjectTx(ctx, tx, s.Dialect, auth.target.tenantID, project); e != nil {
			return out, e
		}
	}
	hash, replay, e := s.commandReceipt(ctx, auth, key, body)
	if e != nil {
		return out, e
	}
	if replay {
		if e = json.Unmarshal([]byte(hash), &out); e != nil {
			return out, e
		}
		if e = s.designReceiptEligibilityTx(ctx, auth, project, requirement, &out); e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	out, e = change(ctx, auth)
	if e != nil {
		return RequirementDesignView{}, e
	}
	if e = s.saveCommandReceipt(ctx, auth, key, hash, out); e != nil {
		return RequirementDesignView{}, e
	}
	return out, tx.Commit()
}
func (s *Service) designReceiptEligibilityTx(ctx context.Context, auth *authorizedCommand, project, requirement string, out *RequirementDesignView) error {
	// Historical success bodies stay immutable. Only current eligibility of the
	// receipt's own publication is recomputed; never splice in newer design text.
	if out.Published == nil {
		out.Eligibility = "none"
		if out.Latest != nil && out.Latest.State == "retired" {
			out.Eligibility = "retired"
		}
		return nil
	}
	current, e := s.designViewTx(ctx, auth, project, requirement)
	if e != nil {
		return e
	}
	if current.Published == nil || current.PublishedRevision != out.PublishedRevision || current.Published.SHA256 != out.Published.SHA256 {
		out.Eligibility = "publication_changed"
		return nil
	}
	out.Eligibility = current.Eligibility
	return nil
}
func (s *Service) insertDesignRevisionTx(ctx context.Context, auth *authorizedCommand, project, requirement string, revision RequirementDesignRevision) error {
	if e := assertDesignWrite(auth, project); e != nil {
		return e
	}
	_, e := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_requirement_design_revisions(tenant_id,project_id,requirement_id,revision,snapshot)VALUES(?,?,?,?,?)`), auth.target.tenantID, project, requirement, revision.Revision, stringify(revision))
	return e
}
func (s *Service) advanceDesignTx(ctx context.Context, auth *authorizedCommand, project, requirement string, current designAggregate, revision RequirementDesignRevision, state string, published int) error {
	if e := assertDesignWrite(auth, project); e != nil {
		return e
	}
	if e := s.insertDesignRevisionTx(ctx, auth, project, requirement, revision); e != nil {
		return e
	}
	if current.Version == 0 {
		_, e := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_requirement_designs(tenant_id,project_id,requirement_id,version,latest_revision,published_revision,state,last_adoption_id,created_at,updated_at)VALUES(?,?,?,1,?,?,?,'',?,?)`), auth.target.tenantID, project, requirement, revision.Revision, published, state, revision.CreatedAt, revision.CreatedAt)
		return e
	}
	res, e := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_requirement_designs SET version=version+1,latest_revision=?,published_revision=?,state=?,updated_at=? WHERE tenant_id=? AND project_id=? AND requirement_id=? AND version=?`), revision.Revision, published, state, revision.CreatedAt, auth.target.tenantID, project, requirement, current.Version)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return conflict("需求设计已变化，请刷新后核对草稿")
	}
	return nil
}
func (s *Service) SaveRequirementDesign(ctx context.Context, actor Actor, project, requirement string, request RequirementDesignSaveRequest) (RequirementDesignView, error) {
	if request.ProjectID != project || !designVersion(request.ExpectedVersion) || !designVersion(request.ExpectedRequirementVersion) || request.ExpectedRequirementVersion < 1 {
		return RequirementDesignView{}, invalid("需求设计项目或预期版本无效")
	}
	if e := validateDesignDraft(request.Content, request.Tasks); e != nil {
		return RequirementDesignView{}, e
	}
	return s.designCommand(ctx, actor, project, requirement, purposeProjectWrite, request.IdempotencyKey, map[string]any{"design_save": requirement, "request": request}, func(ctx context.Context, auth *authorizedCommand) (RequirementDesignView, error) {
		source, e := s.getRecordTx(ctx, auth, project, "requirements", requirement)
		if e != nil {
			return RequirementDesignView{}, e
		}
		current, e := s.designAggregateTx(ctx, auth, project, requirement)
		if e != nil {
			return RequirementDesignView{}, e
		}
		if current.Version != request.ExpectedVersion || source.Version != request.ExpectedRequirementVersion {
			return RequirementDesignView{}, conflict("需求或设计已变化，请核对当前版本")
		}
		if e = s.validateDesignOwnersTx(ctx, auth, project, request.Tasks); e != nil {
			return RequirementDesignView{}, e
		}
		revision := RequirementDesignRevision{Revision: current.Latest + 1, State: "draft", Requirement: designSnapshot(source), Content: request.Content, Tasks: request.Tasks, CreatedBy: auth.currentPrincipal.ID, CreatedAt: auth.checkedAt.Format(time.RFC3339Nano)}
		revision.SHA256 = designRevisionHash(revision)
		state := "draft"
		if current.Published > 0 {
			state = "published"
		}
		if e = s.advanceDesignTx(ctx, auth, project, requirement, current, revision, state, current.Published); e != nil {
			return RequirementDesignView{}, e
		}
		if e = s.commandActivity(ctx, auth, "requirement.design_saved", requirement, source.Title); e != nil {
			return RequirementDesignView{}, e
		}
		return s.designViewTx(ctx, auth, project, requirement)
	})
}
func (s *Service) CommandRequirementDesign(ctx context.Context, actor Actor, project, requirement string, request RequirementDesignCommandRequest) (RequirementDesignView, error) {
	if request.ProjectID != project || !designVersion(request.ExpectedVersion) || request.ExpectedVersion < 1 || !designVersion(request.ExpectedRequirementVersion) || request.ExpectedRequirementVersion < 1 || request.Action != "publish" && request.Action != "retire" {
		return RequirementDesignView{}, invalid("需求设计命令或预期版本无效")
	}
	return s.designCommand(ctx, actor, project, requirement, purposeBrowserProjectApprove, request.IdempotencyKey, map[string]any{"design_command": requirement, "request": request}, func(ctx context.Context, auth *authorizedCommand) (RequirementDesignView, error) {
		current, e := s.designAggregateTx(ctx, auth, project, requirement)
		if e != nil {
			return RequirementDesignView{}, e
		}
		source, e := s.designSourceTx(ctx, auth, project, requirement, true)
		if e != nil {
			return RequirementDesignView{}, e
		}
		if current.Version != request.ExpectedVersion || source.Version != request.ExpectedRequirementVersion || current.Latest < 1 {
			return RequirementDesignView{}, conflict("需求或设计已变化，请核对当前版本")
		}
		revision, e := s.designRevisionTx(ctx, auth, project, requirement, current.Latest)
		if e != nil {
			return RequirementDesignView{}, e
		}
		published, state := 0, "retired"
		if request.Action == "publish" {
			if !designSourceCurrent(source, revision.Requirement) {
				return RequirementDesignView{}, conflict("需求来源已变化，须审阅并保存新的设计草稿")
			}
			if e = validateDesignDraft(revision.Content, revision.Tasks); e != nil {
				return RequirementDesignView{}, e
			}
			if len(designDiagnostics(revision)) > 0 {
				return RequirementDesignView{}, invalid("设计尚不完整，请修正发布校验问题")
			}
			if e = s.validateDesignOwnersTx(ctx, auth, project, revision.Tasks); e != nil {
				return RequirementDesignView{}, e
			}
			state = "published"
			published = current.Latest + 1
		}
		revision.Revision = current.Latest + 1
		revision.State = state
		revision.CreatedBy = auth.currentPrincipal.ID
		revision.CreatedAt = auth.checkedAt.Format(time.RFC3339Nano)
		links, e := s.designLinksTx(ctx, auth, project, requirement)
		if e != nil {
			return RequirementDesignView{}, e
		}
		roots := []string{}
		for _, l := range links {
			roots = append(roots, l.TaskID)
		}
		if len(roots) > 0 {
			if e = s.InvalidateDependentsTx(ctx, auth.tx, auth, project, roots, true); e != nil {
				return RequirementDesignView{}, e
			}
		}
		if e = s.advanceDesignTx(ctx, auth, project, requirement, current, revision, state, published); e != nil {
			return RequirementDesignView{}, e
		}
		if e = s.commandActivity(ctx, auth, "requirement.design_"+request.Action, requirement, source.Title); e != nil {
			return RequirementDesignView{}, e
		}
		return s.designViewTx(ctx, auth, project, requirement)
	})
}
