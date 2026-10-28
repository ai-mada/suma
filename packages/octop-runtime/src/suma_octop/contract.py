"""Closed carrier contracts. Provider output never supplies trusted identities."""

from __future__ import annotations

import hashlib
import json
import re
import unicodedata
from pathlib import Path
from urllib.parse import urlsplit

TOOLS = ("list", "read", "write", "edit", "remove", "check")
TOOL_NAMES = tuple("suma_workspace_" + name for name in TOOLS)


class RuntimeFault(RuntimeError):
    """Only the fixed code crosses the carrier; upstream text may contain secrets."""

    def __init__(self, code: str):
        self.code = code
        super().__init__(code)


def safe_id(value: object) -> bool:
    return isinstance(value, str) and 0 < len(value) <= 256 and not any(
        unicodedata.category(c) in {"Cc", "Cs"} for c in value
    )


def closed(value: object, keys: set[str], required: set[str] | None = None) -> dict:
    if not isinstance(value, dict) or set(value) - keys or not (required or keys) <= set(value):
        raise RuntimeFault("request_invalid")
    return value


def parse_json(raw: str | bytes) -> object:
    """Reject duplicate members and NaN rather than normalizing hostile envelopes."""
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise RuntimeFault("request_invalid")
            result[key] = value
        return result
    try:
        value = json.loads(raw, object_pairs_hook=pairs, parse_constant=lambda _: (_ for _ in ()).throw(RuntimeFault("request_invalid")))
        def valid(item, depth=0):
            if depth > 64:
                raise RuntimeFault("request_invalid")
            if isinstance(item, str) and any(unicodedata.category(c) == "Cs" for c in item):
                raise RuntimeFault("request_invalid")
            if isinstance(item, dict):
                for key, child in item.items():
                    valid(key, depth + 1)
                    valid(child, depth + 1)
            elif isinstance(item, list):
                for child in item:
                    valid(child, depth + 1)
        valid(value)
        return value
    except (ValueError, UnicodeError) as exc:
        raise RuntimeFault("request_invalid") from exc


def parse_execute(value: object) -> dict:
    request = closed(value, {"v", "type", "request_id", "goal", "workspace_bridge"})
    if type(request["v"]) is not int or request["v"] != 1 or request["type"] != "execute" or not safe_id(request["request_id"]):
        raise RuntimeFault("request_invalid")
    goal = request["goal"]
    if not isinstance(goal, str) or not goal.strip() or len(goal.encode("utf-8")) > 262144:
        raise RuntimeFault("request_invalid")
    bridge = closed(request["workspace_bridge"], {"version", "endpoint", "token", "policy_sha256", "request_id"})
    if type(bridge["version"]) is not int or bridge["version"] != 1 or bridge["request_id"] != request["request_id"]:
        raise RuntimeFault("workspace_config")
    if not isinstance(bridge["token"], str) or not re.fullmatch(r"[A-Za-z0-9._~-]{16,512}", bridge["token"]):
        raise RuntimeFault("workspace_config")
    if not isinstance(bridge["policy_sha256"], str) or not re.fullmatch(r"[a-f0-9]{64}", bridge["policy_sha256"]):
        raise RuntimeFault("workspace_config")
    try:
        url = urlsplit(bridge["endpoint"])
        if url.scheme != "http" or url.hostname != "127.0.0.1" or not url.port or url.username or url.password or url.query or url.fragment:
            raise ValueError()
    except (TypeError, ValueError) as exc:
        raise RuntimeFault("workspace_config") from exc
    return request


def parse_config(value: object) -> dict:
    keys = {"home", "provider_id", "model", "base_url", "configuration_sha256", "max_steps", "max_output_tokens", "api_key_env_ref"}
    config = closed(value, keys, keys - {"api_key_env_ref"})
    if not isinstance(config["home"], str) or not Path(config["home"]).is_absolute():
        raise RuntimeFault("configuration_invalid")
    if not safe_id(config["provider_id"]) or not re.fullmatch(r"[A-Za-z0-9_-]{1,64}", config["provider_id"]) or not safe_id(config["model"]):
        raise RuntimeFault("configuration_invalid")
    if not isinstance(config["configuration_sha256"], str) or not re.fullmatch(r"[a-f0-9]{64}", config["configuration_sha256"]):
        raise RuntimeFault("configuration_invalid")
    for key, maximum in (("max_steps", 128), ("max_output_tokens", 131072)):
        if type(config[key]) is not int or not 1 <= config[key] <= maximum:
            raise RuntimeFault("configuration_invalid")
    if "api_key_env_ref" in config:
        ref = config["api_key_env_ref"]
        reserved = {"HOME", "USERPROFILE", "PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL", "TEMP", "TMP", "VIRTUAL_ENV", "SUMA_OCTOP_CONFIG", "SUMA_OCTOP_REQUEST", "OCTOP_HOME"}
        if not isinstance(ref, str) or not re.fullmatch(r"[A-Z][A-Z0-9_]{1,127}", ref) or ref in reserved or ref.startswith(("PYTHON", "LD_", "DYLD_", "SUMA_", "LANGCHAIN", "LANGSMITH", "HF_", "OTEL_")):
            raise RuntimeFault("configuration_invalid")
    try:
        url = urlsplit(config["base_url"])
        if url.username or url.password or url.query or url.fragment or not url.hostname or url.scheme not in {"https", "http"} or (url.scheme == "http" and url.hostname not in {"127.0.0.1", "localhost", "::1"}):
            raise ValueError()
    except (TypeError, ValueError) as exc:
        raise RuntimeFault("configuration_invalid") from exc
    return config


def operation_id(request_id: str, tool_call_id: str, tool_name: str) -> str:
    if not safe_id(request_id) or not safe_id(tool_call_id) or tool_name not in TOOL_NAMES:
        raise RuntimeFault("workspace_identity")
    raw = json.dumps([request_id, tool_call_id, tool_name], ensure_ascii=False, separators=(",", ":")).encode()
    return hashlib.sha256(raw).hexdigest()


def workspace_schemas() -> dict[str, dict]:
    """Pinned attempt-workspace-v1, matching the Go host's complete JSON schemas."""
    def text(maximum):
        return {"type": "string", "minLength": 1, "maxLength": maximum}
    def integer(maximum):
        return {"type": "integer", "minimum": 1, "maximum": maximum}
    def obj(properties, *required):
        result = {"type": "object", "properties": properties, "additionalProperties": False}
        if required:
            result["required"] = list(required)
        return result
    limit = 1 << 20
    sha = {"type": "string", "pattern": "^[0-9a-f]{64}$", "minLength": 64, "maxLength": 64}
    write = obj({"path": text(1024), "content": {"type": "string", "maxLength": limit}, "expected_sha256": sha, "expected_absent": {"type": "boolean", "const": True}}, "path", "content")
    write["oneOf"] = [{"required": ["expected_sha256"], "not": {"required": ["expected_absent"]}}, {"required": ["expected_absent"], "not": {"required": ["expected_sha256"]}}]
    return {
        "list": obj({"directory": text(1024), "limit": integer(200), "cursor": text(4096)}),
        "read": obj({"path": text(1024), "start_line": integer(limit + 1), "max_lines": integer(1000)}, "path"),
        "write": write,
        "edit": obj({"path": text(1024), "expected_sha256": sha, "edits": {"type": "array", "minItems": 1, "maxItems": 32, "items": obj({"old_text": text(limit), "new_text": {"type": "string", "maxLength": limit}}, "old_text", "new_text")}}, "path", "expected_sha256", "edits"),
        "remove": obj({"path": text(1024), "expected_sha256": sha}, "path", "expected_sha256"),
        "check": obj({"check_id": text(100)}, "check_id"),
    }


def verify_catalog(result: object) -> dict[str, dict]:
    if not isinstance(result, dict) or result.get("nextCursor") or not isinstance(result.get("tools"), list):
        raise RuntimeFault("workspace_catalog")
    expected = {"suma_workspace_" + name: schema for name, schema in workspace_schemas().items()}
    found = {}
    for item in result["tools"]:
        if not isinstance(item, dict) or item.get("name") not in expected or item["name"] in found or item.get("inputSchema") != expected[item["name"]]:
            raise RuntimeFault("workspace_catalog")
        found[item["name"]] = item
    if set(found) != set(expected):
        raise RuntimeFault("workspace_catalog")
    return found
