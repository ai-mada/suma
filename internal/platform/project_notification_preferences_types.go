package platform

import "strings"

const projectNotificationPreferencesInvalid = "Invalid project notification preferences request"
const projectNotificationPreferencesChanged = "The project notification preferences changed; reload before deciding"

type ProjectNotificationPreferences struct {
	ProjectID                string `json:"project_id"`
	UserID                   string `json:"user_id"`
	Version                  int    `json:"version"`
	FollowedDiscussionsMuted bool   `json:"followed_discussions_muted"`
	UpdatedAt                string `json:"updated_at"`
}

type ProjectNotificationPreferencesCommand struct {
	ProjectID                string `json:"project_id"`
	ExpectedVersion          int    `json:"expected_version"`
	FollowedDiscussionsMuted bool   `json:"followed_discussions_muted"`
	IdempotencyKey           string `json:"idempotency_key"`
}

type ProjectNotificationPreferencesReceipt struct {
	IdempotencyKey string                         `json:"idempotency_key"`
	Preferences    ProjectNotificationPreferences `json:"preferences"`
}

func validProjectNotificationPreferencesCommand(project string, req ProjectNotificationPreferencesCommand) bool {
	return taskSubscriptionID(project) && req.ProjectID == project && req.ExpectedVersion >= 0 && req.ExpectedVersion <= taskSubscriptionMaxVersion && len(req.IdempotencyKey) >= 8 && len(req.IdempotencyKey) <= 128 && strings.TrimSpace(req.IdempotencyKey) != "" && taskWorkspaceText(req.IdempotencyKey)
}
