import unittest
from types import SimpleNamespace

from suma_octop.policy import WorkspaceOnlyMiddleware
from suma_octop.workspace import CURRENT_TOOL_CALL


class PolicyTests(unittest.IsolatedAsyncioTestCase):
    async def test_hallucinated_builtin_never_reaches_executor(self):
        policy = WorkspaceOnlyMiddleware(2, 64)
        reached = []
        async def execute(request):
            reached.append(request)
        for name in ("execute", "read_file", "write_file", "task", "ask_agent", "web_search", "acp_runner", "suma_workspace_write_extra"):
            result = await policy.awrap_tool_call(SimpleNamespace(tool_call={"id": "call", "name": name}), execute)
            self.assertEqual(result.status, "error")
        self.assertEqual(reached, [])
        self.assertEqual(policy.fatal_code, "workspace_tool_denied")

    async def test_real_call_identity_is_hidden_context_not_model_argument(self):
        policy = WorkspaceOnlyMiddleware(2, 64)
        async def execute(request):
            return CURRENT_TOOL_CALL.get()
        request = SimpleNamespace(tool_call={"id": "actual-id", "name": "suma_workspace_write", "args": {"_meta": {"suma/operation_id": "forged"}}})
        self.assertEqual(await policy.awrap_tool_call(request, execute), ("actual-id", "suma_workspace_write"))
        self.assertIsNone(CURRENT_TOOL_CALL.get())


if __name__ == "__main__":
    unittest.main()
