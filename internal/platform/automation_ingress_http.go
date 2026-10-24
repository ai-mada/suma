package platform

import (
	"errors"
	"io"
	"mime"
	"net/http"
)

var automationIngressSlots = make(chan struct{}, 32)

func (s *Server) automationIngressHTTP(w http.ResponseWriter, r *http.Request, p Principal, scope, path string, parts []string, project string, m map[string]any) bool {
	collection := len(parts) == 3 && parts[0] == "automations" && parts[2] == "ingresses"
	if !collection && parts[0] != "automation-ingresses" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	ctx := r.Context()
	switch {
	case collection && r.Method == "POST":
		var req AutomationIngressCreateRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateAutomationIngress(ctx, p, project, parts[1], req)
		respond(w, 201, out, err)
	case collection && r.Method == "GET":
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ListAutomationIngresses(ctx, p, project, parts[1], limit, cursor)
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == "GET":
		out, err := s.Service.GetAutomationIngress(ctx, p, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == "POST":
		var req AutomationIngressCommandRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandAutomationIngress(ctx, p, project, parts[1], req)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "revisions" && r.Method == "GET":
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.AutomationIngressRevisions(ctx, p, project, parts[1], limit, cursor)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "deliveries" && r.Method == "GET":
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.AutomationIngressDeliveries(ctx, p, project, parts[1], limit, cursor)
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[2] == "deliveries" && r.Method == "GET":
		out, err := s.Service.GetAutomationIngressDelivery(ctx, p, project, parts[1], parts[3])
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
func ingressSignatureError() error {
	return &APIError{401, "invalid_signature", "事件签名无效或凭据不可用", nil}
}
func (s *Server) automationIngressPublicHTTP(w http.ResponseWriter, r *http.Request, path string, parts []string) bool {
	if parts[0] != "hooks" {
		return false
	}
	if r.Method != "POST" || len(parts) != 2 || !ingressLowerHex(parts[1], 32) {
		writeError(w, missing())
		return true
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || len(r.Header.Values("Cookie")) > 0 || len(r.Header.Values("Authorization")) > 0 || len(r.Header.Values("Origin")) > 0 || len(r.Header.Values("Content-Encoding")) > 0 {
		writeError(w, invalid("公共事件入口不接受会话、查询、来源或编码头"))
		return true
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		writeError(w, invalid("事件必须使用 JSON content-type"))
		return true
	}
	for _, header := range []string{"X-Hub-Signature-256", "X-GitHub-Delivery", "X-GitHub-Event", "X-Suma-Delivery", "X-Suma-Timestamp", "X-Suma-Signature-256"} {
		if len(r.Header.Values(header)) > 1 {
			writeError(w, ingressSignatureError())
			return true
		}
	}
	select {
	case automationIngressSlots <- struct{}{}:
		defer func() { <-automationIngressSlots }()
	default:
		writeError(w, &APIError{429, "ingress_busy", "事件接收繁忙，请稍后重试", nil})
		return true
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	var tooLarge *http.MaxBytesError
	if len(raw) > 2<<20 || errors.As(err, &tooLarge) {
		writeError(w, &APIError{413, "body_too_large", "事件正文超过 2 MiB", nil})
		return true
	}
	if err != nil {
		writeError(w, invalid("事件正文不可读"))
		return true
	}
	out, err := s.Service.ReceiveAutomationIngress(r.Context(), parts[1], r.Header, raw)
	status := 202
	if out.Duplicate {
		status = 200
	}
	respond(w, status, out, err)
	return true
}
