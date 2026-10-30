package platform

import (
	"context"
	"encoding/json"
)

// The reverse notification reference belongs to the stored source, not the
// public handoff or its original command receipt. The embedded fields retain
// the existing flattened payload representation.
type taskHandoffStored struct {
	TaskHandoff
	NotificationID string `json:"notification_id,omitempty"`
}

func decodeTaskHandoffPayload(raw, id, project, taskID, state string, version int) (out taskHandoffStored, valid bool) {
	if json.Unmarshal([]byte(raw), &out) != nil {
		return out, false
	}
	if !handoffID(id, false) || !handoffID(taskID, false) || out.ID != id || out.ProjectID != project || out.TaskID != taskID || version < 1 || out.Version != version || out.State != state || !handoffID(out.ToUserID, false) || !handoffID(out.ToResourceID, false) || out.Basis.TaskVersion < 1 {
		return out, false
	}
	return out, state == "pending" || state == "accepted" || state == "returned" || state == "withdrawn"
}

func handoffNotificationSourceAvailable(stored taskHandoffStored, item NotificationInboxItem, recipient string) bool {
	return stored.NotificationID == item.ID && stored.ToUserID == recipient && stored.ToResourceID == item.SourceResourceID && stored.Basis.TaskVersion == item.SourceTaskVersion
}

// GetTaskHandoff resolves one explicit source, including terminal history. It
// does not enumerate recipients or scan history pages, and reuses the original
// current-basis and action projection without granting any command authority.
func (s *Service) GetTaskHandoff(ctx context.Context, actor Actor, project, taskID, id string) (out TaskHandoff, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(taskID, false) || !handoffID(id, false) {
		return out, invalid("Invalid task handoff page")
	}
	ctx, auth, cleanup, err := s.handoffAuthority(ctx, actor, project, id, false)
	if err != nil {
		return out, err
	}
	defer cleanup()
	out, err = s.taskHandoffTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	if out.TaskID != taskID {
		return TaskHandoff{}, missing()
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return TaskHandoff{}, err
	}
	basis, blockers, err := s.handoffBasisTx(ctx, auth, task)
	if err != nil {
		return TaskHandoff{}, err
	}
	return s.handoffProjectionTx(ctx, auth, out, basis, blockers)
}
