package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func decodeRunnerPairing(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	raw, err := io.ReadAll(r.Body)
	if err != nil || RejectDuplicateReleaseJSON(raw) != nil {
		return runnerPairingError(400, "invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return runnerPairingError(400, "invalid")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return runnerPairingError(400, "invalid")
	}
	return nil
}

func (s *Server) runnerPairingMachineHTTP(w http.ResponseWriter, r *http.Request, parts []string) bool {
	if len(parts) != 3 || parts[0] != "runner-pairings" || (parts[2] != "proposal" && parts[2] != "status") {
		return false
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, runnerPairingError(400, "invalid"))
		return true
	}
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		writeError(w, unauthenticated())
		return true
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if parts[2] == "proposal" {
		var request RunnerPairingProposalRequest
		if err := decodeRunnerPairing(w, r, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ProposeRunnerPairing(r.Context(), parts[1], request, token, s.PublicURL)
		respond(w, http.StatusOK, out, err)
	} else {
		var request RunnerPairingStatusRequest
		if err := decodeRunnerPairing(w, r, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.RunnerPairingStatus(r.Context(), parts[1], request, token, s.PublicURL)
		respond(w, http.StatusOK, out, err)
	}
	return true
}

func (s *Server) runnerPairingBrowserHTTP(w http.ResponseWriter, r *http.Request, actor Actor, parts []string) bool {
	if len(parts) == 0 || parts[0] != "runner-pairings" {
		return false
	}
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return true
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.URL.RawQuery) > 8192 || len(query["project_id"]) != 1 {
			writeError(w, runnerPairingError(400, "invalid"))
			return true
		}
		for key, values := range query {
			if len(values) != 1 || (key != "project_id" && key != "status" && key != "q" && key != "limit" && key != "cursor") {
				writeError(w, runnerPairingError(400, "invalid"))
				return true
			}
		}
		out, err := s.Service.listRunnerPairings(r.Context(), actor, query.Get("project_id"), runnerPairingInboxQuery{status: query.Get("status"), q: query.Get("q"), limit: query.Get("limit"), cursor: query.Get("cursor")}, s.PublicURL)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if len(parts) == 1 && r.Method == http.MethodPost && r.URL.RawQuery == "" {
		var request RunnerPairingCreateRequest
		if err := decodeRunnerPairing(w, r, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateRunnerPairing(r.Context(), actor, request, s.PublicURL)
		respond(w, http.StatusCreated, out, err)
		return true
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.URL.RawQuery) > 4096 || len(query) != 1 || len(query["project_id"]) != 1 || !handoffID(query.Get("project_id"), false) {
			writeError(w, runnerPairingError(400, "invalid"))
			return true
		}
		out, err := s.Service.GetRunnerPairing(r.Context(), actor, query.Get("project_id"), parts[1], s.PublicURL)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost && r.URL.RawQuery == "" {
		var request RunnerPairingCommandRequest
		if err := decodeRunnerPairing(w, r, &request); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandRunnerPairing(r.Context(), actor, parts[1], request, s.PublicURL)
		respond(w, http.StatusOK, out, err)
		return true
	}
	writeError(w, runnerPairingError(400, "invalid"))
	return true
}
