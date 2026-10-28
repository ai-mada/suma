"""Focused invariants; no provider, user account or external service is contacted."""

import unittest
from suma_octop.contract import RuntimeFault, operation_id, parse_execute, parse_json, verify_catalog, workspace_schemas


class ContractTests(unittest.TestCase):
    def test_duplicate_members_are_rejected(self):
        with self.assertRaises(RuntimeFault):
            parse_json('{"v":1,"v":2}')
        with self.assertRaises(RuntimeFault):
            parse_json('"\\ud800"')

    def test_complete_catalog_not_just_field_names(self):
        catalog = {"tools": [{"name": "suma_workspace_" + name, "inputSchema": schema} for name, schema in workspace_schemas().items()]}
        self.assertEqual(len(verify_catalog(catalog)), 6)
        catalog["tools"][0]["inputSchema"]["additionalProperties"] = True
        with self.assertRaises(RuntimeFault):
            verify_catalog(catalog)

    def test_operation_identity_is_request_and_real_call_bound(self):
        first = operation_id("request", "provider-call", "suma_workspace_write")
        self.assertEqual(first, operation_id("request", "provider-call", "suma_workspace_write"))
        self.assertNotEqual(first, operation_id("request2", "provider-call", "suma_workspace_write"))
        with self.assertRaises(RuntimeFault):
            operation_id("request", "provider-call", "execute")

    def test_execute_cannot_supply_arbitrary_mcp_server(self):
        value = {"v": 1, "type": "execute", "request_id": "r", "goal": "code", "workspace_bridge": {"version": 1, "endpoint": "https://example.com/mcp", "token": "x" * 32, "policy_sha256": "a" * 64, "request_id": "r"}}
        with self.assertRaises(RuntimeFault):
            parse_execute(value)


if __name__ == "__main__":
    unittest.main()
