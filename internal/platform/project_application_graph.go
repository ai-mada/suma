package platform

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

const applicationGraphMaxNodes = 1000
const applicationGraphMaxEdges = 5000
const applicationImpactMaxDepth = 64

// This small transactional projection is only a graph read model. The complete
// immutable version remains the authority for source, quality and registration.
// Graph reads never pull thousands of full source/manifest/evidence snapshots.
type applicationVersionGraphProjection struct {
	ProjectID                  string                  `json:"project_id"`
	ApplicationID              string                  `json:"application_id"`
	VersionID                  string                  `json:"version_id"`
	VersionSHA256              string                  `json:"version_sha256"`
	Label                      string                  `json:"label"`
	Dependencies               []ApplicationDependency `json:"dependencies"`
	DependencyCoverageComplete bool                    `json:"dependency_coverage_complete"`
}
type applicationGraphRecord struct {
	app     ProjectApplication
	version *applicationVersionGraphProjection
}
type applicationGraphVersionRef struct{ app, sha string }

func (s *Service) InitializeApplicationGraph(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_application_graph_projections(version_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,application_id VARCHAR(64) NOT NULL,snapshot_sha256 VARCHAR(64) NOT NULL,projection_sha256 VARCHAR(64) NOT NULL,payload TEXT NOT NULL)`))
	return err
}

func (s *Service) saveApplicationGraphProjectionTx(ctx context.Context, auth *authorizedCommand, v ApplicationCodeVersion) error {
	if err := assertProjectCommand(auth.tx, auth, v.ProjectID); err != nil {
		return err
	}
	if v.ID == "" || v.ApplicationID == "" || v.Dependencies == nil || len(v.Dependencies) > 64 || applicationVersionDigest(v) != v.SnapshotSHA256 {
		return conflict(applicationChanged)
	}
	p := applicationVersionGraphProjection{ProjectID: v.ProjectID, ApplicationID: v.ApplicationID, VersionID: v.ID, VersionSHA256: v.SnapshotSHA256, Label: v.Label, Dependencies: v.Dependencies, DependencyCoverageComplete: v.DependencyCoverageComplete}
	raw := stringify(p)
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_application_graph_projections(version_id,tenant_id,project_id,application_id,snapshot_sha256,projection_sha256,payload)VALUES(?,?,?,?,?,?,?)`), v.ID, auth.target.tenantID, v.ProjectID, v.ApplicationID, v.SnapshotSHA256, digest(raw), raw)
	return err
}

func (s *Service) applicationGraphRecordsTx(ctx context.Context, auth *authorizedCommand, project string) ([]applicationGraphRecord, int, error) {
	out := []applicationGraphRecord{}
	if err := assertProjectRead(auth, project); err != nil {
		return out, 0, err
	}
	var total int
	if err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_applications WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&total); err != nil {
		return out, 0, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT a.id,a.version,a.app_key,a.state,a.parent_id,a.owner_team_id,a.current_version_id,a.payload,COALESCE(v.id,''),COALESCE(v.label,''),COALESCE(v.snapshot_sha256,''),COALESCE(g.application_id,''),COALESCE(g.snapshot_sha256,''),COALESCE(g.projection_sha256,''),COALESCE(g.payload,'') FROM suma_project_applications a LEFT JOIN suma_application_versions v ON v.tenant_id=a.tenant_id AND v.project_id=a.project_id AND v.application_id=a.id AND v.id=a.current_version_id LEFT JOIN suma_application_graph_projections g ON g.tenant_id=a.tenant_id AND g.project_id=a.project_id AND g.application_id=a.id AND g.version_id=a.current_version_id WHERE a.tenant_id=? AND a.project_id=? ORDER BY a.id LIMIT ?`), auth.target.tenantID, project, applicationGraphMaxNodes)
	if err != nil {
		return out, total, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key, state, parent, team, current, raw, vid, label, vsha, gapp, gsha, psha, payload string
		var version int
		if err = rows.Scan(&id, &version, &key, &state, &parent, &team, &current, &raw, &vid, &label, &vsha, &gapp, &gsha, &psha, &payload); err != nil {
			return out, total, err
		}
		app, e := decodeProjectApplication(raw, project, id, version, key, state, parent, team, current)
		if e != nil {
			return out, total, e
		}
		record := applicationGraphRecord{app: app}
		if current != "" {
			var projection applicationVersionGraphProjection
			if current != vid || gapp != id || gsha != vsha || digest(payload) != psha || json.Unmarshal([]byte(payload), &projection) != nil || projection.ProjectID != project || projection.ApplicationID != id || projection.VersionID != vid || projection.VersionSHA256 != vsha || projection.Label != label || projection.Dependencies == nil || len(projection.Dependencies) > 64 {
				return out, total, conflict(applicationChanged)
			}
			record.version = &projection
		}
		out = append(out, record)
	}
	return out, total, rows.Err()
}

func (s *Service) applicationGraphVersionRefsTx(ctx context.Context, auth *authorizedCommand, project string, records []applicationGraphRecord) (map[string]applicationGraphVersionRef, error) {
	refs := map[string]applicationGraphVersionRef{}
	ids := []string{}
	seen := map[string]bool{}
	for _, record := range records {
		if record.version == nil {
			continue
		}
		for _, dep := range record.version.Dependencies {
			if !seen[dep.TargetVersionID] {
				seen[dep.TargetVersionID] = true
				ids = append(ids, dep.TargetVersionID)
			}
			if len(ids) >= applicationGraphMaxEdges {
				break
			}
		}
		if len(ids) >= applicationGraphMaxEdges {
			break
		}
	}
	if len(ids) == 0 {
		return refs, nil
	}
	sort.Strings(ids)
	args := []any{auth.target.tenantID, project}
	for _, id := range ids {
		args = append(args, id)
	}
	query := `SELECT id,application_id,snapshot_sha256 FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + `)`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return refs, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, app, sha string
		if err = rows.Scan(&id, &app, &sha); err != nil {
			return refs, err
		}
		refs[id] = applicationGraphVersionRef{app: app, sha: sha}
	}
	return refs, rows.Err()
}

func buildApplicationGraph(project string, records []applicationGraphRecord, total int, refs map[string]applicationGraphVersionRef) ApplicationDependencyGraph {
	out := ApplicationDependencyGraph{ProjectID: project, Nodes: []ApplicationGraphNode{}, Edges: []ApplicationGraphEdge{}, Cycles: [][]string{}, ApplicationTotal: total, EdgeTotalExact: total == len(records), Complete: total == len(records), MaxApplications: applicationGraphMaxNodes, MaxEdges: applicationGraphMaxEdges}
	index := map[string]applicationGraphRecord{}
	for _, record := range records {
		index[record.app.ID] = record
		app := record.app
		node := ApplicationGraphNode{ApplicationID: app.ID, Name: app.Name, Kind: app.Kind, State: app.State, ParentApplicationID: app.ParentApplicationID, OwnerTeamID: app.OwnerTeamID}
		if record.version != nil {
			node.VersionID, node.VersionLabel, node.DependencyCoverageComplete = record.version.VersionID, record.version.Label, record.version.DependencyCoverageComplete
		}
		out.Nodes = append(out.Nodes, node)
	}
	for _, record := range records {
		if record.version == nil {
			continue
		}
		for _, dep := range record.version.Dependencies {
			out.EdgeTotal++
			if len(out.Edges) >= applicationGraphMaxEdges {
				out.Complete = false
				continue
			}
			state := "current"
			target, ok := index[dep.TargetApplicationID]
			ref, exists := refs[dep.TargetVersionID]
			if !ok {
				if total > len(records) {
					state = "outside_loaded_scope"
				} else {
					state = "missing_application"
				}
			} else if !exists || ref.app != dep.TargetApplicationID || ref.sha != dep.TargetVersionSHA256 {
				state = "missing_version"
			} else if target.version == nil || target.version.VersionID != dep.TargetVersionID {
				state = "historical_version"
			}
			out.Edges = append(out.Edges, ApplicationGraphEdge{SourceApplicationID: record.app.ID, SourceVersionID: record.version.VersionID, TargetApplicationID: dep.TargetApplicationID, TargetVersionID: dep.TargetVersionID, TargetVersionSHA256: dep.TargetVersionSHA256, ObservationID: dep.ObservationID, State: state})
		}
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ApplicationID < out.Nodes[j].ApplicationID })
	sort.Slice(out.Edges, func(i, j int) bool { return stringify(out.Edges[i]) < stringify(out.Edges[j]) })
	out.Cycles = applicationGraphCycles(out.Nodes, out.Edges)
	out.SHA256 = digest(stringify(out))
	return out
}

// Cycles are strongly connected groups, not fabricated ordered paths. This is
// O(V+E), preserving real application cycles instead of forcing them into DAGs.
func applicationGraphCycles(nodes []ApplicationGraphNode, edges []ApplicationGraphEdge) [][]string {
	adj := map[string][]string{}
	self := map[string]bool{}
	known := map[string]bool{}
	for _, node := range nodes {
		known[node.ApplicationID] = true
	}
	for _, edge := range edges {
		if known[edge.SourceApplicationID] && known[edge.TargetApplicationID] {
			adj[edge.SourceApplicationID] = append(adj[edge.SourceApplicationID], edge.TargetApplicationID)
			if edge.SourceApplicationID == edge.TargetApplicationID {
				self[edge.SourceApplicationID] = true
			}
		}
	}
	for id := range adj {
		sort.Strings(adj[id])
	}
	indices, low, onStack := map[string]int{}, map[string]int{}, map[string]bool{}
	stack := []string{}
	next := 0
	groups := [][]string{}
	var visit func(string)
	visit = func(id string) {
		next++
		indices[id], low[id] = next, next
		stack = append(stack, id)
		onStack[id] = true
		for _, to := range adj[id] {
			if indices[to] == 0 {
				visit(to)
				low[id] = min(low[id], low[to])
			} else if onStack[to] {
				low[id] = min(low[id], indices[to])
			}
		}
		if low[id] == indices[id] {
			group := []string{}
			for len(stack) > 0 {
				last := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[last] = false
				group = append(group, last)
				if last == id {
					break
				}
			}
			if len(group) > 1 || self[id] {
				sort.Strings(group)
				groups = append(groups, group)
			}
		}
	}
	for _, node := range nodes {
		if indices[node.ApplicationID] == 0 {
			visit(node.ApplicationID)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return strings.Join(groups[i], "\x00") < strings.Join(groups[j], "\x00") })
	return groups
}

func applicationGraphImpact(graph ApplicationDependencyGraph, id string) (ApplicationImpact, error) {
	out := ApplicationImpact{Graph: graph, ApplicationID: id, Items: []ApplicationImpactItem{}, Complete: graph.Complete, MaxDepth: applicationImpactMaxDepth}
	nodes := map[string]ApplicationGraphNode{}
	reverse := map[string][]string{}
	for _, node := range graph.Nodes {
		nodes[node.ApplicationID] = node
		if !node.DependencyCoverageComplete {
			out.Complete = false
		}
	}
	if _, exists := nodes[id]; !exists {
		return out, missing()
	}
	for _, edge := range graph.Edges {
		if edge.State != "current" && edge.State != "historical_version" {
			out.Complete = false
		}
		if _, ok := nodes[edge.SourceApplicationID]; ok {
			reverse[edge.TargetApplicationID] = append(reverse[edge.TargetApplicationID], edge.SourceApplicationID)
		}
	}
	for target := range reverse {
		sort.Strings(reverse[target])
	}
	type queued struct {
		id   string
		path []string
	}
	queue := []queued{{id: id, path: []string{id}}}
	seen := map[string]bool{id: true}
	for head := 0; head < len(queue); head++ {
		item := queue[head]
		for _, dependent := range reverse[item.id] {
			if seen[dependent] {
				continue
			}
			if len(item.path) > applicationImpactMaxDepth {
				out.Complete = false
				continue
			}
			seen[dependent] = true
			chain := append(append([]string{}, item.path...), dependent)
			node := nodes[dependent]
			out.Items = append(out.Items, ApplicationImpactItem{ApplicationID: dependent, VersionID: node.VersionID, OwnerTeamID: node.OwnerTeamID, Depth: len(chain) - 1, Path: chain})
			queue = append(queue, queued{id: dependent, path: chain})
		}
	}
	return out, nil
}

func (s *Service) applicationGraphTx(ctx context.Context, auth *authorizedCommand, project string) (ApplicationDependencyGraph, error) {
	records, total, err := s.applicationGraphRecordsTx(ctx, auth, project)
	if err != nil {
		return ApplicationDependencyGraph{}, err
	}
	refs, err := s.applicationGraphVersionRefsTx(ctx, auth, project, records)
	if err != nil {
		return ApplicationDependencyGraph{}, err
	}
	return buildApplicationGraph(project, records, total, refs), nil
}
func (s *Service) GetApplicationGraph(ctx context.Context, actor Actor, project string) (ApplicationDependencyGraph, error) {
	if !testPlanActor(actor) {
		return ApplicationDependencyGraph{}, forbidden()
	}
	if !handoffID(project, false) {
		return ApplicationDependencyGraph{}, invalid(applicationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return ApplicationDependencyGraph{}, err
	}
	defer close()
	return s.applicationGraphTx(ctx, auth, project)
}
func (s *Service) GetApplicationImpact(ctx context.Context, actor Actor, project string, request ApplicationImpactRequest) (ApplicationImpact, error) {
	if !testPlanActor(actor) {
		return ApplicationImpact{}, forbidden()
	}
	if !handoffID(project, false) || request.ProjectID != project || !handoffID(request.ApplicationID, false) || !contentDigest.MatchString(request.ExpectedGraphSHA256) {
		return ApplicationImpact{}, invalid(applicationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, request.ApplicationID, purposeProjectRead)
	if err != nil {
		return ApplicationImpact{}, err
	}
	defer close()
	graph, err := s.applicationGraphTx(ctx, auth, project)
	if err != nil {
		return ApplicationImpact{}, err
	}
	if graph.SHA256 != request.ExpectedGraphSHA256 {
		return ApplicationImpact{}, conflict(applicationChanged)
	}
	return applicationGraphImpact(graph, request.ApplicationID)
}
