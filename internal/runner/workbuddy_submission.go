package runner

import (
	"context"

	"github.com/ai-mada/suma/internal/workbuddy"
)

type workBuddySubmissionError struct{ failure workbuddy.Error }

func (e *workBuddySubmissionError) Error() string {
	switch e.failure.Code {
	case "submission_configuration_invalid":
		return "WorkBuddy submission configuration is invalid"
	case "submission_context_cancelled":
		return "WorkBuddy submission was cancelled before preparation"
	case "submission_task_checkpoint_failed":
		return "WorkBuddy task was created but its identity could not be checkpointed; reconcile the original task"
	case "submission_requires_reconciliation":
		return "WorkBuddy submission outcome requires reconciliation; no new task was created by this call"
	default:
		return "WorkBuddy original submission checkpoint is unavailable; reconcile before continuing"
	}
}
func (e *workBuddySubmissionError) Unwrap() error { return &e.failure }
func workBuddySubmissionFailure(code string, unknown bool) error {
	return &workBuddySubmissionError{failure: workbuddy.Error{Code: code, OutcomeUnknown: unknown}}
}

// submitWorkBuddyTask is the transport/journal seam for a future authorized
// cloud adapter. It grants no dispatch authority and is deliberately not wired
// into Execute or capability discovery until the full cloud contract is ready.
// A durable prepared record may already represent a chargeable request; only
// the first successful preparation may submit. Recovery reads an exact known
// task, never guesses by title or resends an initial prompt.
func submitWorkBuddyTask(ctx context.Context, journal *Journal, assignment Assignment, client *workbuddy.Client, request workbuddy.CreateRequest, credentialRef string, credentialVersion int) (workbuddy.Task, error) {
	var empty workbuddy.Task
	if ctx == nil || journal == nil || client == nil {
		return empty, workBuddySubmissionFailure("submission_configuration_invalid", false)
	}
	if ctx.Err() != nil {
		return empty, workBuddySubmissionFailure("submission_context_cancelled", false)
	}
	requestSHA, err := workbuddy.CreateRequestDigest(request)
	if err != nil {
		return empty, err
	}
	binding := WorkBuddyBinding{RequestSHA256: requestSHA, CredentialRef: credentialRef, CredentialVersion: credentialVersion, Endpoint: client.Endpoint()}
	readOriginal := func(checkpoint WorkBuddyCheckpoint) (workbuddy.Task, error) {
		if checkpoint.Binding != binding {
			return empty, workBuddySubmissionFailure("submission_checkpoint_unavailable", true)
		}
		if checkpoint.State != "task_known" || checkpoint.TaskID == "" {
			return empty, workBuddySubmissionFailure("submission_requires_reconciliation", true)
		}
		task, err := client.GetTask(ctx, checkpoint.TaskID)
		if err != nil {
			// A failed read cannot erase the already known remote identity.
			return workbuddy.Task{ID: checkpoint.TaskID}, err
		}
		return task, nil
	}
	// Restart recovery may already have recorded an unknown final Runner result.
	// Reading that original cloud identity remains legal; Begin must continue to
	// reject new writes on a final assignment.
	checkpoint, exists, err := journal.WorkBuddySubmission(assignment)
	if err != nil {
		return empty, workBuddySubmissionFailure("submission_checkpoint_unavailable", true)
	}
	if exists {
		return readOriginal(checkpoint)
	}
	first, err := journal.BeginWorkBuddySubmission(assignment, binding)
	if err != nil {
		// A conflicting or unreadable original record must not imply that a
		// previously started external effect did not happen.
		return empty, workBuddySubmissionFailure("submission_checkpoint_unavailable", true)
	}
	if !first {
		checkpoint, exists, readErr := journal.WorkBuddySubmission(assignment)
		if readErr != nil || !exists || checkpoint.Binding != binding {
			return empty, workBuddySubmissionFailure("submission_checkpoint_unavailable", true)
		}
		return readOriginal(checkpoint)
	}

	task, err := client.CreateTask(ctx, request)
	if err != nil {
		// Keep prepared even for a local cancellation between preparation and
		// transport. Nothing grants a second create permit for this attempt.
		return empty, err
	}
	if err = journal.RecordWorkBuddyTask(assignment, binding, task.ID); err != nil {
		// Preserve the known remote identity in the result for explicit recovery
		// while retaining prepared locally. A later call still cannot re-create.
		return task, workBuddySubmissionFailure("submission_task_checkpoint_failed", true)
	}
	return task, nil
}
