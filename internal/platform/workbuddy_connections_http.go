package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

// Callback is the private server-side adapter for an OAuth response. It never
// accepts project, owner, endpoint, or operation identity from the provider.
// The exact initiating browser credential is still checked by the command.
func (c *WorkBuddyConnections) Callback(ctx context.Context, actor Actor, state, code, providerError string) (WorkBuddyCallbackResult, error) {
	if c == nil || c.s == nil || !actor.IsBrowser() || !workBuddyCommandOpaque(state, 256) || (providerError != "" && providerError != "access_denied") || (providerError == "" && !workBuddyCommandOpaque(code, 8192)) || (providerError != "" && code != "") {
		return WorkBuddyCallbackResult{}, invalid(workBuddyInvalid)
	}
	id, mac, found := strings.Cut(state, ".")
	if !found || !handoffID(id, false) || len(mac) != 43 {
		return WorkBuddyCallbackResult{}, invalid(workBuddyInvalid)
	}
	// This bounded lookup selects only scope identifiers for the current owner.
	// No lookup result or credential is returned before the command's current
	// authority transaction validates the nonce and original browser reference.
	lookup, cancel := context.WithTimeout(ctx, authorityTransactionBudget)
	var project string
	err := c.s.DB.QueryRowContext(lookup, c.s.q(`SELECT project_id FROM suma_workbuddy_oauth_operations WHERE id=? AND tenant_id=? AND owner_id=? AND kind='authorization'`), id, actor.credential.tenantID, actor.credential.subjectID).Scan(&project)
	cancel()
	if err == sql.ErrNoRows {
		return WorkBuddyCallbackResult{}, missing()
	}
	if err != nil {
		return WorkBuddyCallbackResult{}, err
	}
	key := "workbuddy-callback:" + id
	var connection WorkBuddyConnection
	if providerError == "access_denied" {
		connection, err = c.CancelAuthorization(ctx, actor, project, id, WorkBuddyAuthorizationCancel{ProjectID: project, ExpectedVersion: 1, State: state, IdempotencyKey: key})
	} else {
		connection, err = c.CompleteAuthorization(ctx, actor, project, id, WorkBuddyAuthorizationComplete{ProjectID: project, ExpectedVersion: 1, State: state, Code: code, IdempotencyKey: key})
	}
	if err != nil {
		return WorkBuddyCallbackResult{}, err
	}
	redirect := "/?" + url.Values{"project": {project}, "view": {"settings"}, "sub": {"tools"}, "panel": {"workbuddy:" + connection.ID}}.Encode()
	return WorkBuddyCallbackResult{ProjectID: project, ConnectionID: connection.ID, State: connection.State, RedirectURL: redirect}, nil
}

// The initial cross-site redirect cannot receive a SameSite=Strict cookie.
// GET only serves an isolated, no-referrer bridge: no authority or grant is
// consumed. It clears the query before any same-origin request. The subsequent
// POST uses the ordinary cookie, original-view, origin, and command guards.
func workBuddyOAuthCallbackLanding(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workbuddy-oauth/callback" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// Even malformed OAuth responses never enter a token exchange on GET.
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		writeError(w, invalid(workBuddyUnavailable))
		return true
	}
	value := base64.RawStdEncoding.EncodeToString(nonce[:])
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+value+"'; style-src 'nonce-"+value+"'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	language := w.Header().Get("Content-Language")
	if language == "" {
		language = r.Header.Get("Accept-Language")
	}
	lang, status, back := "zh-CN", "正在完成授权…", "返回工作台"
	if strings.HasPrefix(strings.ToLower(language), "en") {
		lang, status, back = "en-US", "Completing authorization…", "Return to workbench"
	}
	html := strings.NewReplacer("NONCE_VALUE", value, "HTML_LANG", lang, "STATUS_TEXT", status, "RETURN_TEXT", back).Replace(workBuddyCallbackHTML)
	_, _ = io.WriteString(w, html)
	return true
}

const workBuddyCallbackHTML = `<!doctype html><html lang="HTML_LANG"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>WorkBuddy · suma</title><style nonce="NONCE_VALUE">body{font:16px/1.6 system-ui,sans-serif;background:#f7f7fb;color:#20202c;margin:0;padding:24px}main{max-width:560px;margin:12vh auto;padding:24px;background:white;border:1px solid #dedee8;border-radius:8px}h1{font-size:24px}a{display:inline-flex;align-items:center;min-height:44px;color:#6853c4}p{overflow-wrap:anywhere}@media(prefers-color-scheme:dark){body{background:#171922;color:#f1f0f8}main{background:#21242f;border-color:#414454}a{color:#c1b4ff}}</style><main><h1>WorkBuddy · suma</h1><p id="status" role="status">STATUS_TEXT</p><a id="return" href="/">RETURN_TEXT</a></main><script nonce="NONCE_VALUE">(async()=>{
const search=location.search,query=new URLSearchParams(search),state=query.get('state')||'',code=query.get('code')||'',error=query.get('error')||'';
history.replaceState(null,'',location.pathname);
const status=document.getElementById('status'),back=document.getElementById('return');let english=document.documentElement.lang.startsWith('en');
const localize=language=>{if(typeof language==='string'&&/^(en|zh)(-|$)/i.test(language)){english=language.toLowerCase().startsWith('en');document.documentElement.lang=english?'en-US':'zh-CN';}status.textContent=english?'Completing authorization…':'正在完成授权…';back.textContent=english?'Return to workbench':'返回工作台';};
const fail=()=>{status.textContent=english?'Authorization was not completed. Return to the workbench and inspect the connection before starting a new authorization.':'授权未完成。请返回工作台核对连接状态，再决定是否重新授权。';};
try{
if(search.length>32768||[...query.keys()].some(key=>!['state','code','error','error_description'].includes(key)||query.getAll(key).length!==1)||!state||(!code&&!error)||(code&&error)){fail();return;}
const sessionResponse=await fetch('/api/v1/auth/session',{credentials:'same-origin',cache:'no-store'});
localize(sessionResponse.headers.get('Content-Language')||navigator.language);
if(!sessionResponse.ok){fail();return;}
const session=await sessionResponse.json();
if(typeof session.user?.id!=='string'||typeof session.organization?.id!=='string'){fail();return;}
const payload=error?{state,error}:{state,code};
const response=await fetch('/api/v1/workbuddy-oauth/callback',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json','X-Suma-Expected-User':session.user.id,'X-Suma-Expected-Organization':session.organization.id},body:JSON.stringify(payload)});
if(!response.ok){fail();return;}
const result=await response.json();
if(typeof result.redirect_url!=='string'||!result.redirect_url.startsWith('/?')){fail();return;}
location.replace(result.redirect_url);
}catch{fail();}
})();</script></html>`

func (s *Server) workBuddyConnectionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string, c *WorkBuddyConnections) bool {
	if len(parts) == 0 || (parts[0] != "workbuddy-connections" && parts[0] != "workbuddy-authorizations" && parts[0] != "workbuddy-oauth") {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !actor.IsBrowser() || scope != "" {
		writeError(w, forbidden())
		return true
	}
	if c == nil {
		writeError(w, &APIError{http.StatusServiceUnavailable, workBuddyUnavailable, workBuddyUnavailable, nil})
		return true
	}
	bad := func() { writeError(w, invalid(workBuddyInvalid)) }
	if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" || r.Body == nil {
			bad()
			return true
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
		if e != nil || len(raw) > 64<<10 {
			bad()
			return true
		}
		switch {
		case len(parts) == 1 && parts[0] == "workbuddy-authorizations":
			var req WorkBuddyAuthorizationRequest
			if woc.Decode(raw, &req) != nil {
				bad()
				return true
			}
			out, e := c.StartAuthorization(r.Context(), actor, req.ProjectID, req)
			respond(w, http.StatusCreated, out, e)
		case len(parts) == 2 && parts[0] == "workbuddy-oauth" && parts[1] == "callback":
			var req WorkBuddyCallbackRequest
			var fields map[string]json.RawMessage
			if woc.Decode(raw, &req) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 2 {
				bad()
				return true
			}
			out, e := c.Callback(r.Context(), actor, req.State, req.Code, req.Error)
			respond(w, http.StatusOK, out, e)
		case len(parts) == 3 && parts[0] == "workbuddy-connections" && parts[2] == "refresh":
			var req WorkBuddyRefreshRequest
			if woc.Decode(raw, &req) != nil {
				bad()
				return true
			}
			out, e := c.Refresh(r.Context(), actor, req.ProjectID, parts[1], req)
			respond(w, http.StatusOK, out, e)
		case len(parts) == 3 && parts[0] == "workbuddy-connections" && parts[2] == "disconnect":
			var req WorkBuddyDisconnectRequest
			if woc.Decode(raw, &req) != nil {
				bad()
				return true
			}
			out, e := c.Disconnect(r.Context(), actor, req.ProjectID, parts[1], req)
			respond(w, http.StatusOK, out, e)
		default:
			writeError(w, missing())
		}
		return true
	}
	if r.Method != http.MethodGet || (parts[0] != "workbuddy-connections" && parts[0] != "workbuddy-authorizations") || len(parts) > 2 || (parts[0] == "workbuddy-authorizations" && len(parts) != 2) {
		writeError(w, missing())
		return true
	}
	query, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	for key, values := range query {
		if (key != "project_id" && !(len(parts) == 1 && (key == "limit" || key == "cursor"))) || len(values) != 1 || values[0] == "" {
			bad()
			return true
		}
	}
	project := query.Get("project_id")
	if !handoffID(project, false) {
		bad()
		return true
	}
	if len(parts) == 2 {
		if parts[0] == "workbuddy-authorizations" {
			out, e := c.Authorization(r.Context(), actor, project, parts[1])
			respond(w, http.StatusOK, out, e)
		} else {
			out, e := c.Get(r.Context(), actor, project, parts[1])
			respond(w, http.StatusOK, out, e)
		}
		return true
	}
	limit := 20
	if value := query.Get("limit"); value != "" {
		limit, e = strconv.Atoi(value)
		if e != nil || strconv.Itoa(limit) != value || limit < 1 || limit > 50 {
			bad()
			return true
		}
	}
	out, e := c.List(r.Context(), actor, project, limit, query.Get("cursor"))
	respond(w, http.StatusOK, out, e)
	return true
}
