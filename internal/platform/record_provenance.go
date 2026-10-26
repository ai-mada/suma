package platform

// Protected code inputs and immutable adoption proof are domain-owned. Generic
// record writes must not manufacture them through either top-level fields or Data.
func rejectProtectedRecordProvenanceInput(body map[string]any) error {
	// Repair lineage is bound by the protected composition domain and cannot be
	// manufactured as ordinary task metadata or raw execution configuration.
	for _, name := range []string{"candidate_conflict_repair", "conflict_repair", "conflict_basis"} {
		if _, present := body[name]; present {
			return invalid(compositionInvalid)
		}
		if data, ok := body["data"].(map[string]any); ok {
			if _, present := data[name]; present {
				return invalid(compositionInvalid)
			}
		}
	}
	if _, present := body["code_baseline"]; present {
		return invalid(compositionInvalid)
	}
	if _, present := body["failure_diagnosis_adoption"]; present {
		return invalid(diagnosisInvalid)
	}
	if data, ok := body["data"].(map[string]any); ok {
		if _, present := data["code_baseline"]; present {
			return invalid(compositionInvalid)
		}
		if _, present := data["failure_diagnosis_adoption"]; present {
			return invalid(diagnosisInvalid)
		}
	}
	return nil
}
