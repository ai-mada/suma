package platform

// Worklog is one complete immutable manual human revision. The current row is
// an index of its latest revision; display permission flags are recomputed.
type Worklog struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	Version       int    `json:"version"`
	Status        string `json:"status"`
	Source        string `json:"source"`
	TaskID        string `json:"task_id"`
	TaskVersion   int    `json:"task_version"`
	TaskTitle     string `json:"task_title"`
	MemberID      string `json:"member_id"`
	MemberVersion int    `json:"member_version"`
	MemberName    string `json:"member_name"`
	UserID        string `json:"user_id"`
	UserName      string `json:"user_name"`
	WorkDate      string `json:"work_date"`
	Minutes       int    `json:"minutes"`
	Note          string `json:"note"`
	Reason        string `json:"reason"`
	ActorID       string `json:"actor_id"`
	ActorName     string `json:"actor_name"`
	OnBehalfAck   bool   `json:"on_behalf_ack"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	CanCorrect    bool   `json:"can_correct"`
	CanVoid       bool   `json:"can_void"`
}

type WorklogCreateRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	MemberID            string `json:"member_id"`
	WorkDate            string `json:"work_date"`
	Minutes             int    `json:"minutes"`
	Note                string `json:"note"`
	OnBehalfAck         bool   `json:"on_behalf_ack"`
	IdempotencyKey      string `json:"idempotency_key"`
}

type WorklogCorrectionRequest struct {
	ProjectID           string `json:"project_id"`
	ExpectedVersion     int    `json:"expected_version"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	WorkDate            string `json:"work_date"`
	Minutes             int    `json:"minutes"`
	Note                string `json:"note"`
	Reason              string `json:"reason"`
	OnBehalfAck         bool   `json:"on_behalf_ack"`
	IdempotencyKey      string `json:"idempotency_key"`
}

type WorklogVoidRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason"`
	OnBehalfAck     bool   `json:"on_behalf_ack"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type WorklogListRequest struct {
	TaskID   string `json:"task_id"`
	MemberID string `json:"member_id"`
	FromDate string `json:"from_date"`
	ToDate   string `json:"to_date"`
	Status   string `json:"status"`
	Limit    int    `json:"limit"`
	Cursor   string `json:"cursor"`
}

type WorklogSummary struct {
	ActiveCount  int `json:"active_count"`
	VoidedCount  int `json:"voided_count"`
	TotalMinutes int `json:"total_minutes"`
}

type WorklogPage struct {
	ProjectID       string         `json:"project_id"`
	Items           []Worklog      `json:"items"`
	Total           int            `json:"total"`
	Limit           int            `json:"limit"`
	NextCursor      string         `json:"next_cursor"`
	ScopeVersion    int            `json:"scope_version"`
	Summary         WorklogSummary `json:"summary"`
	CanCreate       bool           `json:"can_create"`
	CanManageOthers bool           `json:"can_manage_others"`
	SelfMemberID    string         `json:"self_member_id"`
}

type WorklogRevisionPage struct {
	ProjectID    string    `json:"project_id"`
	WorklogID    string    `json:"worklog_id"`
	Items        []Worklog `json:"items"`
	Total        int       `json:"total"`
	Limit        int       `json:"limit"`
	NextCursor   string    `json:"next_cursor"`
	ScopeVersion int       `json:"scope_version"`
}
