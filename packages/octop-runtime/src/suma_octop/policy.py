"""Public LangChain middleware: the allowlist is an execution boundary, not a hint."""

from langchain.agents.middleware import AgentMiddleware
from langchain_core.messages import ToolMessage

from .contract import RuntimeFault, TOOL_NAMES, safe_id
from .workspace import CURRENT_TOOL_CALL


class WorkspaceOnlyMiddleware(AgentMiddleware):
    def __init__(self, max_steps: int, max_output_tokens: int):
        self.max_steps = max_steps
        self.max_output_tokens = max_output_tokens
        self.model_steps = 0
        self.fatal_code = None

    def _model_request(self, request):
        if self.fatal_code:
            raise RuntimeFault(self.fatal_code)
        self.model_steps += 1
        if self.model_steps > self.max_steps:
            self.fatal_code = "model_steps_exhausted"
            raise RuntimeFault("model_steps_exhausted")
        allowed = [tool for tool in (request.tools or []) if getattr(tool, "name", None) in TOOL_NAMES]
        if len(allowed) != 6 or {tool.name for tool in allowed} != set(TOOL_NAMES):
            self.fatal_code = "workspace_catalog"
            raise RuntimeFault("workspace_catalog")
        settings = dict(request.model_settings or {})
        settings["max_tokens"] = self.max_output_tokens
        settings["parallel_tool_calls"] = False
        return request.override(tools=allowed, model_settings=settings)

    def wrap_model_call(self, request, handler):
        return handler(self._model_request(request))

    async def awrap_model_call(self, request, handler):
        return await handler(self._model_request(request))

    def _identity(self, request):
        call = request.tool_call
        if call.get("name") not in TOOL_NAMES or not safe_id(call.get("id")):
            return None
        return call["id"], call["name"]

    def _denied(self, request):
        # Even a provider's hallucinated builtin must not reach the SDK handler.
        self.fatal_code = "workspace_tool_denied"
        return ToolMessage(content='{"error":{"code":"workspace_tool_denied","fatal":true}}', tool_call_id=str(request.tool_call.get("id") or "denied"), status="error")

    def wrap_tool_call(self, request, handler):
        identity = self._identity(request)
        if identity is None:
            return self._denied(request)
        token = CURRENT_TOOL_CALL.set(identity)
        try:
            return handler(request)
        finally:
            CURRENT_TOOL_CALL.reset(token)

    async def awrap_tool_call(self, request, handler):
        identity = self._identity(request)
        if identity is None:
            return self._denied(request)
        token = CURRENT_TOOL_CALL.set(identity)
        try:
            return await handler(request)
        finally:
            CURRENT_TOOL_CALL.reset(token)
