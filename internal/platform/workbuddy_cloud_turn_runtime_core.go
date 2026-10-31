package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	"github.com/ai-mada/suma/internal/workbuddyacp"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type cloudTurnRuntimeHooks struct {
	Check, CheckWork func(context.Context) (cloudTurnLease, error)
	Begin            func(context.Context) error
	Append, Finish   func(context.Context, cloudTurnRuntimeEvent) error
	Heartbeat        func(context.Context) error
	Take             func(context.Context) ([]cloudTurnRuntimeCommand, error)
	Ack              func(context.Context, cloudTurnRuntimeCommand, string, string) error
	HTTP             *http.Client
	Poll, Beat       time.Duration
}

type cloudTurnWireContext uint8

const (
	cloudTurnNewWork cloudTurnWireContext = iota
	cloudTurnPromptWire
)

type cloudTurnRoundTripper func(*http.Request) (*http.Response, error)

func (f cloudTurnRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cloudTurnPromptResult struct {
	call workbuddyacp.Call
	err  error
}
type cloudTurnNextResult struct {
	event workbuddyacp.Event
	err   error
}
type cloudTurnCommandResult struct {
	command cloudTurnRuntimeCommand
	err     error
	ackErr  error
}
type cloudTurnPermissionRef struct {
	ref        string
	permission wc.CloudTurnPermission
	inFlight   bool
	decided    bool
}

// The hooks are the existing transactional lease/inbox bridge, not another
// scheduler. Only this original permit stack retains the live SDK connection.
func runCloudTurnRuntime(parent context.Context, turnID string, h cloudTurnRuntimeHooks) {
	finish := func(event cloudTurnRuntimeEvent) {
		event.Kind = "terminal"
		if event.Usage == nil {
			event.UsageState = "unreported"
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer cancel()
		_ = h.Finish(ctx, event)
	}
	reject := func(code string) {
		finish(cloudTurnRuntimeEvent{State: "rejected", ErrorCode: code, KnownNotSent: true})
	}
	lease, err := h.CheckWork(parent)
	if err != nil || lease.Client == nil || lease.Remaining <= 0 {
		reject("workbuddy_cloud_turn_authorization_changed")
		return
	}
	ctx, cancel := context.WithTimeout(parent, min(lease.Remaining, 15*time.Minute))
	defer cancel()
	beat := h.Beat
	if beat <= 0 || beat > 10*time.Second {
		beat = 5 * time.Second
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(beat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if h.Heartbeat(ctx) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()
	access, err := lease.Client.GetSessionAccess(ctx, lease.Turn.Source.Collaboration.TaskID)
	if err != nil || !access.Available || access.Task.ID != lease.Turn.Source.Collaboration.TaskID {
		reject("workbuddy_cloud_turn_unavailable")
		return
	}
	if _, err = h.Check(ctx); err != nil {
		reject("workbuddy_cloud_turn_authorization_changed")
		return
	}
	promptWire := make(chan struct{})
	var promptOnce sync.Once
	client := http.Client{}
	if h.HTTP != nil {
		client = *h.HTTP
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.Transport = cloudTurnRoundTripper(func(request *http.Request) (*http.Response, error) {
		check := h.Check
		if request.Context().Value(cloudTurnNewWork) == true {
			check = h.CheckWork
		}
		if _, checkErr := check(request.Context()); checkErr != nil {
			return nil, checkErr
		}
		if request.Context().Value(cloudTurnPromptWire) == true {
			promptOnce.Do(func() { close(promptWire) })
		}
		return base.RoundTrip(request)
	})
	session, err := workbuddyacp.Attach(ctx, access, &client, workbuddyacp.Setup{InitializeID: newID(), LoadID: newID(), OnHistory: func(workbuddyacp.Packet) error {
		// History belongs to the original task. Never duplicate it as this new
		// reply, or interpret a replayed user chunk as a new input instruction.
		_, checkErr := h.Check(ctx)
		return checkErr
	}})
	if err != nil {
		reject("workbuddy_cloud_turn_unavailable")
		return
	}
	defer session.Close()
	// A cancel arriving during attachment prevents a new Prompt. There is no
	// locally-owned active RPC to cancel, so no remote-stop ACK is fabricated.
	preCommands, err := h.Take(ctx)
	if err != nil {
		reject("workbuddy_cloud_turn_authorization_changed")
		return
	}
	if len(preCommands) != 0 {
		for _, command := range preCommands {
			_ = h.Ack(ctx, command, "rejected", "workbuddy_cloud_turn_rejected")
		}
		reject("workbuddy_cloud_turn_interrupted")
		return
	}
	if err = h.Begin(ctx); err != nil {
		reject("workbuddy_cloud_turn_authorization_changed")
		return
	}
	promptDone := make(chan cloudTurnPromptResult, 1)
	promptExited := make(chan struct{})
	go func() {
		defer close(promptExited)
		wireCtx := context.WithValue(context.WithValue(ctx, cloudTurnNewWork, true), cloudTurnPromptWire, true)
		call, sendErr := session.Prompt(wireCtx, turnID, lease.Prompt)
		promptDone <- cloudTurnPromptResult{call, sendErr}
	}()
	defer func() { cancel(); session.Close(); <-promptExited }()
	var first *cloudTurnPromptResult
	select {
	case <-promptWire:
	case result := <-promptDone:
		first = &result
	case <-ctx.Done():
		finish(cloudTurnRuntimeEvent{State: "unknown", ErrorCode: "workbuddy_cloud_turn_interrupted"})
		return
	}
	if first != nil && first.err != nil {
		state := "unknown"
		known := !cloudTurnSDKUnknown(first.err)
		if known {
			state = "rejected"
		}
		finish(cloudTurnRuntimeEvent{State: state, ErrorCode: "workbuddy_cloud_turn_unknown", KnownNotSent: known})
		return
	}
	// Start consuming while the Prompt HTTP request is still in flight. Some
	// providers wait for a permission answer before acknowledging that request.
	next := make(chan cloudTurnNextResult, 1)
	var nextRead <-chan cloudTurnNextResult = next
	nextExited := make(chan struct{})
	go func() {
		defer close(nextExited)
		for {
			event, nextErr := session.Next(ctx)
			select {
			case next <- cloudTurnNextResult{event, nextErr}:
			case <-ctx.Done():
				return
			}
			if nextErr != nil {
				return
			}
		}
	}()
	defer func() { cancel(); session.Close(); <-nextExited }()
	secrets := append(append([]string{}, lease.Secrets...), access.Ticket)
	filter := newCloudTurnTextFilter(secrets)
	ledger := newCloudTurnToolLedger()
	permissions := map[string]*cloudTurnPermissionRef{}
	commands := make(chan cloudTurnCommandResult, wc.CloudTurnMaxPermissions+1)
	var commandWG sync.WaitGroup
	defer func() { cancel(); commandWG.Wait() }()
	inFlight, commandUnknown, incomplete := 0, false, false
	promptAck, promptObserved, terminal := false, false, false
	ending := cloudTurnRuntimeEvent{State: "unknown", UsageState: "unreported"}
	appendEvent := func(event cloudTurnRuntimeEvent) error { return h.Append(ctx, event) }
	processPrompt := func(result cloudTurnPromptResult) error {
		promptObserved = true
		if result.err != nil || result.call.State != "sent" {
			return errors.New("workbuddy_cloud_turn_unknown")
		}
		promptAck = true
		return appendEvent(cloudTurnRuntimeEvent{Kind: "prompt_sent"})
	}
	if first != nil {
		if err = processPrompt(*first); err != nil {
			finish(cloudTurnRuntimeEvent{State: "unknown", ErrorCode: "workbuddy_cloud_turn_unknown"})
			return
		}
	}
	poll := h.Poll
	if poll <= 0 || poll > time.Second {
		poll = 200 * time.Millisecond
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if terminal && promptObserved && inFlight == 0 {
			text, _ := filter.Flush()
			if text != "" {
				entry := wc.CloudTurnEntry{Kind: "assistant_text", Text: text}
				if err = appendEvent(cloudTurnRuntimeEvent{Kind: "entry", Entry: &entry}); err != nil {
					ending.State, ending.ErrorCode = "unknown", cloudTurnAppendCode(err)
					finish(ending)
					return
				}
			}
			pending := false
			for _, permission := range permissions {
				pending = pending || !permission.decided
			}
			if promptAck && ledger.Complete() && !pending && !commandUnknown && !incomplete && ending.ErrorCode == "" {
				ending.State = "completed"
			} else {
				ending.State = "unknown"
				if ending.ErrorCode == "" {
					ending.ErrorCode = "workbuddy_cloud_turn_unknown"
				}
			}
			finish(ending)
			return
		}
		select {
		case <-ctx.Done():
			ending.State, ending.ErrorCode = "unknown", "workbuddy_cloud_turn_interrupted"
			finish(ending)
			return
		case result := <-promptDone:
			if err = processPrompt(result); err != nil {
				ending.State, ending.ErrorCode = "unknown", "workbuddy_cloud_turn_unknown"
				finish(ending)
				return
			}
		case received := <-nextRead:
			if received.err != nil {
				if terminal {
					nextRead = nil
					continue
				}
				ending.State, ending.ErrorCode = "unknown", "workbuddy_cloud_turn_interrupted"
				finish(ending)
				return
			}
			if terminal {
				continue // A later observation cannot heal this already-ended turn.
			}
			if _, err = h.Check(ctx); err != nil {
				ending.State, ending.ErrorCode = "unknown", "workbuddy_cloud_turn_authorization_changed"
				finish(ending)
				return
			}
			event := received.event
			switch event.Kind {
			case "prompt_response":
				if event.CallID != turnID || event.PromptResponse == nil {
					incomplete = true
					continue
				}
				terminal = true
				ending.StopReason = string(event.PromptResponse.StopReason)
				ending.Usage, ending.UsageState = cloudTurnProjectUsage(event)
				for _, permission := range permissions {
					if !permission.decided && !permission.inFlight {
						incomplete = true
					}
				}
			case "prompt_error":
				if event.CallID == turnID {
					terminal, incomplete = true, true
					ending.ErrorCode = "workbuddy_cloud_turn_rejected"
				}
			case "permission_request":
				permission, projectErr := cloudTurnProjectPermission(event.Packet, newID(), turnID, secrets)
				if projectErr != nil || event.PermissionRef == "" {
					ending.ErrorCode = "workbuddy_cloud_turn_unavailable"
					finish(ending)
					return
				}
				if err = appendEvent(cloudTurnRuntimeEvent{Kind: "permission", Permission: &permission}); err != nil {
					ending.ErrorCode = cloudTurnAppendCode(err)
					finish(ending)
					return
				}
				permissions[permission.ID] = &cloudTurnPermissionRef{ref: event.PermissionRef, permission: permission}
				ledger.Permission(permission.ToolCallID)
			default:
				projected, projectErr := cloudTurnProjectUpdate(event.Packet, filter, ledger, secrets)
				if projectErr != nil {
					ending.ErrorCode = "workbuddy_cloud_turn_unavailable"
					finish(ending)
					return
				}
				incomplete = incomplete || projected.Incomplete
				if projected.ToolID != "" {
					if err = appendEvent(cloudTurnRuntimeEvent{Kind: "tool", ToolCallID: projected.ToolID, ToolState: projected.ToolState, ToolStart: projected.ToolStart}); err != nil {
						ending.ErrorCode = cloudTurnAppendCode(err)
						finish(ending)
						return
					}
				}
				for _, entry := range projected.Entries {
					if err = appendEvent(cloudTurnRuntimeEvent{Kind: "entry", Entry: &entry}); err != nil {
						ending.ErrorCode = cloudTurnAppendCode(err)
						finish(ending)
						return
					}
				}
			}
		case <-ticker.C:
			if terminal {
				continue
			}
			pending, takeErr := h.Take(ctx)
			if takeErr != nil {
				ending.ErrorCode = "workbuddy_cloud_turn_authorization_changed"
				finish(ending)
				return
			}
			for _, command := range pending {
				var call func() error
				if command.Command.TurnID != turnID {
					if h.Ack(ctx, command, "rejected", "workbuddy_cloud_turn_rejected") != nil {
						commandUnknown = true
					}
					continue
				}
				switch command.Command.Kind {
				case "permission":
					permission := permissions[command.Command.PermissionID]
					kind := ""
					if permission != nil && !permission.decided && !permission.inFlight && command.Permission.ID == command.Command.PermissionID && command.Permission.TurnID == turnID && command.Permission.ToolCallID == permission.permission.ToolCallID && command.Permission.DetailsSHA256 == permission.permission.DetailsSHA256 {
						for _, option := range permission.permission.Options {
							if option.ID == command.Command.OptionID {
								kind = option.Kind
								break
							}
						}
					}
					if kind != "reject_once" && (kind != "allow_once" || !permission.permission.ApproveAllowed || !command.Permission.ApproveAllowed) {
						if h.Ack(ctx, command, "rejected", "workbuddy_cloud_turn_rejected") != nil {
							commandUnknown = true
						}
						continue
					}
					check := h.Check
					if kind == "allow_once" {
						check = h.CheckWork
					}
					if _, checkErr := check(ctx); checkErr != nil {
						if h.Ack(ctx, command, "rejected", "workbuddy_cloud_turn_authorization_changed") != nil {
							commandUnknown = true
						}
						continue
					}
					permission.inFlight = true
					ref, option, work := permission.ref, command.Command.OptionID, kind == "allow_once"
					call = func() error {
						return session.ReplyPermission(context.WithValue(ctx, cloudTurnNewWork, work), ref, &option)
					}
				case "cancel":
					if _, checkErr := h.Check(ctx); checkErr == nil {
						// Restrict the DB snapshot to refs already observed by this
						// runtime before invoking the SDK. A newly arriving request
						// must never inherit this earlier cancellation's ACK.
						scope := make([]cloudTurnPermissionVersion, 0, len(command.cancelPermissions))
						for _, version := range command.cancelPermissions {
							if permission := permissions[version.ID]; permission != nil && !permission.decided && !permission.inFlight {
								scope = append(scope, version)
							}
						}
						command.cancelPermissions = scope
						call = func() error { return session.Cancel(context.WithValue(ctx, cloudTurnNewWork, false)) }
					}
				}
				if call == nil {
					if h.Ack(ctx, command, "rejected", "workbuddy_cloud_turn_rejected") != nil {
						commandUnknown = true
					}
					continue
				}
				inFlight++
				commandWG.Add(1)
				go func(command cloudTurnRuntimeCommand, call func() error) {
					defer commandWG.Done()
					result := cloudTurnCommandResult{command: command, err: call()}
					// The original command stack records its fact even if the
					// consumer exits or loses authorization in the meantime.
					ackCtx, done := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
					state, code := cloudTurnCommandOutcome(result.err)
					result.ackErr = h.Ack(ackCtx, command, state, code)
					done()
					select {
					case commands <- result:
					case <-ctx.Done():
					}
				}(command, call)
			}
		case result := <-commands:
			inFlight--
			state, _ := cloudTurnCommandOutcome(result.err)
			if result.ackErr != nil || state == "unknown" {
				commandUnknown = true
			}
			if permission := permissions[result.command.Command.PermissionID]; permission != nil {
				permission.inFlight = false
				permission.decided = state == "acknowledged"
			}
			if result.command.Command.Kind == "cancel" && state == "acknowledged" {
				for _, version := range result.command.cancelPermissions {
					if permission := permissions[version.ID]; permission != nil && !permission.inFlight {
						permission.decided = true
					}
				}
			}
		}
	}
}

func cloudTurnSDKUnknown(err error) bool {
	var failure *workbuddyacp.Error
	return errors.As(err, &failure) && failure.EffectUnknown
}

func cloudTurnCommandOutcome(err error) (string, string) {
	if err == nil {
		return "acknowledged", ""
	}
	if cloudTurnSDKUnknown(err) {
		return "unknown", "workbuddy_cloud_turn_unknown"
	}
	return "rejected", "workbuddy_cloud_turn_rejected"
}

func cloudTurnAppendCode(err error) string {
	var api *APIError
	if errors.As(err, &api) && api.Code == "workbuddy_cloud_turn_limited" {
		return api.Code
	}
	return "workbuddy_cloud_turn_authorization_changed"
}

func cloudTurnProjectUsage(event workbuddyacp.Event) (*tc.Usage, string) {
	if event.PromptResponse == nil || event.PromptResponse.Usage == nil {
		return nil, "unreported"
	}
	data, err := json.Marshal(event.PromptResponse.Usage)
	if err != nil {
		return nil, "unreported"
	}
	var counts struct {
		Input, Output, CachedRead, CachedWrite, Thought *int64
	}
	var raw map[string]*int64
	if json.Unmarshal(data, &raw) != nil {
		return nil, "unreported"
	}
	counts.Input, counts.Output, counts.CachedRead, counts.CachedWrite, counts.Thought = raw["inputTokens"], raw["outputTokens"], raw["cachedReadTokens"], raw["cachedWriteTokens"], raw["thoughtTokens"]
	for _, value := range []*int64{counts.Input, counts.Output, counts.CachedRead, counts.CachedWrite, counts.Thought} {
		if value != nil && (*value < 0 || *value > tc.MaxSafeUsage) {
			return nil, "unreported"
		}
	}
	if counts.Input == nil || counts.Output == nil {
		return nil, "unreported"
	}
	return &tc.Usage{InputTokens: counts.Input, OutputTokens: counts.Output, CachedInputTokens: counts.CachedRead, CacheWriteInputTokens: counts.CachedWrite, ReasoningOutputTokens: counts.Thought}, "partial"
}
