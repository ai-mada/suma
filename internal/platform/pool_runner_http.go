package platform

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
)

// Machine requests bypass human Authenticate deliberately. They are never
// converted to a Principal with the pool owner's or grant requester's roles.
func (s *Server) poolRunnerHTTP(w http.ResponseWriter, r *http.Request, path string) bool {
	if !strings.HasPrefix(path, "pool-runner/") {
		return false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") || strings.TrimSpace(token) == "" {
		writeError(w, &APIError{401, "pool_runner_token_required", "资源池执行节点需要独立 Bearer 凭证", nil})
		return true
	}
	if s.poolWorkBuddyCloudHTTP(w, r, token, path, s.WorkBuddy) {
		return true
	}
	if s.poolWorkspaceTerminalHTTP(w, r, token, path) {
		return true
	}
	if s.poolWorkspaceWriteHTTP(w, r, token, path) {
		return true
	}
	if s.poolWorkspaceObservationHTTP(w, r, token, path) {
		return true
	}
	if path == "pool-runner/session" && r.Method == http.MethodGet {
		out, err := s.Service.GetExecutionPoolRunnerSession(r.Context(), token, r.URL.Query().Get("runner_id"))
		respond(w, 200, out, err)
		return true
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "资源池机器更新必须使用 POST", nil})
		return true
	}
	decode := func(out any) error {
		raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
		if err != nil || len(raw) > 2<<20 {
			return invalid("机器请求超过 2 MiB 或不可读")
		}
		if err = RejectDuplicateReleaseJSON(raw); err != nil {
			return err
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err = d.Decode(out); err != nil {
			return invalid("机器请求字段无效")
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return invalid("机器请求包含额外内容")
		}
		return nil
	}
	switch path {
	case "pool-runner/repository-integrations/authorize":
		var req PoolRepositoryAuthorizationRequest
		if err := decode(&req); err != nil {
			writeError(w, err)
			break
		}
		if req.TenantID != req.Assignment.TenantID || req.RunnerID != req.Assignment.PoolRunnerID {
			writeError(w, forbidden())
			break
		}
		out, err := s.Service.AuthorizePoolRepositoryIntegration(r.Context(), token, req.RepositoryAuthorizationRequest)
		respond(w, 200, out, err)
	case "pool-runner/claim":
		var req dag.PoolClaimRequest
		if err := decode(&req); err != nil {
			writeError(w, err)
			break
		}
		var proof *poolMachineProof
		a, err := s.Engine.ClaimPoolWithOperation(r.Context(), req, func(tx *sql.Tx) (dag.PoolClaimAuthority, error) {
			var auth dag.PoolClaimAuthority
			var err error
			proof, auth, err = s.Service.authorizePoolRunnerTx(r.Context(), tx, token, req.RunnerID, req.Scopes)
			if err == nil && (proof.runner.PoolID != req.PoolID || proof.runner.WorkerID != req.WorkerID) {
				return auth, forbidden()
			}
			return auth, err
		}, func(tx *sql.Tx, scope dag.PoolScope, run, node string) (bool, error) {
			if err := s.Service.checkPlanningExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "planning_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkDispatchRecommendationExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkWorkspaceTerminalExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run, false); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkTaskConversationReplyExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkKnowledgeIntelligenceExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run, true); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "knowledge_intelligence_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkIntelligentOperationsExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run, true); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "intelligent_operation_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkFailureDiagnosisExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run, true); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && apiErr.Code == "failure_diagnosis_basis_changed" {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkRepositoryIntegrationExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run); err != nil {
				var apiErr *APIError
				if automationAuthorityFailure(err) || errors.As(err, &apiErr) && strings.HasPrefix(apiErr.Code, "repository_integration_") {
					return false, nil
				}
				return false, err
			}
			if err := s.Service.checkCodeInputExecutionTx(r.Context(), tx, scope.TenantID, scope.ProjectID, run, node, true); err != nil {
				var api *APIError
				if automationAuthorityFailure(err) || errors.As(err, &api) && strings.HasPrefix(api.Code, "candidate_composition_") {
					return false, nil
				}
				return false, err
			}
			return s.Service.PoolToolClaimEligibleTx(r.Context(), tx, proof, scope, run, node)
		})
		if err == nil && a != nil && a.DispatchState == "active" {
			err = s.Service.decoratePoolRepositoryAssignment(r.Context(), token, a)
			if err == nil {
				err = s.Service.decorateCodeInputAssignment(r.Context(), candidateSubject{pool: true, token: token, tenant: a.TenantID, project: a.ProjectID}, a)
			}
		}
		respond(w, 200, PoolRunnerClaimResponse{Assignment: a}, err)
	case "pool-runner/tool-inventory":
		var req PoolToolInventoryRequest
		if err := decode(&req); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.ReportPoolToolInventory(r.Context(), token, req)
		respond(w, 200, out, err)
	case "pool-runner/heartbeat":
		var req PoolRunnerAssignmentRequest
		if err := decode(&req); err != nil {
			writeError(w, err)
			break
		}
		if err := validatePoolEnvelope(req); err != nil {
			writeError(w, err)
			break
		}
		err := s.Engine.HeartbeatWithAuthority(r.Context(), req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment, func(tx *sql.Tx) error {
			_, err := s.Service.PoolRunnerAssignmentAuthorityTx(r.Context(), tx, token, req.Assignment)
			if err != nil {
				return err
			}
			if err := s.Service.checkPlanningExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID); err != nil {
				return err
			}
			if err := s.Service.checkDispatchRecommendationExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID); err != nil {
				return err
			}
			if err := s.Service.checkWorkspaceTerminalExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID, true); err != nil {
				return err
			}
			if err := s.Service.checkTaskConversationReplyExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID); err != nil {
				return err
			}
			if err := s.Service.checkKnowledgeIntelligenceExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID, false); err != nil {
				return err
			}
			if err := s.Service.checkIntelligentOperationsExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID, false); err != nil {
				return err
			}
			if err := s.Service.checkFailureDiagnosisExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID, false); err != nil {
				return err
			}
			if err := s.Service.checkCodeInputExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID, req.Assignment.NodeID, false); err != nil {
				return err
			}
			return s.Service.checkRepositoryIntegrationExecutionTx(r.Context(), tx, req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment.RunID)
		})
		if errors.Is(err, dag.ErrCancelled) {
			writeJSON(w, 200, map[string]bool{"cancel_requested": true})
		} else {
			respond(w, 200, map[string]bool{"cancel_requested": false}, err)
		}
	case "pool-runner/progress":
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
		if readErr != nil || len(raw) > 2<<20 {
			writeError(w, invalid("Invalid Go workspace operation progress"))
			break
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			writeError(w, invalid("机器请求字段无效"))
			break
		}
		if _, host := fields["host_operation"]; host {
			var request PoolHostOperationProgressRequest
			if len(fields) != 6 {
				writeError(w, invalid("Invalid Go workspace operation progress"))
				break
			}
			if err := decode(&request); err != nil {
				writeError(w, err)
				break
			}
			if err := validatePoolEnvelope(request.PoolRunnerAssignmentRequest); err != nil {
				writeError(w, err)
				break
			}
			if request.OperationID != request.HostOperation.EventID {
				writeError(w, invalid("Invalid Go workspace operation progress"))
				break
			}
			err := s.Engine.HostOperationProgressWithAuthority(r.Context(), request.Assignment.TenantID, request.Assignment.ProjectID, request.Assignment, request.OperationID, request.HostOperation, func(tx *sql.Tx) error {
				proof, err := s.Service.PoolRunnerAssignmentAuthorityTx(r.Context(), tx, token, request.Assignment)
				if err != nil {
					return err
				}
				return s.Service.ValidatePoolHostOperationProgressTx(r.Context(), tx, proof, request.Assignment, request.HostOperation)
			})
			respond(w, 200, map[string]bool{"acknowledged": err == nil}, err)
			break
		}
		var req PoolRunnerProgressRequest
		if err := decode(&req); err != nil {
			writeError(w, err)
			break
		}
		if err := validatePoolEnvelope(req.PoolRunnerAssignmentRequest); err != nil {
			writeError(w, err)
			break
		}
		err := s.Engine.ProgressWithAuthority(r.Context(), req.Assignment.TenantID, req.Assignment.ProjectID, req.Assignment, req.OperationID, req.Message, req.ToolEvent, func(tx *sql.Tx) error {
			proof, err := s.Service.PoolRunnerAssignmentAuthorityTx(r.Context(), tx, token, req.Assignment)
			if err != nil {
				return err
			}
			return s.Service.ValidatePoolToolProgressTx(r.Context(), tx, proof, req.Assignment, req.ToolEvent)
		})
		respond(w, 200, map[string]bool{"acknowledged": err == nil}, err)
	case "pool-runner/results":
		var req PoolRunnerResultRequest
		if err := decode(&req); err != nil {
			writeError(w, err)
			break
		}
		if err := validatePoolEnvelope(req.PoolRunnerAssignmentRequest); err != nil {
			writeError(w, err)
			break
		}
		a, result := req.Assignment, req.Result
		if result.RunID != a.RunID || result.NodeID != a.NodeID || result.Attempt != a.Attempt || result.Fence != a.Fence || result.WorkerID != a.WorkerID {
			writeError(w, invalid("结果与固定派发身份不一致"))
			break
		}
		var proof *poolMachineProof
		err := s.Engine.CompleteBound(r.Context(), a.TenantID, a.ProjectID, result, func(tx *sql.Tx) error {
			var e error
			proof, e = s.Service.PoolRunnerAssignmentAuthorityTx(r.Context(), tx, token, a)
			return e
		}, func(tx *sql.Tx, state string) error {
			if result.ArtifactEvidence != nil || result.ReleaseEvidence != nil {
				return &APIError{422, "pool_capability_unsupported", "资源池制品与生产部署需独立授权协议", nil}
			}
			if err := s.Service.ValidatePoolToolResultTx(r.Context(), tx, proof, a, result); err != nil {
				return err
			}
			authorize := func(current *sql.Tx) error {
				_, err := s.Service.PoolRunnerAssignmentAuthorityTx(r.Context(), current, token, a)
				return err
			}
			if err := s.Service.recordPlanningOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordDispatchRecommendationOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordTaskConversationReplyOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordWorkspaceTerminalOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordKnowledgeIntelligenceOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordIntelligentOperationsOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.recordTaskTestPlanReportTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			if err := s.Service.recordFailureDiagnosisOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result, authorize); err != nil {
				return err
			}
			if err := s.Service.validateCandidateCompositionResultTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			if err := s.Service.validateStartupFailureResultTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			if err := s.Service.validateCandidateCompositionRepairResultTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			if err := s.Service.validateWorkspaceTerminalCandidateResultTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			if err := s.Service.validateCodeBaselineResultTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			if err := s.Service.validateRepositoryIntegrationResultTx(r.Context(), tx, a.TenantID, a.ProjectID, result); err != nil {
				return err
			}
			return s.Service.recordRepositoryIntegrationOutcomeTx(r.Context(), tx, a.TenantID, a.ProjectID, result)
		})
		if err == nil {
			// Completion remains committed if projection is temporarily unavailable;
			// the durable failed-event projector retries the same scoped issue.
			if run, readErr := s.Engine.GetRun(r.Context(), a.TenantID, a.ProjectID, a.RunID); readErr == nil {
				if projectionErr := s.Service.FailureIssue(r.Context(), a.TenantID, a.ProjectID, run); projectionErr != nil {
					slog.Error("pool failure issue pending", "run_id", a.RunID, "code", "failure_projection_pending")
				}
			}
		}
		respond(w, 200, map[string]bool{"acknowledged": err == nil}, err)
	default:
		writeError(w, &APIError{422, "pool_capability_unsupported", "该资源池机器协议尚未提供此独立能力", nil})
	}
	return true
}

func validatePoolEnvelope(req PoolRunnerAssignmentRequest) error {
	a := req.Assignment
	if req.RunnerID != a.PoolRunnerID || req.TenantID != a.TenantID || req.ProjectID != a.ProjectID || a.PoolID == "" || a.PoolRunnerID == "" || a.PoolSlotID == "" || a.PoolGrantID == "" || a.PoolGrantVersion < 1 || a.PoolCredentialID == "" || a.PoolCredentialVersion < 1 || a.DispatchID == "" {
		return invalid("机器回写缺少一致的固定租户、项目和派发范围")
	}
	return nil
}
