package platform

import "context"

// A preview accepts references, never caller-provided knowledge content. The
// same resolver is used before dispatch acceptance, so a retired publication,
// changed source or different revision cannot silently enter the reviewed goal.
func (s *Service) taskContextForDispatchTx(ctx context.Context, auth *authorizedCommand, project, taskID string, refs []ProjectKnowledgeReference) (TaskContext, error) {
	var out TaskContext
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	out, err := s.taskContextTx(auth.ctx, auth.tx, auth.currentPrincipal, project, taskID)
	if err != nil {
		return out, err
	}
	out.Design, err = s.resolveTaskRequirementDesignTx(auth.ctx, auth, project, taskID)
	if err != nil {
		return out, err
	}
	if len(refs) == 0 {
		return out, nil
	}
	if len(refs) > 16 {
		return out, invalid("每次派发最多选择16个项目知识修订")
	}
	out.Knowledge, err = s.resolvePublishedKnowledgeTx(auth.ctx, auth, project, refs)
	return out, err
}

func knowledgeReferences(items []ProjectKnowledgeContextItem) []ProjectKnowledgeReference {
	if len(items) == 0 {
		return nil
	}
	refs := make([]ProjectKnowledgeReference, len(items))
	for i, item := range items {
		refs[i] = ProjectKnowledgeReference{ID: item.ID, Revision: item.Revision, SHA256: item.SHA256}
	}
	return refs
}
