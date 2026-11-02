package platform

import (
	"github.com/ai-mada/suma/internal/strictjson"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func decodePoolPairing(w http.ResponseWriter, r *http.Request, out any) error {
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if e != nil || strictjson.Decode(raw, out, 4096) != nil {
		return poolPairingError(400, "invalid")
	}
	return nil
}
func (s *Server) poolRunnerPairingMachineHTTP(w http.ResponseWriter, r *http.Request, parts []string) bool {
	if len(parts) == 0 || parts[0] != "pool-runner-pairings" {
		return false
	}
	if len(parts) != 3 || (parts[2] != "proposal" && parts[2] != "status") || r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeError(w, poolPairingError(400, "invalid"))
		return true
	}
	headers := r.Header.Values("Authorization")
	if len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") {
		writeError(w, unauthenticated())
		return true
	}
	token := strings.TrimPrefix(headers[0], "Bearer ")
	if parts[2] == "proposal" {
		var req PoolRunnerPairingProposalRequest
		if e := decodePoolPairing(w, r, &req); e != nil {
			writeError(w, e)
			return true
		}
		out, e := s.Service.ProposePoolRunnerPairing(r.Context(), parts[1], req, token, s.PublicURL)
		respond(w, 200, out, e)
	} else {
		var req PoolRunnerPairingStatusRequest
		if e := decodePoolPairing(w, r, &req); e != nil {
			writeError(w, e)
			return true
		}
		out, e := s.Service.PoolRunnerPairingStatus(r.Context(), parts[1], req, token, s.PublicURL)
		respond(w, 200, out, e)
	}
	return true
}
func (s *Server) poolRunnerPairingBrowserHTTP(w http.ResponseWriter, r *http.Request, actor Actor, parts []string) bool {
	if len(parts) < 3 || parts[0] != "execution-pools" || parts[2] != "runner-pairings" {
		return false
	}
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return true
	}
	pool := parts[1]
	w.Header().Set("Cache-Control", "no-store")
	if len(parts) == 3 && r.Method == http.MethodGet {
		out, e := s.Service.listPoolRunnerPairings(r.Context(), actor, pool, func() (poolPairingListQuery, error) {
			values, e := url.ParseQuery(r.URL.RawQuery)
			if e != nil || len(r.URL.RawQuery) > 8192 {
				return poolPairingListQuery{}, poolPairingError(400, "invalid")
			}
			for key, items := range values {
				if len(items) != 1 || (key != "status" && key != "q" && key != "limit" && key != "cursor") || (key != "q" && items[0] == "") {
					return poolPairingListQuery{}, poolPairingError(400, "invalid")
				}
			}
			return poolPairingListQuery{status: values.Get("status"), q: values.Get("q"), limit: values.Get("limit"), cursor: values.Get("cursor")}, nil
		}, s.PublicURL)
		respond(w, 200, out, e)
		return true
	}
	if r.URL.RawQuery != "" {
		writeError(w, poolPairingError(400, "invalid"))
		return true
	}
	if len(parts) == 3 && r.Method == http.MethodPost {
		var req PoolRunnerPairingCreateRequest
		if e := decodePoolPairing(w, r, &req); e != nil {
			writeError(w, e)
			return true
		}
		if req.PoolID != pool {
			writeError(w, poolPairingError(400, "invalid"))
			return true
		}
		out, e := s.Service.CreatePoolRunnerPairing(r.Context(), actor, req, s.PublicURL)
		respond(w, 201, out, e)
		return true
	}
	if len(parts) == 4 && r.Method == http.MethodGet {
		out, e := s.Service.GetPoolRunnerPairing(r.Context(), actor, pool, parts[3], s.PublicURL)
		respond(w, 200, out, e)
		return true
	}
	if len(parts) == 5 && parts[4] == "commands" && r.Method == http.MethodPost {
		var req PoolRunnerPairingCommandRequest
		if e := decodePoolPairing(w, r, &req); e != nil {
			writeError(w, e)
			return true
		}
		if req.PoolID != pool {
			writeError(w, poolPairingError(400, "invalid"))
			return true
		}
		out, e := s.Service.CommandPoolRunnerPairing(r.Context(), actor, parts[3], req, s.PublicURL)
		respond(w, 200, out, e)
		return true
	}
	writeError(w, poolPairingError(400, "invalid"))
	return true
}
