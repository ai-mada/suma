"""Frozen common model contract and attempt-local Go gateway capability.

Hashing follows Go encoding/json's struct order and HTML escaping. Supplier
credentials and real endpoints are deliberately absent from the local envelope.
"""

from __future__ import annotations

import hashlib
import json
import re
from urllib.parse import urlsplit

from .contract import RuntimeFault, closed, safe_id

MAX_SAFE = 9007199254740991


def model_json(value: object) -> str:
    text = json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)
    for char in "<>&\u2028\u2029":
        text = text.replace(char, "\\u" + format(ord(char), "04x"))
    return text


def model_hash(value: object) -> str:
    return hashlib.sha256(model_json(value).encode("utf-8")).hexdigest()


def _number(value, minimum=0, maximum=MAX_SAFE):
    if type(value) is not int or not minimum <= value <= maximum:
        raise RuntimeFault("model_binding_invalid")
    return value


def _text(value, maximum=256):
    if not safe_id(value) or value.strip() != value or len(value.encode("utf-8")) > maximum:
        raise RuntimeFault("model_binding_invalid")
    return value


def _id(value):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,99}", value):
        raise RuntimeFault("model_binding_invalid")
    return value


def _sha(value):
    if not isinstance(value, str) or not re.fullmatch(r"[a-f0-9]{64}", value):
        raise RuntimeFault("model_binding_invalid")
    return value


def validate_binding(value: object) -> dict:
    raw = closed(value, {"protocol", "deployment", "parameters", "parameters_sha256", "binding_sha256", "input_token_reservation", "output_token_reservation", "budget", "rate"}, {"protocol", "deployment", "parameters", "parameters_sha256", "binding_sha256", "input_token_reservation", "output_token_reservation"})
    d = closed(raw["deployment"], {"deployment_id", "revision", "deployment_sha256", "protocol", "model", "endpoint_origin", "endpoint_config_sha256", "context_window"})
    deployment = {"deployment_id": _id(d["deployment_id"]), "revision": _number(d["revision"], 1, 2147483647), "deployment_sha256": _sha(d["deployment_sha256"]), "protocol": _text(d["protocol"]), "model": _text(d["model"]), "endpoint_origin": d["endpoint_origin"], "endpoint_config_sha256": _sha(d["endpoint_config_sha256"]), "context_window": _number(d["context_window"], 1, 10000000)}
    origin = deployment["endpoint_origin"]
    if not isinstance(origin, str) or not origin or len(origin.encode("utf-8")) > 2048 or origin.strip() != origin:
        raise RuntimeFault("model_binding_invalid")
    try:
        url = urlsplit(origin)
        _ = url.port
        if url.username or url.password or url.query or url.fragment or url.path or not url.hostname or url.scheme not in {"http", "https"} or origin != url.scheme + "://" + url.netloc or (url.scheme == "http" and url.hostname not in {"127.0.0.1", "localhost", "::1"}):
            raise ValueError()
    except ValueError as exc:
        raise RuntimeFault("model_binding_invalid") from exc
    if deployment["protocol"] != "openai-chat-completions-v1" or model_hash({**deployment, "deployment_sha256": ""}) != deployment["deployment_sha256"]:
        raise RuntimeFault("model_binding_invalid")
    p = closed(raw["parameters"], {"max_steps", "max_output_tokens"})
    parameters = {"max_steps": _number(p["max_steps"], 1, 128), "max_output_tokens": _number(p["max_output_tokens"], 1, 131072)}
    result = {"protocol": _text(raw["protocol"]), "deployment": deployment, "parameters": parameters, "parameters_sha256": _sha(raw["parameters_sha256"]), "binding_sha256": _sha(raw["binding_sha256"]), "input_token_reservation": _number(raw["input_token_reservation"], 1), "output_token_reservation": _number(raw["output_token_reservation"], 1)}
    if "budget" in raw:
        b = closed(raw["budget"], {"token_limit", "cost_limit_micros", "currency"}, set())
        budget = {}
        if "token_limit" in b:
            budget["token_limit"] = _number(b["token_limit"])
        if "cost_limit_micros" in b:
            budget["cost_limit_micros"] = _number(b["cost_limit_micros"])
            if b.get("currency") not in {"CNY", "USD"}:
                raise RuntimeFault("model_binding_invalid")
            budget["currency"] = b["currency"]
        elif "currency" in b:
            raise RuntimeFault("model_binding_invalid")
        if not budget:
            raise RuntimeFault("model_binding_invalid")
        result["budget"] = budget
    if "rate" in raw:
        r = closed(raw["rate"], {"id", "version", "currency", "input_rate_micros_per_million", "output_rate_micros_per_million"})
        if r["currency"] not in {"CNY", "USD"}:
            raise RuntimeFault("model_binding_invalid")
        result["rate"] = {"id": _id(r["id"]), "version": _number(r["version"], 1), "currency": r["currency"], "input_rate_micros_per_million": _number(r["input_rate_micros_per_million"]), "output_rate_micros_per_million": _number(r["output_rate_micros_per_million"])}
    if result["protocol"] != "suma-model-execution-v1" or result["parameters_sha256"] != model_hash(parameters) or result["binding_sha256"] != model_hash({**result, "binding_sha256": ""}) or result["input_token_reservation"] != deployment["context_window"] * parameters["max_steps"] or result["output_token_reservation"] != parameters["max_output_tokens"] or result["input_token_reservation"] + result["output_token_reservation"] > MAX_SAFE:
        raise RuntimeFault("model_binding_invalid")
    budget = result.get("budget", {})
    if "token_limit" in budget and result["input_token_reservation"] + result["output_token_reservation"] > budget["token_limit"]:
        raise RuntimeFault("model_binding_invalid")
    rate = result.get("rate")
    reserved_cost = None
    if rate:
        reserved_cost = (result["input_token_reservation"] * rate["input_rate_micros_per_million"] + result["output_token_reservation"] * rate["output_rate_micros_per_million"] + 999999) // 1000000
        if reserved_cost > MAX_SAFE:
            raise RuntimeFault("model_binding_invalid")
    if "cost_limit_micros" in budget:
        if not rate or rate["currency"] != budget["currency"] or reserved_cost > budget["cost_limit_micros"]:
            raise RuntimeFault("model_binding_invalid")
    return result


def resolve_execution(request: dict, local: object) -> dict:
    try:
        raw = closed(local, {"binding", "gateway"})
        selected = validate_binding(request["model_execution"])
        approved = validate_binding(raw["binding"])
        gateway = closed(raw["gateway"], {"protocol", "base_url", "token", "request_id", "binding_sha256"})
        endpoint, token = gateway["base_url"], gateway["token"]
        if not isinstance(endpoint, str) or not re.fullmatch(r"http://127\.0\.0\.1:[1-9][0-9]*/v1", endpoint) or not 1 <= (urlsplit(endpoint).port or 0) <= 65535 or not isinstance(token, str) or not re.fullmatch(r"[A-Za-z0-9._~-]{16,512}", token) or gateway["protocol"] != "suma-model-gateway-v1" or gateway["request_id"] != request["request_id"] or gateway["binding_sha256"] != selected["binding_sha256"] or model_json(selected) != model_json(approved):
            raise RuntimeFault("model_binding_invalid")
        return {"binding": selected, "gateway": dict(gateway)}
    except (KeyError, ValueError, TypeError, UnicodeError, RuntimeFault) as exc:
        raise RuntimeFault("model_binding_invalid") from exc


def model_execution_capability() -> dict:
    return {"protocol": "suma-model-execution-v1", "provider_protocol": "openai-chat-completions-v1", "parameters": ["max_steps", "max_output_tokens"]}
