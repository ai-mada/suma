"""Public Octop Harness construction with no GUI, plugins, teams or delegated CLI."""

from __future__ import annotations

from pathlib import Path

from .contract import RuntimeFault
from .policy import WorkspaceOnlyMiddleware
from .workspace import WorkspaceBridge


class FrozenHarness:
    """A public stream facade; the official ACP adapter cannot alter execution inputs."""

    def __init__(self, harness, binding, bridge, policy):
        self.harness, self.binding, self.bridge, self.policy = harness, binding, bridge, policy

    async def stream(self, value):
        from octop_harness import ChatRequest
        request = ChatRequest(
            messages=value["messages"], thread_id=value["thread_id"], source="suma-acp",
            model="suma_gateway/" + self.binding["deployment"]["model"], skills=[],
            configurable={"model_settings": {"max_tokens": self.binding["parameters"]["max_output_tokens"], "parallel_tool_calls": False}},
            recursion_limit=self.binding["parameters"]["max_steps"] * 3 + 10,
        )
        async for chunk in self.harness.stream(request):
            yield chunk
        if self.bridge.fatal_code:
            raise RuntimeFault(self.bridge.fatal_code)
        if self.policy.fatal_code:
            raise RuntimeFault(self.policy.fatal_code)


async def run_child(config: dict, request: dict, execution: dict):
    # Imports follow private HOME setup in the entrypoint. Never bootstrap vendor
    # workspace templates or copy any personal Octop DB, agent, history or credential.
    from acp import run_agent
    from octop_harness import HarnessAgent, HarnessAgentConfig, ModelConfig, ProviderConfig
    from octop_harness.acp.server import build_harness_acp_agent
    from octop_harness.llm.factory import ChatModelFactory

    home = Path(config["home"])
    binding, gateway = execution["binding"], execution["gateway"]
    parameters = binding["parameters"]
    selected_model = binding["deployment"]["model"]
    frozen_ref = "suma_gateway/" + selected_model

    class FrozenModelFactory(ChatModelFactory):
        """Use a public factory seam to disable hidden SDK retries, not streaming."""

        def get_chat_model(self, model_ref, *, max_retries=None):
            if model_ref != frozen_ref or max_retries not in (None, 0):
                raise RuntimeFault("model_binding_invalid")
            model = super().get_chat_model(model_ref, max_retries=0)
            # langchain-openai injects temperature=1 for some model names even
            # when callers omit it. Temperature is not part of our frozen
            # parameter contract; clear that public field rather than expanding
            # the host broker's allowlist or silently selecting a new setting.
            model.temperature = None
            return model

    async with WorkspaceBridge(request) as bridge:
        policy = WorkspaceOnlyMiddleware(parameters["max_steps"], parameters["max_output_tokens"])
        settings = HarnessAgentConfig(
            name="suma-octop-harness", workspace_dir=home / "agent", language="en",
            # DeepAgents performs its own skill-directory listing before the
            # model call. Its workspace lives under home/agent, so the backend
            # must include that private metadata subtree as well as home/state.
            # This is still fresh runtime state, never the Go source worktree.
            backend={"type": "filesystem", "root_dir": str(home), "virtual_mode": True},
            providers=[ProviderConfig(id="suma_gateway", name="Suma attempt model gateway", protocol="openai", base_url=gateway["base_url"], api_key=gateway["token"], stream_usage=True, models=[ModelConfig(id=selected_model, max_output_tokens=parameters["max_output_tokens"], api_mode="chat_completions", api_fallback="none", native_tool_search=False)])],
            default_model=frozen_ref,
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
        factory = FrozenModelFactory(settings.providers, agent_config=settings)
        try:
            async with HarnessAgent(settings, model_factory=factory) as harness:
                await run_agent(build_harness_acp_agent(FrozenHarness(harness, binding, bridge, policy), agent_id="suma-octop-harness"), stdio_buffer_limit_bytes=2 * 1024 * 1024)
        finally:
            await factory.aclose()
