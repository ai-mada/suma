"""Only new gateway semantics; no provider call or SDK construction is needed."""

import copy
import unittest
from types import SimpleNamespace

from suma_octop.contract import RuntimeFault, TOOL_NAMES, parse_config
from suma_octop.model import model_hash, resolve_execution, validate_binding
from suma_octop.policy import WorkspaceOnlyMiddleware


def binding():
    deployment = {"deployment_id": "model-a", "revision": 1, "deployment_sha256": "", "protocol": "openai-chat-completions-v1", "model": "vendor/model-中文<&>", "endpoint_origin": "https://provider.example", "endpoint_config_sha256": "a" * 64, "context_window": 8192}
    deployment["deployment_sha256"] = model_hash(deployment)
    parameters = {"max_steps": 2, "max_output_tokens": 128}
    value = {"protocol": "suma-model-execution-v1", "deployment": deployment, "parameters": parameters, "parameters_sha256": model_hash(parameters), "binding_sha256": "", "input_token_reservation": 16384, "output_token_reservation": 128}
    value["binding_sha256"] = model_hash(value)
    return value


class ModelTests(unittest.TestCase):
    def test_binding_and_gateway_have_one_frozen_identity(self):
        selected = binding()
        request = {"request_id": "request", "model_execution": selected}
        local = {"binding": selected, "gateway": {"protocol": "suma-model-gateway-v1", "base_url": "http://127.0.0.1:4311/v1", "token": "attempt-token-only-0123456789", "request_id": "request", "binding_sha256": selected["binding_sha256"]}}
        self.assertEqual(validate_binding(selected), selected)
        self.assertEqual(resolve_execution(request, local), local)
        for key, invalid in (("base_url", "https://provider.example/v1"), ("request_id", "different"), ("binding_sha256", "b" * 64)):
            altered = copy.deepcopy(local)
            altered["gateway"][key] = invalid
            with self.assertRaises(RuntimeFault):
                resolve_execution(request, altered)
        with self.assertRaises(RuntimeFault):
            resolve_execution(request, {"binding": selected, "base_url": "https://provider.example/v1", "api_key": "old-secret"})

    def test_config_has_no_supplier_or_model_override(self):
        with self.assertRaises(RuntimeFault):
            parse_config({"home": "unused", "provider_id": "supplier", "api_key_env_ref": "OLD_KEY"})

    def test_model_step_header_is_fixed_and_monotonic(self):
        policy = WorkspaceOnlyMiddleware(2, 128)
        class Request(SimpleNamespace):
            def override(self, **fields):
                return Request(**{**vars(self), **fields})
        request = Request(tools=[SimpleNamespace(name=name) for name in TOOL_NAMES], model_settings={"max_tokens": 999, "extra_headers": {"X-Suma-Model-Step": "99"}})
        first = policy._model_request(request)
        second = policy._model_request(request)
        self.assertEqual(first.model_settings["max_tokens"], 128)
        self.assertEqual(first.model_settings["extra_headers"], {"X-Suma-Model-Step": "0"})
        self.assertEqual(second.model_settings["extra_headers"], {"X-Suma-Model-Step": "1"})
        with self.assertRaises(RuntimeFault):
            policy._model_request(request)


if __name__ == "__main__":
    unittest.main()
