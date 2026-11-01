package platform

import (
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
)

type TaskDevelopmentContinuationOption struct {
	Selection dc.DevelopmentContinuationSource `json:"selection"`
	Candidate cc.Binding                       `json:"candidate"`
	NodeName  string                           `json:"node_name"`
}

type TaskDevelopmentContinuationOptions struct {
	ProjectID        string                              `json:"project_id"`
	TaskID           string                              `json:"task_id"`
	TaskVersion      int                                 `json:"task_version"`
	SourceTaskRunID  string                              `json:"source_task_run_id"`
	SourceRunVersion int                                 `json:"source_run_version"`
	SourceRunState   string                              `json:"source_run_state"`
	Items            []TaskDevelopmentContinuationOption `json:"items"`
	Total            int                                 `json:"total"`
	Limit            int                                 `json:"limit"`
	NextCursor       string                              `json:"next_cursor,omitempty"`
	Complete         bool                                `json:"complete"`
	Reason           string                              `json:"reason,omitempty"`
}
