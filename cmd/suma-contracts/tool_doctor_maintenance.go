package main

// Keep the maintenance recommendation closed and separate from the existing
// four availability checks. This describes read facts, never a new permission.
func toolDoctorMaintenanceSchemas(schemas map[string]any) {
	model := toolContractMap(schemas, "ToolProfileMaintenanceResolution")
	model["additionalProperties"] = false
	properties := toolContractMap(model, "properties")
	for _, name := range []string{"profile_id", "worker_id"} {
		field := toolContractMap(properties, name)
		field["minLength"], field["maxLength"] = 1, 100
		field["pattern"] = `^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`
	}
	toolContractMap(properties, "profile_version")["minimum"] = 1
	for _, name := range []string{"inspection_id", "recovery_id"} {
		field := toolContractMap(properties, name)
		field["minLength"], field["maxLength"] = 1, 150
		field["pattern"] = `^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$`
	}
	type branch struct {
		action, ids string
		reasons     []string
	}
	branches := []branch{
		{"inspect", "", []string{"maintenance_supported"}},
		{"view_inspection", "inspection", []string{"inspection_pending", "inspection_claimed", "inspection_unknown"}},
		{"recover_inspection", "inspection", []string{"inspection_unknown", "recovery_expired"}},
		{"view_recovery", "both", []string{"recovery_requested", "recovery_proof_stored"}},
		{"open_worker", "", []string{"maintenance_expired", "maintenance_unsupported", "maintenance_unknown", "worker_unavailable"}},
		{"none", "", []string{"profile_revoked"}},
	}
	var shapes []any
	var actions, reasons []string
	for _, item := range branches {
		actions = append(actions, item.action)
		for _, reason := range item.reasons {
			found := false
			for _, existing := range reasons {
				found = found || reason == existing
			}
			if !found {
				reasons = append(reasons, reason)
			}
		}
		fields := map[string]any{"action": map[string]any{"const": item.action}, "reason": map[string]any{"enum": item.reasons}}
		shape := map[string]any{"properties": fields}
		if item.ids == "" {
			fields["inspection_id"], fields["recovery_id"] = false, false
		} else {
			required := []string{"inspection_id"}
			if item.ids == "both" {
				required = append(required, "recovery_id")
			} else {
				fields["recovery_id"] = false
			}
			shape["required"] = required
		}
		shapes = append(shapes, shape)
	}
	model["oneOf"] = shapes
	toolContractMap(properties, "action")["enum"] = actions
	toolContractMap(properties, "reason")["enum"] = reasons
	toolContractMap(schemas, "ToolProfileDiagnostics", "properties", "maintenance_resolution")["description"] = "Current maintenance observation bound to tool.id/version/worker and the enclosing checked_at/expires_at. An original inspection can belong to another profile on the same worker: read it and its original profile before navigation. No GET effects, new probe, repair or permission."
}
