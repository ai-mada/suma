import {validOpenCodeCapability} from './opencode-execution-model';
import {validCodexCatalog} from './codex-execution-model';
import {validOctopCapability} from './octop-execution-model';
import {validClaudeCapability} from './claude-cli-model';
import {validACPCapability} from './acp-execution-model';
import { tr as t, UiError } from "./i18n";
import type { RecordItem, Workflow } from "./api";
import type { ToolDiagnostic, ToolExecutionProfile, ToolObservation as ContractToolObservation, ToolProfile as ContractToolProfile } from "../../../packages/contracts/src";
export type ToolProfileDefinition = ToolExecutionProfile;
export type ToolObservation = ContractToolObservation;
export type ToolProfile = ContractToolProfile;
export type ToolUsage = {
  input_tokens?: number;
  cached_input_tokens?: number;
  cache_write_input_tokens?: number;
  output_tokens?: number;
  reasoning_output_tokens?: number;
};
export type UsageState = "complete" | "partial" | "unreported";
export type ToolEvent = {
	/** Verified display identity; original message remains the source evidence. */
	diagnostic?: ToolDiagnostic | null;
  schema_version: 1;
  command_id: string;
  profile_sha256: string;
  protocol: string;
  kind: string;
  session_id?: string;
  item_id?: string;
  message?: string;
  status?: string;
  usage?: ToolUsage;
  usage_state?: UsageState;
};
// Presentation provenance lives only on a cloned view. The non-enumerable
// symbol is never part of persisted/wire ToolEvent JSON or the original fact.
const toolEventSource = Symbol("tool-event-source");
export type ToolEventView = ToolEvent & { readonly [toolEventSource]?: string };

export function toolEventRows(events: readonly ToolEvent[]) {
  const occurrences = new Map<string, number>();
  return events.map(event => {
    // Canonical raw content also guards against a changed payload being shown
    // inside an already-open disclosure, even if a parent identity is reused.
    const content = JSON.stringify(event, (_key, value: unknown) =>
      value && typeof value === "object" && !Array.isArray(value)
        ? Object.fromEntries(Object.keys(value).sort().map(key => [key, (value as Record<string, unknown>)[key]]))
        : value);
    const source = (event as ToolEventView)[toolEventSource] || null;
    const base = JSON.stringify([source, content]);
    const occurrence = occurrences.get(base) || 0;
    occurrences.set(base, occurrence + 1);
    // Without durable provenance, equal-content occurrences are only sibling
    // keys; indistinguishable duplicates do not establish independent identity.
    return { event, key: JSON.stringify([source, content, occurrence]) };
  });
}
export type ToolEvidence = import("../../../packages/contracts/src").ToolEvidence;
export type TaskContextRecord = {
  id: string;
  version: number;
  title: string;
  description: string;
  acceptance_criteria: string[];
};
export type TaskDispatchPlan = import("../../../packages/contracts/src").TaskDispatchPlan;
export function taskPlanStarted(
  plan: Pick<TaskDispatchPlan, "state" | "run_id">,
) {
  return plan.state === "started";
}
export type ToolPage<T> = { items: T[]; total: number; next_cursor?: string };
export type ExecutionWorker = {
  id: string;
  project_id: string;
  worker_id: string;
  version: number;
  status: string;
  deployment: boolean;
  created_at: string;
  expires_at: string;
};
export type ToolRunBinding = {
  profile_id: string;
  profile_sha256: string;
  command: string;
  run_id: string;
  node_id: string;
};
export function toolReturnPanel(run: string, node: string) {
  if (
    !/^[\x21-\x7e]{1,150}$/.test(run) ||
    (node && !/^[\x21-\x7e]{1,150}$/.test(node))
  )
    return "";
  return (
    "return-run:" + encodeURIComponent(run) + "/" + encodeURIComponent(node)
  );
}
export function toolReturnRun(
  panel: string,
): { run: string; node: string } | undefined {
  if (!panel.startsWith("return-run:") || panel.length > 1000) return undefined;
  try {
    const pieces = panel.slice(11).split("/");
    if (pieces.length !== 2) return undefined;
    const run = decodeURIComponent(pieces[0]),
      node = decodeURIComponent(pieces[1]);
    if (toolReturnPanel(run, node) !== panel) return undefined;
    return { run, node };
  } catch {
    return undefined;
  }
}
const encoder = new TextEncoder();
const id = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const digest = /^[a-f0-9]{64}$/;
const protocols: Record<string, string> = {
  command: "text-v1",
  suma: "suma-jsonl-v1",
  codex: "codex-jsonl-v1",
  opencode: "opencode-sdk-v1",
  harness: "dsh-multica-jsonl-v1",
  codebuddy: "codebuddy-jsonl-v1",
  claude: "claude-jsonl-v1",
  octop: "octop-harness-acp-v1",
  qoder: "acp-stdio-v1",
};
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new UiError("development.model.the_inventory_must_use_a_json_object");
  return value as Record<string, unknown>;
}
function fields(value: Record<string, unknown>, allowed: string[]) {
  if (Object.keys(value).some((key) => !allowed.includes(key)))
    throw new UiError("development.model.the_inventory_contains_private_fields_or_secret");
}

function invalidCapability(): never {
  throw new UiError("development.model.the_tool_capability_format_is_invalid");
}
function publicText(value: unknown, maxBytes: number): value is string {
  return typeof value === "string" && value.length > 0 && value.trim() === value && encoder.encode(value).byteLength <= maxBytes && !/[\u0000-\u001f\u007f-\u009f]/.test(value);
}
function publicOrigin(value: unknown): boolean {
  // Only the public origin belongs in inventory. Never import credentials,
  // endpoint paths, query strings or fragments from a local provider config.
  if (!publicText(value, 2048) || !/^(https?):\/\/[^/?#@\s]+$/.test(value)) return false;
  try {
    const origin = new URL(value);
    return !origin.username && !origin.password && (origin.protocol === "https:" || (origin.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(origin.hostname)));
  } catch { return false; }
}
const acpForbidden = ['workspace_tools_version','model_execution_protocol','terminal_session_protocol','test_report_protocol','conflict_repair_protocol','planning','knowledge_extraction','failure_diagnosis','intelligent_operations','dispatch_recommendation','task_conversation_reply','opencode_execution','octop_execution','claude_execution'] as const;
function acpToolProfile(profile: Record<string, unknown> | ToolProfileDefinition) {
  return profile.adapter === 'qoder' && profile.protocol === 'acp-stdio-v1' && profile.read_only === false && validACPCapability(profile.acp_execution) && !acpForbidden.some(key=>!!profile[key]);
}
function validatePublicCapabilities(profile: Record<string, unknown>, observation: Record<string, unknown>) {
  if (profile.user_input_protocol !== undefined && (typeof profile.user_input_protocol !== "string" || profile.user_input_protocol !== "" && (profile.user_input_protocol !== "codex-user-input-v1" || profile.adapter !== "codex" || profile.protocol !== "codex-app-server-v1" || profile.read_only !== false))) invalidCapability();
  const acp = acpToolProfile(profile);
  if (profile.acp_execution !== undefined && !acp || profile.adapter === 'qoder' &&
    (profile.protocol !== 'acp-stdio-v1' || profile.read_only !== false || acpForbidden.some(key=>!!profile[key]) || observation.available === true && !acp)) invalidCapability();
  const claude = profile.adapter === 'claude' && profile.protocol === 'claude-jsonl-v1' && validClaudeCapability(profile.claude_execution);
  if(profile.claude_execution!==undefined&&(!claude||!['2.1.314','2.1.314 (Claude Code)'].includes(String(profile.tool_version)))||profile.adapter==='claude'&&(profile.protocol!=='claude-jsonl-v1'||profile.read_only!==false||observation.available===true&&!claude||profile.workspace_tools_version||profile.model_execution_protocol||profile.terminal_session_protocol||profile.test_report_protocol||profile.conflict_repair_protocol||profile.planning||profile.knowledge_extraction||profile.failure_diagnosis||profile.intelligent_operations||profile.dispatch_recommendation||profile.task_conversation_reply||profile.opencode_execution||profile.octop_execution))invalidCapability();
  const native = profile.adapter === "suma" && profile.protocol === "suma-jsonl-v1";
  // A dedicated shell may expose live files for reading, but never borrows an
  // agent's model, writable workspace tools, test proof or automation purpose.
  const terminal = profile.terminal_session_protocol === "suma-pty-v1" && profile.adapter === "command" && profile.protocol === "text-v1" && profile.read_only === false && !profile.workspace_tools_version && !profile.model_execution_protocol && !profile.test_report_protocol && !profile.conflict_repair_protocol && !profile.planning && !profile.knowledge_extraction && !profile.failure_diagnosis && !profile.intelligent_operations && !profile.dispatch_recommendation && !profile.task_conversation_reply && !profile.opencode_execution && !profile.octop_execution;
  if (profile.terminal_session_protocol !== undefined && (typeof profile.terminal_session_protocol !== "string" || profile.terminal_session_protocol !== "" && !terminal)) invalidCapability();
  const openCode = profile.adapter === "opencode" && profile.protocol === "opencode-sdk-v1" && validOpenCodeCapability(profile.opencode_execution);
  const octop = profile.adapter === 'octop' && profile.protocol === 'octop-harness-acp-v1' && validOctopCapability(profile.octop_execution);
  if(profile.octop_execution!==undefined&&!octop||profile.adapter==='octop'&&(profile.protocol!=='octop-harness-acp-v1'||profile.read_only!==false||profile.model_execution_protocol!=='suma-model-execution-v1'||observation.available===true&&!octop||octop&&profile.workspace_tools_version!=='attempt-workspace-v1'))invalidCapability();
  if (profile.opencode_execution !== undefined && !openCode || profile.adapter === "opencode" && (!openCode || profile.read_only!==false || profile.workspace_tools_version!=="attempt-workspace-v1")) invalidCapability();
  // These purposes use the existing host gateway, with no independent model
  // configuration and no workspace/automation authority. Match the Go
  // SupportsDispatchRecommendationProfile/SupportsTaskConversationReplyProfile
  // contract without manufacturing an enabled administrator ToolProfile.
  let gatewayPurpose = false;
  for (const [field, protocol] of [["dispatch_recommendation", "suma-dispatch-recommendation-v1"], ["task_conversation_reply", "suma-task-conversation-reply-v1"]] as const) {
    if (profile[field] === undefined || profile[field] === null) continue;
    const capability = record(profile[field]);
    fields(capability, ["protocol"]);
    if (gatewayPurpose || capability.protocol !== protocol || !native || profile.read_only !== true || profile.model_execution_protocol !== "suma-model-execution-v1" || profile.workspace_tools_version || profile.test_report_protocol || profile.conflict_repair_protocol || profile.planning || profile.knowledge_extraction || profile.failure_diagnosis || profile.intelligent_operations || profile.opencode_execution || profile.octop_execution) invalidCapability();
    gatewayPurpose = true;
  }
  const workspace = (native || openCode || octop) && profile.read_only === false && profile.workspace_tools_version === "attempt-workspace-v1";
  if (native && workspace && profile.model_execution_protocol !== "suma-model-execution-v1") invalidCapability();
  const codexWorkspace = profile.adapter === "codex" && profile.protocol === "codex-app-server-v1" && profile.read_only === false && validCodexCatalog(observation.codex_catalog);
  for (const [field, protocol, permitted] of [
    ["workspace_tools_version", "attempt-workspace-v1", workspace],
    ["model_execution_protocol", "suma-model-execution-v1", gatewayPurpose || native && workspace || profile.adapter === "octop" && profile.protocol === "octop-harness-acp-v1" && profile.read_only === false],
    ["conflict_repair_protocol", "suma.candidate-conflict-repair.v1", native && workspace],
  ] as const) {
    if (profile[field] !== undefined && profile[field] !== "" && (profile[field] !== protocol || !permitted)) invalidCapability();
    if (profile[field] !== undefined && typeof profile[field] !== "string") invalidCapability();
  }
  const purposes = [["planning", "suma-schedule-plan-v1"], ["knowledge_extraction", "suma-knowledge-extraction-v1"], ["failure_diagnosis", "suma-failure-diagnosis-v1"], ["intelligent_operations", "suma-intelligent-operations-v1"]] as const;
  let purposeCount = 0;
  for (const [field, protocol] of purposes) {
    if (profile[field] === undefined || profile[field] === null) continue;
    const capability = record(profile[field]);
    fields(capability, ["protocol", "model", "endpoint_origin"]);
    if (++purposeCount > 1 || !native || profile.read_only !== true || profile.workspace_tools_version || profile.model_execution_protocol || profile.conflict_repair_protocol || capability.protocol !== protocol || !publicText(capability.model, 256) || !publicOrigin(capability.endpoint_origin)) invalidCapability();
  }
  if (profile.test_report_protocol !== undefined && (typeof profile.test_report_protocol !== "string" || profile.test_report_protocol !== "" && (profile.test_report_protocol !== "suma-junit-report-v1" || profile.adapter !== "command" || profile.protocol !== "text-v1" || profile.read_only !== false || profile.workspace_tools_version || profile.model_execution_protocol || profile.conflict_repair_protocol || purposeCount || profile.opencode_execution || profile.octop_execution))) invalidCapability();
  const read = observation.workspace_observation_protocol, write = observation.workspace_write_protocol;
  if (read !== undefined && typeof read !== "string" || write !== undefined && typeof write !== "string") invalidCapability();
  if (read || write) {
    if ((!workspace && !codexWorkspace && !terminal) || observation.available !== true || read !== "workspace-observation-v1" || write && (!workspace || write !== "workspace-write-v1")) invalidCapability();
  }
  if (observation.model_deployments !== undefined) {
    const deployments = observation.model_deployments;
    if (!Array.isArray(deployments) || deployments.length > 64 || deployments.length > 0 && profile.model_execution_protocol !== "suma-model-execution-v1") invalidCapability();
    const seen = new Set<string>();
    for (const item of deployments) {
      const deployment = record(item);
      fields(deployment, ["deployment_id", "revision", "deployment_sha256", "protocol", "model", "endpoint_origin", "endpoint_config_sha256", "context_window"]);
      if (typeof deployment.deployment_id !== "string" || !id.test(deployment.deployment_id) || !Number.isSafeInteger(deployment.revision) || (deployment.revision as number) < 1 || (deployment.revision as number) > 2147483647 || deployment.protocol !== "openai-chat-completions-v1" || !publicText(deployment.model, 256) || !publicOrigin(deployment.endpoint_origin) || typeof deployment.deployment_sha256 !== "string" || !digest.test(deployment.deployment_sha256) || typeof deployment.endpoint_config_sha256 !== "string" || !digest.test(deployment.endpoint_config_sha256) || !Number.isSafeInteger(deployment.context_window) || (deployment.context_window as number) < 1 || (deployment.context_window as number) > 10000000) invalidCapability();
      const key = JSON.stringify([deployment.deployment_id, deployment.revision]);
      if (seen.has(key)) invalidCapability();
      seen.add(key);
    }
  }
}
export function parseToolManifest(text: string): ToolObservation[] {
  if (encoder.encode(text).byteLength > 256 * 1024)
    throw new UiError("development.model.the_tool_inventory_exceeds_256_kib_reduce");
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new UiError("development.model.the_tool_inventory_is_not_valid_json");
  }
  if (!Array.isArray(raw)) {
    const envelope = record(raw);
    fields(envelope, ["items", "schema_version", "execution_enabled"]);
    raw = envelope.items;
  }
  if (!Array.isArray(raw) || raw.length > 128)
    throw new UiError("development.model.the_tool_inventory_must_contain_at_most");
  const seen = new Set<string>();
  return raw.map((item) => {
    const observation = record(item);
    fields(observation, ["profile", "available", "reason", "model_deployments", "codex_catalog", "workspace_observation_protocol", "workspace_write_protocol"] satisfies (keyof ToolObservation)[]);
    const profile = record(observation.profile);
    if (observation.codex_catalog !== undefined && (!validCodexCatalog(observation.codex_catalog) || observation.available !== true || profile.adapter !== 'codex' || profile.protocol !== 'codex-app-server-v1')) invalidCapability();
    if (profile.protocol === 'codex-app-server-v1' && (profile.read_only !== false || profile.workspace_tools_version || profile.model_execution_protocol || profile.planning || profile.knowledge_extraction || profile.failure_diagnosis || profile.intelligent_operations || observation.available === true && !observation.codex_catalog)) invalidCapability();
    fields(profile, [
      "command_id",
      "adapter",
      "protocol",
      "profile_sha256",
      "executable_sha256",
      "tool_version",
      "read_only",
      "terminal_session_protocol",
      "user_input_protocol",
      "workspace_tools_version", "model_execution_protocol", "conflict_repair_protocol", "test_report_protocol",
      "planning", "knowledge_extraction", "failure_diagnosis", "intelligent_operations", "dispatch_recommendation", "task_conversation_reply", "opencode_execution", "octop_execution", "claude_execution", "acp_execution",
    ] satisfies (keyof ToolProfileDefinition)[]);
    if (
      typeof profile.command_id !== "string" ||
      !id.test(profile.command_id) ||
      seen.has(profile.command_id)
    )
      throw new UiError("development.model.the_inventory_command_id_is_invalid_or");
    seen.add(profile.command_id);
    if (
      typeof profile.adapter !== "string" ||
      protocols[profile.adapter] !== profile.protocol && !(profile.adapter === "codex" && profile.protocol === "codex-app-server-v1")
    )
      throw new UiError("development.model.the_adapter_and_protocol_do_not_match");
    if (
      typeof observation.available !== "boolean" ||
      typeof observation.reason !== "string" ||
      encoder.encode(observation.reason).byteLength > 1000 ||
      (!observation.available && !observation.reason.trim())
    )
      throw new UiError("development.model.the_inventory_must_declare_availability_reasons_are");
    for (const name of ["profile_sha256", "executable_sha256"])
      if (
        profile[name] !== undefined &&
        profile[name] !== "" &&
        (typeof profile[name] !== "string" ||
          !digest.test(profile[name] as string))
      )
        throw new UiError("development.model.tool_fingerprints_must_be_lowercase_sha_256");
    if (
      profile.tool_version !== undefined &&
      (typeof profile.tool_version !== "string" ||
        encoder.encode(profile.tool_version).byteLength > 256)
    )
      throw new UiError("development.model.the_tool_version_format_is_invalid");
    if (
      profile.read_only !== undefined &&
      typeof profile.read_only !== "boolean"
    )
      throw new UiError("development.model.the_tool_read_only_declaration_format_is");
    if (
      observation.available &&
      (!digest.test(String(profile.profile_sha256 || "")) ||
        !digest.test(String(profile.executable_sha256 || "")) ||
        typeof profile.tool_version !== "string" ||
        !profile.tool_version ||
        typeof profile.read_only !== "boolean")
    )
      throw new UiError("development.model.an_available_tool_requires_complete_configuration_and");
    validatePublicCapabilities(profile, observation);
    return observation as unknown as ToolObservation;
  });
}
export function toolBinding(
  node: Record<string, unknown>,
  profile?: ToolProfile,
): Record<string, unknown> {
  const config = {
    ...(node.config &&
    typeof node.config === "object" &&
    !Array.isArray(node.config)
      ? node.config
      : {}),
  } as Record<string, unknown>;
  delete config.tool_profile_id;
  delete config.tool_profile_sha256;
  delete config.claude_execution;
  delete config.acp_execution;
  if (!profile) return { config };
  if (!profile.id || !digest.test(profile.profile.profile_sha256))
    throw new UiError("development.model.choose_an_official_tool_with_a_fixed");
  return {
    command: [profile.profile.command_id],
    ...(profile.profile.adapter==='claude'||profile.profile.adapter==='qoder'?{max_retries:0}:{}),
    config: {
      ...config,
      tool_profile_id: profile.id,
      tool_profile_sha256: profile.profile.profile_sha256,
      ...(profile.profile.adapter==='claude'&&validClaudeCapability(profile.profile.claude_execution)?{claude_execution:{...profile.profile.claude_execution}}:{}),
      ...(acpToolProfile(profile.profile)?{acp_execution:{...profile.profile.acp_execution}}:{}),
    },
  };
}
export function taskToolProtocol(profile: ToolProfileDefinition) {
  return (
    acpToolProfile(profile) ||
    (profile.adapter === "suma" && profile.protocol === "suma-jsonl-v1") ||
    (profile.adapter === "opencode" && profile.protocol === "opencode-sdk-v1" && validOpenCodeCapability(profile.opencode_execution)) ||
    (profile.adapter === "codex" && ["codex-jsonl-v1","codex-app-server-v1"].includes(profile.protocol)) ||
    (profile.adapter === "harness" && profile.protocol === "dsh-multica-jsonl-v1") ||
    (profile.adapter === "codebuddy" && profile.protocol === "codebuddy-jsonl-v1") ||
    (profile.adapter === 'claude' && profile.protocol === 'claude-jsonl-v1' && validClaudeCapability(profile.claude_execution)) ||
    (profile.adapter === 'octop' && profile.protocol === 'octop-harness-acp-v1' && profile.read_only===false && profile.workspace_tools_version==='attempt-workspace-v1' && profile.model_execution_protocol==='suma-model-execution-v1' && validOctopCapability(profile.octop_execution))
  );
}
export function dispatchRequest(
  project: string,
  task: RecordItem,
  workflow: Workflow,
  node: string,
  profile: ToolProfile,
): Record<string, unknown> {
  if (
    !task.id ||
    !Number.isInteger(task.version) ||
    task.version < 1 ||
    (task.project_id && task.project_id !== project)
  )
    throw new UiError("development.model.read_the_current_project_s_official_task");
  if (
    profile.project_id !== project ||
    profile.state !== "enabled" ||
    !profile.available
  )
    throw new UiError("development.model.tool_dispatch_eligibility_is_unconfirmed_check_the");
  if (profile.profile.read_only)
    throw new UiError("development.model.task_development_requires_a_write_permitted_isolated");
  if (!taskToolProtocol(profile.profile))
    throw new UiError("development.model.task_dispatch_supports_only_suma_codex_structured");
  if (
    !workflow.id ||
    !Number.isInteger(workflow.version) ||
    workflow.version < 1 ||
    workflow.nodes.find((item) => item.id === node)?.type !== "agent"
  )
    throw new UiError("development.model.choose_an_actual_agent_node_in_a");
  if (
    !workflow.nodes.some(
      (item) =>
        item.type === "test" &&
        item.config?.candidate_from === node &&
        item.command?.length === 1,
    )
  )
    throw new UiError("development.model.the_workflow_lacks_an_independent_test_that");
  return {
    project_id: project,
    task_id: task.id,
    expected_task_version: task.version,
    workflow_id: workflow.id,
    workflow_version: workflow.version,
    agent_node_id: node,
    tool_profile_id: profile.id,
    expected_tool_version: profile.version,
  };
}
const eventKinds = new Set([
  "session_started",
  "turn_started",
  "activity_started",
  "activity_updated",
  "activity_completed",
  "message",
  "usage",
  "error",
  "turn_completed",
  "turn_failed",
  "waiting_input",
]);
export function toolFactsForAttempt(
  events: unknown[],
  node: unknown,
): ToolEventView[] {
  const target = node as
    | { id?: string; run_id?: string; attempt?: number; fence?: number; worker_id?: string }
    | undefined;
  if (!target?.id || !target.attempt || !Number.isSafeInteger(target.fence) || target.fence! < 0 || !target.worker_id)
    return [];
  return events.flatMap((raw) => {
    const event = raw as {
      sequence?: number;
      run_id?: string;
      type?: string;
      node_id?: string;
      data?: Record<string, unknown>;
    };
    if (
      event.type !== "tool_progress" ||
      (target.run_id !== undefined && event.run_id !== target.run_id) ||
      event.node_id !== target.id ||
      event.data?.attempt !== target.attempt ||
      event.data?.fence !== target.fence ||
      event.data?.worker_id !== target.worker_id ||
      Object.hasOwn(event.data || {}, 'host_operation')
    )
      return [];
    const fact = event.data!.tool_event as ToolEvent | undefined;
    if (
      !fact ||
      typeof fact !== "object" || Array.isArray(fact) ||
      fact.schema_version !== 1 ||
      !eventKinds.has(fact.kind) ||
      typeof fact.command_id !== "string" ||
      typeof fact.profile_sha256 !== "string" ||
      typeof fact.protocol !== "string" ||
      [fact.message,fact.status,fact.session_id,fact.item_id].some(value => value !== undefined && typeof value !== "string") ||
      !id.test(fact.command_id) ||
      !digest.test(fact.profile_sha256) ||
      !Object.values(protocols).includes(fact.protocol)
    )
      return [];
    const view: ToolEventView = { ...fact };
    if (typeof event.run_id === "string" && event.run_id !== ""
      && Number.isSafeInteger(event.sequence) && event.sequence! > 0
      && typeof target.id === "string" && typeof target.worker_id === "string"
      && Number.isSafeInteger(target.attempt) && target.attempt! > 0
      && Number.isSafeInteger(target.fence) && target.fence! > 0) {
      Object.defineProperty(view, toolEventSource, { value: JSON.stringify([
        event.run_id, event.sequence, target.id, target.attempt, target.fence, target.worker_id,
      ]) });
    }
    return [view];
  });
}
export const toolStateNames: Record<string, string> = {
  get paused() { return t("development.model.paused"); },
  get enabled() { return t("development.model.enabled"); },
  get revoked() { return t("development.model.revoked"); },
};
export const toolEventNames: Record<string, string> = {
  get session_started() { return t("development.model.external_session_recorded"); },
  get turn_started() { return t("development.model.tool_started_processing"); },
  get activity_started() { return t("development.model.activity_started"); },
  get activity_updated() { return t("development.model.activity_progress"); },
  get activity_completed() { return t("development.model.activity_completed"); },
  get message() { return t("development.model.tool_message"); },
  get usage() { return t("development.model.tool_usage_report"); },
  get error() { return t("development.model.tool_error"); },
  get turn_completed() { return t("development.facts.tool_reported_turn_completion"); },
  get turn_failed() { return t("development.facts.tool_reported_turn_failure"); },
  get waiting_input() { return t("development.model.waiting_for_external_input"); },
};
