package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const projectKnowledgeRevisionInvalid = "Invalid project knowledge revision request"
const projectKnowledgeRevisionSource = "The immutable project knowledge revision is inconsistent"

func (s *Service) GetProjectKnowledgeRevision(ctx context.Context, actor Actor, project, id string, revision int) (out ProjectKnowledgeRevisionDetail, err error) {
	if !handoffID(project, false) || !handoffID(id, false) || revision < 1 || revision > 2147483647 {
		return out, invalid(projectKnowledgeRevisionInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead, func(auth *authorizedCommand) error {
		// Existence is scoped to the current authorized project. A historical
		// read deliberately does not select today's latest/published pointer.
		var exists string
		readErr := auth.tx.QueryRowContext(auth.ctx, s.q(`SELECT id FROM suma_project_knowledge WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, id).Scan(&exists)
		if errors.Is(readErr, sql.ErrNoRows) {
			return missing()
		}
		if readErr != nil {
			return readErr
		}
		var raw string
		readErr = auth.tx.QueryRowContext(auth.ctx, s.q(`SELECT snapshot FROM suma_project_knowledge_revisions WHERE tenant_id=? AND project_id=? AND knowledge_id=? AND revision=?`), auth.target.tenantID, project, id, revision).Scan(&raw)
		if errors.Is(readErr, sql.ErrNoRows) {
			return missing()
		}
		if readErr != nil {
			return readErr
		}
		var snapshot ProjectKnowledgeRevision
		if json.Unmarshal([]byte(raw), &snapshot) != nil || snapshot.Revision != revision || !knowledgeKind(snapshot.Kind) || !knowledgeMethod(snapshot.Method) || !knowledgeState(snapshot.State) || !contentDigest.MatchString(snapshot.SHA256) || knowledgeRevisionHash(snapshot) != snapshot.SHA256 {
			return conflict(projectKnowledgeRevisionSource)
		}
		out = ProjectKnowledgeRevisionDetail{ProjectID: project, KnowledgeID: id, Snapshot: snapshot}
		return nil
	})
	if err != nil {
		return ProjectKnowledgeRevisionDetail{}, err
	}
	return out, nil
}
