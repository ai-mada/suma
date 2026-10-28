"""JSONL carrier parent and ACP child lifecycle.

ACP's cancel notification is a request, never a stop receipt. Only after the child
has exited can this process emit agent_parent_stopped. Go owns the outer process
tree, durable effects, independent tests and candidate seal.
"""

from __future__ import annotations

import asyncio
import importlib.metadata
import json
import os
import subprocess
import sys
import importlib.resources
from datetime import datetime, timezone
from pathlib import Path

from . import ACP_VERSION, HARNESS_VERSION, MCP_VERSION, PROTOCOL, VERSION
from .contract import RuntimeFault, TOOL_NAMES, closed, parse_config, parse_execute, parse_json, safe_id
from .model import model_execution_capability, resolve_execution


def versions() -> dict:
    expected = {"octop-harness": HARNESS_VERSION, "agent-client-protocol": ACP_VERSION, "mcp": MCP_VERSION}
    for package, version in expected.items():
        if importlib.metadata.version(package) != version:
            raise RuntimeFault("sdk_version_mismatch")
    lock = importlib.resources.files("suma_octop").joinpath("dependency-lock.json")
    if lock.is_file():
        for package, version in json.loads(lock.read_text(encoding="utf-8")).items():
            if importlib.metadata.version(package) != version:
                raise RuntimeFault("sdk_version_mismatch")
    # Inspect public APIs, without constructing an agent, state or provider client.
    from acp import run_agent, spawn_agent_process
    from octop_harness import HarnessAgent, HarnessAgentConfig, ChatRequest
    from octop_harness.acp.server import build_harness_acp_agent
    if not all(callable(api) for api in (run_agent, spawn_agent_process, HarnessAgent, HarnessAgentConfig, ChatRequest, build_harness_acp_agent)):
        raise RuntimeFault("sdk_api_missing")
    return {"harness_version": HARNESS_VERSION, "acp_sdk_version": ACP_VERSION}


def emit(frame: dict):
    data = json.dumps({"v": 1, **frame}, ensure_ascii=False, separators=(",", ":"))
    sys.stdout.write(data + "\n")
    sys.stdout.flush()


def config_from_environment() -> dict:
    raw = os.environ.get("SUMA_OCTOP_CONFIG", "")
    if not raw or len(raw.encode()) > 32768:
        raise RuntimeFault("configuration_invalid")
    return parse_config(parse_json(raw))


def private_environment(config: dict, request: dict, execution: dict) -> dict:
    home = Path(config["home"])
    # Create once and fail closed on reused/symlinked state. Recovery belongs to
    # the Go operation ledger, not replaying a previous model conversation.
    if home.is_symlink() or (home.exists() and any(home.iterdir())):
        raise RuntimeFault("state_not_fresh")
    for parent in (home, *home.parents):
        if parent.is_symlink():
            raise RuntimeFault("state_not_fresh")
    home.mkdir(mode=0o700, parents=True, exist_ok=True)
    (home / ".suma-attempt").write_text(request["request_id"], encoding="utf-8")
    for child in ("tmp", "state", "agent", "logs"):
        (home / child).mkdir(mode=0o700)
    env = {key: value for key, value in os.environ.items() if key.upper() in {"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "PATH", "LANG", "LC_ALL"}}
    env.update({"HOME": str(home), "USERPROFILE": str(home), "OCTOP_HOME": str(home), "TEMP": str(home / "tmp"), "TMP": str(home / "tmp"), "PYTHONNOUSERSITE": "1", "PYTHONDONTWRITEBYTECODE": "1", "PYTHONUNBUFFERED": "1", "PYTHONUTF8": "1", "HF_HUB_OFFLINE": "1", "LANGCHAIN_TRACING_V2": "false", "LANGSMITH_TRACING": "false", "SUMA_OCTOP_CONFIG": json.dumps(config), "SUMA_OCTOP_REQUEST": json.dumps(request), "SUMA_MODEL_EXECUTION": json.dumps(execution)})
    return env


class CarrierClient:
    """An ACP client with no filesystem/terminal or permission-grant capabilities."""

    def __init__(self, request_id: str):
        self.request_id = request_id
        self.session_id = None
        self.failure = None

    async def request_permission(self, **kwargs):
        from acp.schema import DeniedOutcome, RequestPermissionResponse
        self.failure = "permission_not_supported"
        return RequestPermissionResponse(outcome=DeniedOutcome(outcome="cancelled"))

    async def session_update(self, session_id, update, **kwargs):
        if session_id != self.session_id:
            self.failure = "acp_session_mismatch"
            return
        meta = getattr(update, "field_meta", None) or {}
        if meta.get("octop.error") is True:
            self.failure = "agent_execution_failed"
        kind = getattr(update, "session_update", "")
        if kind in {"tool_call", "tool_call_update"}:
            title = getattr(update, "title", "")
            tool = title.removeprefix("suma_workspace_") if title in TOOL_NAMES else None
            # Do not forward raw arguments, tool output, reasoning, logs or secrets.
            status = {"pending": "started", "in_progress": "started", "completed": "completed", "failed": "failed"}.get(getattr(update, "status", ""))
            if tool and status:
                emit({"type": "progress", "request_id": self.request_id, "session_id": session_id, "kind": "tool", "status": status, "tool": tool})

    async def read_text_file(self, **kwargs):
        raise RuntimeFault("acp_client_operation_denied")

    write_text_file = read_text_file
    create_terminal = read_text_file
    terminal_output = read_text_file
    release_terminal = read_text_file
    wait_for_terminal_exit = read_text_file
    kill_terminal = read_text_file

    async def ext_method(self, **kwargs):
        raise RuntimeFault("acp_client_operation_denied")

    async def ext_notification(self, **kwargs):
        raise RuntimeFault("acp_client_operation_denied")


async def read_frames(queue: asyncio.Queue):
    # A daemon feeder avoids Windows pipe support assumptions and shutdown waits
    # on a blocking stdin read. The ACP SDK itself uses the same Windows pattern.
    import threading
    loop = asyncio.get_running_loop()
    def feed():
        try:
            while True:
                line = sys.stdin.buffer.readline(2 * 1024 * 1024 + 1)
                loop.call_soon_threadsafe(queue.put_nowait, line)
                if not line:
                    return
        except (OSError, RuntimeError):
            return
    threading.Thread(target=feed, daemon=True).start()


async def run_parent(config: dict, queue: asyncio.Queue):
    from acp import PROTOCOL_VERSION, spawn_agent_process, text_block
    from acp.schema import ClientCapabilities, Implementation

    raw = await queue.get()
    if not raw or len(raw) > 2 * 1024 * 1024:
        raise RuntimeFault("request_invalid")
    request = parse_execute(parse_json(raw))
    rid = request["request_id"]
    execution = resolve_execution(request, parse_json(os.environ.get("SUMA_MODEL_EXECUTION", "")))
    binding = execution["binding"]
    env = private_environment(config, request, execution)
    sdk = versions()
    emit({"type": "ready", "request_id": rid, **sdk, "binding_sha256": binding["binding_sha256"]})
    client = CarrierClient(rid)
    cancelled = False
    terminal = None
    process = None
    try:
        entry = str(Path(sys.argv[0]).resolve())
        async with spawn_agent_process(client, sys.executable, entry, "--acp-child", env=env, cwd=config["home"], transport_kwargs={"stderr": subprocess.DEVNULL, "limit": 2 * 1024 * 1024, "shutdown_timeout": 2.0}) as (connection, process):
            initialized = await asyncio.wait_for(connection.initialize(protocol_version=PROTOCOL_VERSION, client_capabilities=ClientCapabilities(), client_info=Implementation(name="suma-octop-runtime", version=VERSION)), 45)
            if initialized.protocol_version != PROTOCOL_VERSION or initialized.auth_methods:
                raise RuntimeFault("acp_capabilities_invalid")
            session = await asyncio.wait_for(connection.new_session(cwd=config["home"], mcp_servers=[]), 15)
            if not safe_id(session.session_id):
                raise RuntimeFault("acp_session_invalid")
            client.session_id = session.session_id
            emit({"type": "started", "request_id": rid, "session_id": client.session_id})
            emit({"type": "progress", "request_id": rid, "session_id": client.session_id, "kind": "model", "status": "started"})
            prompt = asyncio.create_task(connection.prompt(session_id=client.session_id, prompt=[text_block(request["goal"])]))
            following = asyncio.create_task(queue.get())
            try:
                done, _ = await asyncio.wait({prompt, following}, return_when=asyncio.FIRST_COMPLETED)
                if following in done:
                    next_raw = following.result()
                    if not next_raw:
                        raise RuntimeFault("carrier_disconnected")
                    frame = closed(parse_json(next_raw), {"v", "type", "request_id"})
                    if type(frame["v"]) is not int or frame != {"v": 1, "type": "cancel", "request_id": rid}:
                        raise RuntimeFault("request_invalid")
                    cancelled = True
                    await connection.cancel(session_id=client.session_id)
                    # Notification delivery is not a stop ACK. Exit the SDK process
                    # context and wait for actual child exit before emitting receipt.
                    prompt.cancel()
                else:
                    answer = prompt.result()
                    if answer.stop_reason != "end_turn" or client.failure:
                        raise RuntimeFault(client.failure or "acp_terminal_invalid")
                    terminal = {"type": "completed", "request_id": rid, "session_id": client.session_id, "binding_sha256": binding["binding_sha256"], "stop_reason": "end_turn", "completed_at": datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")}
            finally:
                following.cancel()
                if not prompt.done():
                    prompt.cancel()
                await asyncio.gather(prompt, following, return_exceptions=True)
    except Exception as exc:
        terminal = {"type": "failed", "request_id": rid, "code": exc.code if isinstance(exc, RuntimeFault) else "agent_execution_failed"}
    stopped = process is None or process.returncode is not None
    if not stopped:
        # SDK shutdown normally guarantees this, but do not invent a stop receipt.
        raise RuntimeFault("agent_stop_unconfirmed")
    if not cancelled and process is not None and process.returncode != 0:
        terminal = {"type": "failed", "request_id": rid, "code": "agent_process_failed"}
    if cancelled:
        emit({"type": "cancelled", "request_id": rid, **({"session_id": client.session_id} if client.session_id else {}), "cancel_requested": True, "agent_parent_stopped": True})
    else:
        emit({**(terminal or {"type": "failed", "request_id": rid, "code": "agent_execution_failed"}), "agent_parent_stopped": True})


async def async_main(args: list[str]):
    if args == ["--model-execution-probe"]:
        sys.stdout.write(json.dumps(model_execution_capability(), separators=(",", ":")) + "\n")
        sys.stdout.flush()
        return
    config = config_from_environment()
    if args == ["--probe"]:
        emit({"type": "probe", "protocol": PROTOCOL, "runtime_version": VERSION, **versions(), "workspace_tools_version": "attempt-workspace-v1", "model_execution_protocol": "suma-model-execution-v1", "gateway_protocol": "suma-model-gateway-v1"})
        return
    if args == ["--acp-child"]:
        home = Path(config["home"])
        request = parse_execute(parse_json(os.environ.pop("SUMA_OCTOP_REQUEST", "")))
        execution = resolve_execution(request, parse_json(os.environ.pop("SUMA_MODEL_EXECUTION", "")))
        if not home.is_dir() or home.is_symlink() or (home / ".suma-attempt").read_text(encoding="utf-8") != request["request_id"]:
            raise RuntimeFault("state_not_fresh")
        for key in ("HOME", "USERPROFILE", "OCTOP_HOME"):
            os.environ[key] = str(home)
        # Root handlers are local, without writing raw protocol/provider output.
        import logging
        logging.basicConfig(filename=home / "logs" / "runtime.log", level=logging.WARNING)
        versions()
        from .agent import run_child
        await run_child(config, request, execution)
        return
    if args:
        raise RuntimeFault("arguments_invalid")
    queue = asyncio.Queue()
    await read_frames(queue)
    await run_parent(config, queue)


def main():
    os.environ.update({"HF_HUB_OFFLINE": "1", "LANGCHAIN_TRACING_V2": "false", "LANGSMITH_TRACING": "false"})
    try:
        asyncio.run(async_main(sys.argv[1:]))
    except (Exception, KeyboardInterrupt) as exc:
        # No provider exception text or environment content crosses this boundary.
        code = exc.code if isinstance(exc, RuntimeFault) else "runtime_failed"
        if "--acp-child" not in sys.argv:
            emit({"type": "failed", "code": code, "agent_parent_stopped": False})
        raise SystemExit(1)
