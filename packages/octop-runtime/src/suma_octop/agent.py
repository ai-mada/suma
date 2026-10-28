"""Public Octop Harness construction with no GUI, plugins, teams or delegated CLI."""

from __future__ import annotations

import os
from pathlib import Path

from .contract import RuntimeFault
from .policy import WorkspaceOnlyMiddleware
from .workspace import WorkspaceBridge


class FrozenHarness:
    """A public stream facade; the official ACP adapter cannot alter execution inputs."""

    def __init__(self, harness, config, bridge, policy):
        self.harness, self.config, self.bridge, self.policy = harness, config, bridge, policy

    async def stream(self, value):
        from octop_harness import ChatRequest
        request = ChatRequest(
            messages=value["messages"], thread_id=value["thread_id"], source="suma-acp",
            model=self.config["provider_id"] + "/" + self.config["model"], skills=[],
            configurable={"model_settings": {"max_tokens": self.config["max_output_tokens"], "parallel_tool_calls": False}},
            recursion_limit=self.config["max_steps"] * 3 + 10,
        )
        async for chunk in self.harness.stream(request):
            yield chunk
        if self.bridge.fatal_code:
            raise RuntimeFault(self.bridge.fatal_code)
        if self.policy.fatal_code:
            raise RuntimeFault(self.policy.fatal_code)


async def run_child(config: dict, request: dict):
    # Imports follow private HOME setup in the entrypoint. Never bootstrap vendor
    # workspace templates or copy any personal Octop DB, agent, history or credential.
    from acp import run_agent
    from octop_harness import HarnessAgent, HarnessAgentConfig, ModelConfig, ProviderConfig
    from octop_harness.acp.server import build_harness_acp_agent

    home = Path(config["home"])
    key_ref = config.get("api_key_env_ref")
    key = os.environ.get(key_ref, "") if key_ref else "suma-local-gateway"
    if key_ref and not key:
        raise RuntimeFault("provider_credential_missing")
    async with WorkspaceBridge(request) as bridge:
        policy = WorkspaceOnlyMiddleware(config["max_steps"], config["max_output_tokens"])
        settings = HarnessAgentConfig(
            name="suma-octop-harness", workspace_dir=home / "agent", language="en",
            # DeepAgents performs its own skill-directory listing before the
            # model call. Its workspace lives under home/agent, so the backend
            # must include that private metadata subtree as well as home/state.
            # This is still fresh runtime state, never the Go source worktree.
            backend={"type": "filesystem", "root_dir": str(home), "virtual_mode": True},
            providers=[ProviderConfig(id=config["provider_id"], name="Suma frozen provider", protocol="openai", base_url=config["base_url"], api_key=key, models=[ModelConfig(id=config["model"], max_output_tokens=config["max_output_tokens"], api_mode="chat_completions")])],
            default_model=config["provider_id"] + "/" + config["model"],
            tools=bridge.tools(), middleware=[policy], mcp_server_configs={},
            team_enabled=False, team_peers=(), subagents_auto_load=False, subagents=[],
            acp_delegate_enabled=False, acp_runners={}, ask_user_enabled=False,
            bootstrap_enabled=False, memory_enabled=False, todos_enabled=False,
            web_search_tools=False, media_generation=None, decision=None,
            checkpointer=False, deferred_tools=frozenset(), defer_mcp_tools=False,
            tool_search_mode="eager", session_log_enabled=False, memory_jsonl_enabled=False,
            model_retry_enabled=False, media_offload_enabled=False, skills_dir=[],
            system_prompt=("You are the Suma coding agent. Implement the assigned goal using only the six suma_workspace_* tools. "
                           "Those tools operate on the assigned private code worktree. The SDK filesystem is private runtime state, never source code. "
                           "Use content-hash preconditions, run the configured workspace check before finishing, and report actual results. "
                           "Do not use shell, network, delegation, approvals, arbitrary checks or external tools. Never claim human acceptance or deployment."),
        )
        async with HarnessAgent(settings) as harness:
            await run_agent(build_harness_acp_agent(FrozenHarness(harness, config, bridge, policy), agent_id="suma-octop-harness"), stdio_buffer_limit_bytes=2 * 1024 * 1024)
