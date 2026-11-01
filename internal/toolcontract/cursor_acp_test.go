package toolcontract

import "testing"

func TestCursorACPProfileCannotBorrowAnotherRuntimeOrLogin(t *testing.T) {
	p := acpTestProfile(t)
	p.CommandID = "cursor-reviewed"
	p.Adapter = "cursor"
	p.ACPExecution.Runtime = "cursor"
	p.ACPExecution.AuthStrategy = "explicit_api_key_env"
	if err := ValidateProfile(p, true); err != nil || !SupportsACPExecutionProfile(p) {
		t.Fatal("reviewed Cursor ACP profile rejected", err)
	}
	wrong := CloneProfile(p)
	wrong.Adapter = "qoder"
	if SupportsACPExecutionProfile(wrong) || ValidateProfile(wrong, true) == nil {
		t.Fatal("a Qoder profile borrowed Cursor configuration")
	}
	wrong = CloneProfile(p)
	wrong.ACPExecution.AuthStrategy = "existing_cli_login"
	if SupportsACPExecutionProfile(wrong) || ValidateProfile(wrong, true) == nil {
		t.Fatal("Cursor silently inherited another CLI login policy")
	}
	wrong = CloneProfile(p)
	wrong.ACPExecution.Runtime = "unknown-agent"
	if SupportsACPExecutionProfile(wrong) || ValidateProfile(wrong, true) == nil {
		t.Fatal("an arbitrary ACP runtime acquired dispatch authority")
	}
	if _, err := DecodeACPNodeConfiguration(map[string]any{"acp_execution": p.ACPExecution, "model": "different"}); err == nil {
		t.Fatal("task input replaced the administrator's fixed model")
	}
}
