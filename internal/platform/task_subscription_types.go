package platform

import (
	"regexp"
	"strings"
)

const taskSubscriptionInvalid = "Invalid task subscription request"
const taskSubscriptionChanged = "The task subscription changed; reload before deciding"
const taskSubscriptionSourceInvalid = "The task subscription notification source is invalid"
const taskSubscriptionMaxVersion = 2147483647

type TaskSubscription struct {
	ProjectID string `json:"project_id"`
	TaskID    string `json:"task_id"`
	UserID    string `json:"user_id"`
	Version   int    `json:"version"`
	Following bool   `json:"following"`
	UpdatedAt string `json:"updated_at"`
}
type TaskSubscriptionCommand struct {
	ProjectID       string `json:"project_id"`
	TaskID          string `json:"task_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type TaskSubscriptionReceipt struct {
	IdempotencyKey string           `json:"idempotency_key"`
	Subscription   TaskSubscription `json:"subscription"`
}

var taskSubscriptionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func taskSubscriptionID(value string) bool {
	return value != "." && value != ".." && taskSubscriptionIDPattern.MatchString(value)
}
func validTaskSubscriptionCommand(project, task string, r TaskSubscriptionCommand) bool {
	return r.ProjectID == project && r.TaskID == task && taskSubscriptionID(project) && taskSubscriptionID(task) && r.ExpectedVersion >= 0 && r.ExpectedVersion <= taskSubscriptionMaxVersion && (r.Action == "follow" || r.Action == "unfollow") && len(r.IdempotencyKey) >= 8 && len(r.IdempotencyKey) <= 128 && strings.TrimSpace(r.IdempotencyKey) != "" && taskWorkspaceText(r.IdempotencyKey)
}
