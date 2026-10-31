package platform

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

func desktopRunnerLoopback(r *http.Request) bool {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	peer := net.ParseIP(remote)
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		return false
	}
	local := net.ParseIP(host)
	number, err := strconv.Atoi(port)
	return peer != nil && peer.IsLoopback() && local != nil && local.IsLoopback() && err == nil && number > 0 && number <= 65535
}

func (s *Server) desktopRunnerHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject, path string) bool {
	if path != "desktop-runners" && !strings.HasPrefix(path, "desktop-runners/") {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if scope != "" || !actor.IsBrowser() || !desktopRunnerLoopback(r) {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodPost {
		writeError(w, &APIError{405, "method_not_allowed", "Unsupported request method", nil})
		return true
	}
	if r.URL.RawQuery != "" || path != "desktop-runners/context" && path != "desktop-runners/register" && path != "desktop-runners/renew" {
		writeError(w, desktopRunnerInvalid())
		return true
	}
	var proof desktopRunnerProof
	for name, target := range map[string]*string{"X-Suma-Desktop-Nonce": &proof.nonce, "X-Suma-Desktop-Expires": &proof.expires, "X-Suma-Desktop-Signature": &proof.signature} {
		values := r.Header.Values(name)
		if len(values) != 1 {
			writeError(w, desktopRunnerError(403, "proof_invalid"))
			return true
		}
		*target = values[0]
	}
	// Decode the original bounded body before generic routing can discard
	// duplicate keys; exact canonical string keys reject aliases and nulls.
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	body, err := decode(r)
	if err != nil {
		writeError(w, err)
		return true
	}
	if path == "desktop-runners/renew" {
		var request DesktopRunnerRenewalRequest
		if err = releaseDecode(body, &request); err != nil {
			writeError(w, desktopRunnerInvalid())
			return true
		}
		out, e := s.Service.renewDesktopRunner(r.Context(), actor, request, proof)
		respond(w, http.StatusOK, out, e)
		return true
	}
	names := []string{"project_id", "user_id", "tenant_id"}
	if path == "desktop-runners/register" {
		names = append(names, "worker_id", "adapter_id", "instance_id", "expected_binding_sha256", "base_commit", "credential_sha256", "idempotency_key")
	}
	if len(body) != len(names) {
		writeError(w, desktopRunnerInvalid())
		return true
	}
	for _, name := range names {
		if _, ok := body[name].(string); !ok {
			writeError(w, desktopRunnerInvalid())
			return true
		}
	}
	if path == "desktop-runners/context" {
		var request DesktopRunnerContextRequest
		if err = releaseDecode(body, &request); err != nil {
			writeError(w, desktopRunnerInvalid())
			return true
		}
		out, e := s.Service.desktopRunnerContext(r.Context(), actor, request, proof)
		respond(w, http.StatusOK, out, e)
	} else {
		var request DesktopRunnerRegistrationRequest
		if err = releaseDecode(body, &request); err != nil {
			writeError(w, desktopRunnerInvalid())
			return true
		}
		out, e := s.Service.registerDesktopRunner(r.Context(), actor, request, proof)
		respond(w, http.StatusCreated, out, e)
	}
	return true
}
