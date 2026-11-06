package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/toolcontract"
)

const (
	maxRunWatchCursor      = 9007199254740991
	maxRunWatchEvents      = 100000
	maxRunWatchFailures    = 5
	runWatchUsage          = "run watch requires RUN, with optional --after 0..9007199254740991 and --max-events 1..100000"
	runSchedulingUsage     = "run scheduling requires RUN and --node NODE"
	runQueueUsage          = "run queue accepts --route project|pool, --run RUN, --tool PROFILE, --limit 1..100 and an original --cursor"
	runWatchRetryExhausted = "live run event retries exhausted; resume from the last printed cursor"
	runWatchOutputFailed   = "unable to write a complete live run event"
)

var errRunWatchComplete = errors.New("run watch output limit reached")
var runQueueRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func runObservationCommand(ctx context.Context, c *client, action string, args []string, out io.Writer) error {
	if len(args) == 1 && args[0] == "--help" {
		_, err := io.WriteString(out, c.text(runObservationHelpZH, runObservationHelpEN))
		return err
	}
	switch action {
	case "watch":
		return runWatchCommand(ctx, c, args, out)
	case "attempts", "attempt", "tool-events":
		return runAttemptReadCommand(ctx, c, action, args, out)
	case "inputs":
		return runInputsCommand(ctx, c, args, out)
	case "workbuddy-status":
		return workBuddyProgressCommand(ctx, c, args, out)
	case "workbuddy-list":
		return workBuddyProgressListCommand(ctx, c, args, out)
	case "scheduling":
		if len(args) == 0 || !compositionCLIIdentifier(args[0], 100) {
			return usage(runSchedulingUsage)
		}
		fs := commandFlags("run scheduling")
		node := fs.String("node", "", "exact workflow node ID")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 || !compositionCLIIdentifier(*node, 150) {
			return usage(runSchedulingUsage)
		}
		path := "/api/v1/runs/" + url.PathEscape(args[0]) + "/nodes/" + url.PathEscape(*node) + "/scheduling"
		return contextRead(ctx, c, path, out)
	case "queue":
		fs := commandFlags("run queue")
		limit := fs.String("limit", "50", "page size 1..100")
		cursor := fs.String("cursor", "", "original queue continuation")
		q := url.Values{}
		// Each scope flag is single-valued. An explicitly invalid scope must
		// never broaden the read to the entire authorized project.
		filter := func(key string, valid func(string) bool) func(string) error {
			return func(value string) error {
				if q.Has(key) || !valid(value) {
					return errors.New(runQueueUsage)
				}
				q.Set(key, value)
				return nil
			}
		}
		fs.Func("route", "project or pool execution route", filter("route", func(value string) bool {
			return value == "project" || value == "pool"
		}))
		fs.Func("run", "exact run ID", filter("run_id", runQueueRunID.MatchString))
		fs.Func("tool", "exact tool profile ID", filter("tool_profile_id", toolcontract.ValidCommandID))
		if fs.Parse(args) != nil || fs.NArg() != 0 {
			return usage(runQueueUsage)
		}
		// The scheduler allows up to 2048 decoded bytes in a base64url cursor.
		if _, ok := runWatchDecimal(*limit, 1, 100); !ok ||
			len(*cursor) > 2731 || !utf8.ValidString(*cursor) || strings.ContainsFunc(*cursor, unicode.IsControl) {
			return usage(runQueueUsage)
		}
		q.Set("limit", *limit)
		if *cursor != "" {
			q.Set("cursor", *cursor)
		}
		return contextRead(ctx, c, "/api/v1/scheduling/queue?"+q.Encode(), out)
	default:
		return usage("unknown run command")
	}
}

func runWatchCommand(ctx context.Context, c *client, args []string, out io.Writer) error {
	if len(args) == 0 || !compositionCLIIdentifier(args[0], 100) {
		return usage(runWatchUsage)
	}
	fs := commandFlags("run watch")
	cursor := fs.String("after", "0", "last persisted sequence from the same run")
	var limit int64
	fs.Func("max-events", "stop after this many new events, 1..100000", func(raw string) error {
		value, ok := runWatchDecimal(raw, 1, maxRunWatchEvents)
		if !ok {
			return strconv.ErrSyntax
		}
		limit = value
		return nil
	})
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return usage(runWatchUsage)
	}
	after, ok := runWatchDecimal(*cursor, 0, maxRunWatchCursor)
	if !ok {
		return usage(runWatchUsage)
	}
	return followRunEvents(ctx, c, args[0], after, limit, out)
}

func runWatchDecimal(value string, minimum, maximum int64) (int64, bool) {
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil && n >= minimum && n <= maximum && strconv.FormatInt(n, 10) == value
}

func followRunEvents(ctx context.Context, c *client, run string, after, limit int64, out io.Writer) error {
	var count int64
	failures := 0
	resumeDetails := func() map[string]any { return map[string]any{"run_id": run, "after": after} }
	for ctx.Err() == nil {
		activity, err := c.readRunEventStream(ctx, run, after, func(raw json.RawMessage, sequence int64) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if sequence <= after {
				return nil
			}
			if err := writeRunWatchEvent(out, raw, c.token); err != nil {
				return &failure{Code: "run_watch_output", Message: runWatchOutputFailed, Details: resumeDetails()}
			}
			// The cursor describes delivered stdout, not just network progress.
			after = sequence
			if limit > 0 {
				count++
				if count == limit {
					return errRunWatchComplete
				}
			}
			return nil
		})
		if errors.Is(err, errRunWatchComplete) {
			return nil
		}
		if ctx.Err() != nil {
			break
		}
		status, retryable := runWatchRetryable(err)
		if !retryable {
			return err
		}
		if activity {
			failures = 0
		}
		if err != nil || !activity {
			failures++
		}
		if failures >= maxRunWatchFailures {
			return &failure{Code: "run_watch_retry_exhausted", Message: runWatchRetryExhausted, Status: status, Details: resumeDetails()}
		}
		delay := min(250*time.Millisecond<<max(0, failures-1), 3*time.Second)
		if status == http.StatusTooManyRequests {
			delay = 3 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		timer.Stop()
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil
	}
	return &failure{Code: "transport", Message: "control request cancelled or timed out", Details: resumeDetails()}
}

func runWatchRetryable(err error) (int, bool) {
	if err == nil {
		return 0, true // The server normally rotates complete streams.
	}
	var f *failure
	if !errors.As(err, &f) || f.Code == "run_watch_protocol" {
		return 0, false
	}
	if f.Status == 0 {
		return 0, f.Code == "transport"
	}
	return f.Status, f.Status == http.StatusRequestTimeout || f.Status == http.StatusTooManyRequests || f.Status >= 500 && f.Status <= 599
}

func writeRunWatchEvent(out io.Writer, raw json.RawMessage, token string) error {
	var payload any = raw
	if token != "" {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return err
		}
		payload = redact(payload, token)
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	n, err := out.Write(line)
	if err == nil && n != len(line) {
		return io.ErrShortWrite
	}
	return err
}

const runObservationHelpEN = `Follow and inspect a run in the current --project.

  run watch RUN [--after SEQUENCE] [--max-events 1..100000]
  run scheduling RUN --node NODE
  run attempts RUN --node NODE [--limit 1..100] [--cursor CURSOR]
  run attempt RUN --node NODE --attempt N
  run tool-events RUN --node NODE --attempt N [--limit 1..100] [--cursor CURSOR]
  run inputs RUN --node NODE --attempt N --fence F [--workbench-origin ORIGIN]
  run workbuddy-status RUN --node NODE
  run workbuddy-list [--limit 1..20] [--filter attention|delivery|input] [--task TASK] [--before NEXT_CURSOR]
  run queue [--route project|pool] [--run RUN] [--tool PROFILE] [--limit 1..100] [--cursor CURSOR]

Watch writes one complete JSON event per line. Stop with Ctrl+C or an explicit
event limit. Resume from the last printed sequence of the same run.
Queue filters the current project by execution route and exact run ID. Use the
same filters when continuing with --cursor.
workbuddy-status reads the platform state and the saved WorkBuddy observation.
workbuddy-list reads one page of recorded project submissions. Use --task to
select the exact Suma task. Use --filter attention for records needing action.
Use --filter delivery for current task runs awaiting handoff, or --filter input
for current task runs with a confirmed saved waiting-input observation.
Keep the task and filter when passing the original next_cursor to --before.
A filtered page can be empty and still continue.
inputs uses a login session to report exact question and delivery states without
printing the question or answer. Open workbench_url to review the original.
`

const runObservationHelpZH = `跟踪和查看当前 --project 的运行。

  run watch RUN [--after SEQUENCE] [--max-events 1..100000]
  run scheduling RUN --node NODE
  run attempts RUN --node NODE [--limit 1..100] [--cursor 原游标]
  run attempt RUN --node NODE --attempt 尝试号
  run tool-events RUN --node NODE --attempt 尝试号 [--limit 1..100] [--cursor 原游标]
  run inputs RUN --node NODE --attempt 尝试号 --fence 租约令牌 [--workbench-origin 工作台地址]
  run workbuddy-status RUN --node NODE
  run workbuddy-list [--limit 1..20] [--filter attention|delivery|input] [--task TASK] [--before NEXT_CURSOR]
  run queue [--route project|pool] [--run RUN] [--tool PROFILE] [--limit 1..100] [--cursor CURSOR]

watch 每行输出一条完整 JSON 事件。按 Ctrl+C 或设置事件数量上限结束。
续传使用同一运行最后一条已输出事件的序号。
queue 按执行路径和准确运行 ID 筛选当前项目。使用 --cursor 续页时保留原筛选条件。
workbuddy-status 读取平台状态和已保存的 WorkBuddy 观察。
workbuddy-list 读取当前项目已记录的提交。使用 --task 选择准确的速码任务，
使用 --filter attention 查找待处理提交。--filter delivery 查看正式任务当前运行的
待交付记录，--filter input 查看当前运行中已核实的等待输入记录。
续页时保留任务和筛选条件，将原 next_cursor 传给 --before。空批仍可能有下一页。
inputs 使用登录会话核对指定尝试的问题及送达状态，不输出问答正文。
打开 workbench_url 可在工作台核对原问题。
`
