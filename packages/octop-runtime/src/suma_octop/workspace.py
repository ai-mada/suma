"""Official MCP transport bound to one Go-owned attempt capability.

The host alone implements file access, fixed checks and its durable operation ledger.
No model argument can override authorization or the hidden operation identifier.
"""

from __future__ import annotations

import asyncio
import contextvars
import json
from contextlib import AsyncExitStack
from datetime import timedelta

import httpx
from jsonschema import Draft202012Validator
from langchain_core.tools import StructuredTool
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client

from .contract import RuntimeFault, operation_id, verify_catalog

CURRENT_TOOL_CALL: contextvars.ContextVar[tuple[str, str] | None] = contextvars.ContextVar("suma_tool_call", default=None)
_ORDINARY = {"native_workspace_" + code for code in ("args", "arguments", "path", "missing", "text", "output_limit", "conflict", "operation", "check", "checks_stale", "cursor")}


class BoundedLocalTransport(httpx.AsyncHTTPTransport):
    """Never redirect, use ambient proxy credentials, or exceed bounded MCP bodies."""

    def __init__(self, endpoint: str):
        super().__init__(retries=0)
        self.endpoint = endpoint

    async def handle_async_request(self, request):
        if str(request.url) != self.endpoint or len(await request.aread()) > 2 * 1024 * 1024:
            raise RuntimeFault("workspace_transport")
        response = await super().handle_async_request(request)
        # The Go workspace transport is deliberately JSON-only, not an unbounded SSE feed.
        if response.status_code == 200:
            content = bytearray()
            async for data in response.aiter_bytes():
                content.extend(data)
                if len(content) > 2 * 1024 * 1024:
                    await response.aclose()
                    raise RuntimeFault("workspace_transport")
            await response.aclose()
            return httpx.Response(response.status_code, headers=response.headers, content=bytes(content), request=request)
        return response


class WorkspaceBridge:
    def __init__(self, request: dict):
        self.request = request
        self.bridge = request["workspace_bridge"]
        self.stack = AsyncExitStack()
        self.lock = asyncio.Lock()
        self.catalog = {}
        self.fatal_code = None

    async def __aenter__(self):
        try:
            client = await self.stack.enter_async_context(httpx.AsyncClient(
                transport=BoundedLocalTransport(self.bridge["endpoint"]),
                headers={"Authorization": "Bearer " + self.bridge["token"]},
                timeout=httpx.Timeout(70, connect=5), follow_redirects=False, trust_env=False,
            ))
            read, write, _ = await self.stack.enter_async_context(streamable_http_client(self.bridge["endpoint"], http_client=client))
            self.session = await self.stack.enter_async_context(ClientSession(read, write, read_timeout_seconds=timedelta(seconds=70)))
            identity = await self.session.initialize()
            if identity.serverInfo.name != "suma-native-workspace" or identity.serverInfo.version != "attempt-workspace-v1":
                raise RuntimeFault("workspace_catalog")
            self.catalog = verify_catalog((await self.session.list_tools()).model_dump(by_alias=True, exclude_none=True))
            return self
        except BaseException:
            await self.stack.aclose()
            raise

    async def __aexit__(self, *args):
        return await self.stack.__aexit__(*args)

    def tools(self) -> list[StructuredTool]:
        result = []
        for name, entry in self.catalog.items():
            def create(target, schema):
                async def invoke(**arguments):
                    identity = CURRENT_TOOL_CALL.get()
                    if identity is None or identity[1] != target:
                        raise RuntimeFault("workspace_identity")
                    return await self.invoke(target, arguments, identity[0], schema)
                return invoke
            result.append(StructuredTool(name=name, description=entry.get("description") or name, args_schema=entry["inputSchema"], coroutine=create(name, entry["inputSchema"])))
        return result

    async def invoke(self, name: str, arguments: dict, real_call_id: str, schema: dict) -> str:
        if self.fatal_code:
            raise RuntimeFault(self.fatal_code)
        if name not in self.catalog or not Draft202012Validator(schema).is_valid(arguments):
            raise RuntimeFault("workspace_arguments")
        # Serialize local operations. A transport failure can mean an applied effect;
        # never automatically replay the model or invent a new operation identity.
        async with self.lock:
            try:
                result = await self.session.call_tool(name, arguments, meta={"suma/operation_id": operation_id(self.request["request_id"], real_call_id, name)})
            except Exception as exc:
                self.fatal_code = "workspace_transport"
                raise RuntimeFault("workspace_transport") from exc
        payload = result.model_dump(by_alias=True, exclude_none=True)
        if len(json.dumps(payload, ensure_ascii=False).encode()) > 65536:
            self.fatal_code = "workspace_effects_unknown"
            raise RuntimeFault("workspace_effects_unknown")
        if result.isError:
            error = (result.structuredContent or {}).get("error")
            if not isinstance(error, dict) or error.get("fatal") is not False or error.get("code") not in _ORDINARY:
                self.fatal_code = "workspace_effects_unknown"
                raise RuntimeFault("workspace_effects_unknown")
        return json.dumps(result.structuredContent or {"content": payload.get("content", []), "isError": result.isError}, ensure_ascii=False, separators=(",", ":"))
