package platform

import (
	"errors"
	"net/http"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/executioncontract"
)

func respondRunnerResult(w http.ResponseWriter, err error) {
	if errors.Is(err, dag.ErrRunnerResultClosed) {
		err = &diagnosticAPIError{
			cause: &APIError{http.StatusConflict, executioncontract.RunnerResultClosedCode, "DAG 派发已失效", nil},
			key:   "dag.error.stale_assignment",
		}
	}
	respond(w, http.StatusOK, map[string]bool{"acknowledged": err == nil}, err)
}
