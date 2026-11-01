package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/buildinfo"
	"io"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

const version = buildinfo.Version

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv)) }

func run(args []string, out, stderr io.Writer, lookup func(string) (string, bool)) int {
	app := commandlocale.New("suma-runner", "速码执行器", "suma execution worker", out, stderr, lookup)
	base := app.String("control", "http://127.0.0.1:4310", "控制面服务地址", "control service URL")
	project := app.String("project", "", "项目标识", "project ID")
	id := app.String("worker", "", "稳定的执行器标识", "stable worker ID")
	workspace := app.String("workspace", ".", "执行工作区根目录", "execution workspace root")
	repository := app.String("repository", "", "管理员 Git 检出根目录，供写命令使用", "administrator Git checkout root for write commands")
	baseCommit := app.String("base-commit", "", "隔离尝试使用的完整不可变 Git 提交", "full immutable Git commit for isolated attempts")
	worktrees := app.String("worktrees", "", "仓库 .cache 下的尝试目录（默认 .cache/runner/worktrees）", "attempt directory under repository .cache (default .cache/runner/worktrees)")
	preflight := app.String("merge-preflight", "", "已审核的本地 JSON 请求；只验证证据和提交，不执行合并", "reviewed local JSON request; validate evidence and heads without merging")
	state := app.String("state", ".cache/runner", "执行器持久状态目录", "durable runner state directory")
	inspectState := app.Bool("inspect-state", false, "只读查看指定本地执行日志的安全摘要；不读取凭据或启动执行器", "read-only safe summary of the selected local execution journal; no credentials or worker startup")
	commands := app.String("commands", "", "本地 JSON 命令允许列表", "local JSON command allowlist")
	modelDeployments := app.String("model-deployments", "", "管理员本地模型部署 JSON 允许列表；凭证只使用环境变量引用", "administrator local model deployment JSON allowlist; credentials use environment references")
	externalConfig := app.String("external-config", "", "管理员 JSON 配置：外部工作流插件端点和凭证环境变量名", "administrator JSON map of external workflow plugin endpoints and token environment names")
	allow := app.Bool("allow-exec", false, "允许执行本地允许列表中的命令", "enable execution of local allowlisted commands")
	describe := app.Bool("describe-tools", false, "只读探测本地程序、版本及协议；不读取平台凭证或请求模型", "read-only local executable/version/protocol observations; no platform token or model request")
	prepareTools := app.String("prepare-tools", "", "将可自动配置的本机工具写入全新命令允许列表文件；不连接控制面或领取任务", "prepare local tool commands in a new allowlist file; no control connection or tasks")
	reportTools := app.Bool("report-tools", false, "只上报或恢复一次原工具清单并退出，不领取任务", "report or recover exactly one tool inventory and exit without claiming tasks")
	discoverInstalled := app.Bool("discover-installed-tools", false, "只读检测本机已安装的开发工具，不读取执行凭据或启动任务", "read-only detection of installed developer tools; no worker credentials or tasks")
	describeACPModels := app.Bool("describe-acp-models-stdin", false, "由私有标准输入明确读取已选本机工具的模型目录；不派发任务", "explicitly discover the selected local tool model catalogue from private stdin; no task dispatch")
	localTool := app.String("local-tool", "", "本机检测类型：codex-jsonl、codebuddy-cli、dsh-multica、claude-code-cli、qoder-acp、cursor-cli（仅被动文件检查）；空值检测全部", "local detection entry: codex-jsonl, codebuddy-cli, dsh-multica, claude-code-cli, qoder-acp or cursor-cli (passive files only); empty selects all")
	poolConfig := app.String("pool-config", "", "独立资源池机器及各项目执行范围的私有 JSON 配置", "private JSON configuration of an independent pool machine and per-project execution scopes")
	if stop, code := app.Parse(args, version); stop {
		return code
	}
	if app.FlagProvided("prepare-tools") {
		if toolModeHasAnyFlag(app, "control", "project", "worker", "workspace", "repository", "base-commit", "worktrees", "merge-preflight", "state", "inspect-state", "commands", "model-deployments", "external-config", "pool-config", "allow-exec", "describe-tools", "discover-installed-tools", "describe-acp-models-stdin", "report-tools") {
			return app.Failure("工具配置准备不能与执行器、工作区或其他模式混用", "Tool preparation cannot be combined with worker, workspace or another mode")
		}
		return runToolPreparation(app, *prepareTools, *localTool)
	}
	if app.FlagProvided("report-tools") && (!*reportTools || toolModeHasAnyFlag(app, "inspect-state", "describe-tools", "discover-installed-tools", "describe-acp-models-stdin", "pool-config", "merge-preflight", "external-config", "local-tool")) {
		return app.Failure("单次工具清单上报不能与其他模式、本机检测选择或外部流程配置混用", "Single tool inventory reporting cannot be combined with another mode, local discovery selection or external workflow configuration")
	}
	if *describeACPModels {
		if *base != "http://127.0.0.1:4310" || *project != "" || *id != "" || *workspace != "." || *repository != "" || *baseCommit != "" || *worktrees != "" || *preflight != "" || *state != ".cache/runner" || *inspectState || *commands != "" || *modelDeployments != "" || *externalConfig != "" || *poolConfig != "" || *allow || *describe || *discoverInstalled || *localTool != "" {
			return app.Failure("模型目录探测不能与执行器、工作区或其他模式混用", "Model discovery cannot be combined with worker, workspace or another mode")
		}
		return runACPModelDiscoveryStdin(app, os.Stdin)
	}
	if *discoverInstalled {
		if *base != "http://127.0.0.1:4310" || *project != "" || *id != "" || *workspace != "." || *repository != "" || *baseCommit != "" || *worktrees != "" || *preflight != "" || *state != ".cache/runner" || *inspectState || *commands != "" || *modelDeployments != "" || *externalConfig != "" || *poolConfig != "" || *allow || *describe {
			return app.Failure("本机安装检测不能与执行器、工作区、凭据或命令配置参数混用", "Installed-tool detection cannot be combined with worker, workspace, credential or command configuration")
		}
		return runInstalledToolDiscovery(app, *localTool)
	}
	if *localTool != "" {
		return app.Failure("--local-tool 只能与 --discover-installed-tools 或 --prepare-tools 使用", "--local-tool requires --discover-installed-tools or --prepare-tools")
	}
	// This branch must precede every credential/configuration read and runtime
	// OpenJournal call: opening a live journal can itself mutate durable state.
	if *inspectState {
		if *base != "http://127.0.0.1:4310" || *project != "" || *id != "" || *workspace != "." || *repository != "" || *baseCommit != "" || *worktrees != "" || *preflight != "" || *commands != "" || *modelDeployments != "" || *externalConfig != "" || *poolConfig != "" || *allow || *describe {
			fmt.Fprintln(stderr, app.Text("本地执行日志诊断不能与执行、配置、资源池或控制面参数混用", "local journal inspection cannot be combined with execution, configuration, pool or control options"))
			return 2
		}
		return runJournalInspection(app, *state)
	}
	// Errors from runner, the OS and JSON decoding retain their original text.
	fail := func(err error) int { fmt.Fprintln(stderr, err); return 1 }
	if *poolConfig != "" {
		if *project != "" || *id != "" || *commands != "" || *modelDeployments != "" || *repository != "" || *baseCommit != "" || *worktrees != "" || *externalConfig != "" || *preflight != "" || (!*describe && app.Getenv("SUMA_RUNNER_TOKEN") != "") {
			return app.Failure("资源池模式不能与 --project/--worker、项目执行器配置或凭证混用", "pool mode is independent of --project/--worker and project-runner configuration or credentials")
		}
		if err := runPool(app, *base, *poolConfig, *state, *allow, *describe); err != nil {
			return fail(err)
		}
		return 0
	}
	if *preflight != "" {
		b, e := os.ReadFile(*preflight)
		if e != nil {
			return fail(e)
		}
		var request runner.MergePreflightRequest
		if e = json.Unmarshal(b, &request); e != nil {
			return fail(e)
		}
		result, e := runner.MergePreflight(context.Background(), runner.ExecConfig{Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees}, request)
		if e != nil {
			return fail(e)
		}
		if e = json.NewEncoder(out).Encode(result); e != nil {
			return fail(e)
		}
		return 0
	}
	// A pending report needs only its original identity and locked journal. The
	// current commands file may have moved or become invalid after the lost ACK.
	var reportJournal *runner.Journal
	if *reportTools {
		if *project == "" || *id == "" || app.Getenv("SUMA_RUNNER_TOKEN") == "" {
			return app.Failure("必须提供 --project、--worker 和 SUMA_RUNNER_TOKEN；执行器须持有已登记的执行凭证", "--project, --worker and SUMA_RUNNER_TOKEN are required; worker must have a registered execution credential")
		}
		var openErr error
		reportJournal, openErr = runner.OpenJournal(*state)
		if openErr != nil {
			return fail(openErr)
		}
		defer reportJournal.Close()
		if reportJournal.PendingInventory() != nil {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()
			worker := runner.Worker{Client: &runner.Client{BaseURL: *base, Token: app.Getenv("SUMA_RUNNER_TOKEN"), ProjectID: *project}, Journal: reportJournal, ID: *id}
			return runToolReport(ctx, app, &worker)
		}
	}
	root, err := filepath.Abs(*workspace)
	if err != nil {
		return fail(err)
	}
	specs := map[string]runner.CommandSpec{}
	localModels := []runner.ModelDeploymentSpec{}
	if *modelDeployments != "" {
		raw, readErr := os.ReadFile(*modelDeployments)
		if readErr != nil {
			return fail(readErr)
		}
		localModels, err = runner.DecodeModelDeployments(raw)
		if err != nil {
			return fail(err)
		}
	}
	if *commands != "" {
		b, e := os.ReadFile(*commands)
		if e != nil {
			return fail(e)
		}
		if specs, e = runner.DecodeCommands(b); e != nil {
			return fail(e)
		}
	}
	if err = runner.ValidateToolConfiguration(runner.ExecConfig{Commands: specs, ModelDeployments: localModels}); err != nil {
		return fail(err)
	}
	if *describe {
		items, describeErr := runner.DescribeTools(context.Background(), runner.ExecConfig{AllowExec: *allow, Commands: specs, Workspace: root, Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees, ModelDeployments: localModels})
		if describeErr != nil {
			return fail(describeErr)
		}
		if err = json.NewEncoder(out).Encode(map[string]any{"items": items, "execution_enabled": *allow}); err != nil {
			return fail(err)
		}
		return 0
	}
	if *project == "" || *id == "" || app.Getenv("SUMA_RUNNER_TOKEN") == "" {
		return app.Failure("必须提供 --project、--worker 和 SUMA_RUNNER_TOKEN；执行器须持有已登记的执行凭证", "--project, --worker and SUMA_RUNNER_TOKEN are required; worker must have a registered execution credential")
	}
	journal := reportJournal
	if journal == nil {
		journal, err = runner.OpenJournal(*state)
		if err != nil {
			return fail(err)
		}
		defer journal.Close()
	}
	external := map[string]runner.ExternalSpec{}
	if *externalConfig != "" {
		b, e := os.ReadFile(*externalConfig)
		if e != nil {
			return fail(e)
		}
		if e = json.Unmarshal(b, &external); e != nil {
			return fail(e)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	w := runner.Worker{Client: &runner.Client{BaseURL: *base, Token: app.Getenv("SUMA_RUNNER_TOKEN"), ProjectID: *project}, Journal: journal, ID: *id, Exec: runner.ExecConfig{AllowExec: *allow, Workspace: root, Repository: *repository, BaseCommit: *baseCommit, WorktreeRoot: *worktrees, Commands: specs, ModelDeployments: localModels, External: external, Secrets: []string{app.Getenv("SUMA_RUNNER_TOKEN")}}}
	if *reportTools {
		return runToolReport(ctx, app, &w)
	}
	if err = w.Run(ctx); err != nil && ctx.Err() == nil {
		return fail(err)
	}
	return 0
}
