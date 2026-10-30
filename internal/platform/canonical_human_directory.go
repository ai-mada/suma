package platform

// A canonical human is an active project resource whose JSON string user_id
// agrees with the canonical mapping and an existing current account. These
// ordinary SELECT predicates run under the caller's organization authority
// serialization; they never acquire a new recipient identity lock.
func (s *Service) canonicalHumanUserPredicate() string {
	switch s.Dialect {
	case "postgres":
		return `(jsonb_typeof(CAST(m.data AS JSONB)->'user_id')='string' AND CAST(m.data AS JSONB)->>'user_id'=h.user_id)`
	case "oceanbase":
		return `(CASE WHEN JSON_VALID(m.data) THEN CASE WHEN JSON_TYPE(JSON_EXTRACT(m.data,'$.user_id'))='STRING' THEN JSON_UNQUOTE(JSON_EXTRACT(m.data,'$.user_id')) ELSE NULL END ELSE NULL END=h.user_id)`
	default:
		return `(CASE WHEN json_valid(m.data) THEN CASE WHEN json_type(m.data,'$.user_id')='text' THEN json_extract(m.data,'$.user_id') ELSE NULL END ELSE NULL END=h.user_id)`
	}
}

func (s *Service) canonicalHumanTargetsWhere(tenant, project string, approve bool) (string, []any) {
	roles := `('admin','reviewer','developer','viewer')`
	if approve {
		roles = `('admin','reviewer')`
	}
	where := ` FROM suma_members m JOIN suma_human_resources h ON h.tenant_id=m.tenant_id AND h.project_id=m.project_id AND h.resource_id=m.id JOIN suma_users u ON u.id=h.user_id JOIN suma_org_members om ON om.tenant_id=h.tenant_id AND om.user_id=h.user_id LEFT JOIN suma_project_members pm ON pm.project_id=h.project_id AND pm.user_id=h.user_id WHERE m.tenant_id=? AND m.project_id=? AND m.kind='human' AND m.status='active' AND om.role IN ('owner','admin','member') AND (om.role IN ('owner','admin') OR pm.role IN ` + roles + `) AND ` + s.canonicalHumanUserPredicate()
	return where, []any{tenant, project}
}
