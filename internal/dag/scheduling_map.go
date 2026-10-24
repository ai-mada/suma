package dag

import (
	"encoding/json"
	"fmt"
	"strings"
)

const mapMaterializationChunk = 128

// The complete immutable manifest commits before workers can observe any child.
// next_index is a durable boundary: a crash either commits a whole chunk and its
// children, or leaves the previous boundary and no part of the next chunk.
func (t *transaction) beginMapExpansion(r *runState, n *nodeState) error {
	items, err := mapItems(n.spec)
	if err != nil {
		return err
	}
	if len(items) > 1000 {
		return fmt.Errorf("%w: map expansion exceeds 1000", ErrInvalid)
	}
	template, err := mapTemplate(n.spec)
	if err != nil {
		return err
	}
	if template.Type == "map" {
		return fmt.Errorf("%w: nested map unsupported", ErrUnsupported)
	}
	var generation int
	if err = t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&generation); err != nil {
		return err
	}
	if err = t.exec(`INSERT INTO dag_map_expansions(run_id,node_id,generation,input_sha256,template_sha256,items,template,total,next_index,state)VALUES(?,?,?,?,?,?,?,?,0,'open')`, r.ID, n.ID, generation, fingerprint(items), fingerprint(template), encoded(items), encoded(template), len(items)); err != nil {
		return err
	}
	if err = t.exec(`UPDATE dag_map_progress SET total=?,terminal_count=0,failed_count=0,closed=0 WHERE run_id=? AND node_id=?`, len(items), r.ID, n.ID); err != nil {
		return err
	}
	if err = t.event(r, n.ID, "map_manifest_saved", map[string]any{"generation": generation, "items": len(items), "input_sha256": fingerprint(items), "template_sha256": fingerprint(template)}); err != nil {
		return err
	}
	if err = t.transition(r, n, "waiting", "map_children"); err != nil {
		return err
	}
	return t.materializeMapChunk(r, n)
}

func (t *transaction) materializeMapChunk(r *runState, n *nodeState) error {
	var generation, total, next int
	var itemsJSON, templateJSON, inputSHA, templateSHA, state string
	if err := t.row(`SELECT generation,input_sha256,template_sha256,items,template,total,next_index,state FROM dag_map_expansions WHERE run_id=? AND node_id=? ORDER BY generation DESC LIMIT 1`, r.ID, n.ID).Scan(&generation, &inputSHA, &templateSHA, &itemsJSON, &templateJSON, &total, &next, &state); err != nil {
		return err
	}
	if state == "closed" {
		return nil
	}
	var items []any
	var template Node
	if json.Unmarshal([]byte(itemsJSON), &items) != nil || json.Unmarshal([]byte(templateJSON), &template) != nil || fingerprint(items) != inputSHA || fingerprint(template) != templateSHA || len(items) != total || next < 0 || next > total {
		return fmt.Errorf("%w: map manifest integrity", ErrConflict)
	}
	end := next + mapMaterializationChunk
	if end > total {
		end = total
	}
	for i := next; i < end; i++ {
		item := items[i]
		spec := template
		spec.ID = fmt.Sprintf("%s[%d]", n.ID, i)
		spec.Name = spec.ID
		spec.DependsOn = nil
		spec.Command = append([]string{}, template.Command...)
		value, ok := item.(string)
		if !ok {
			value = encoded(item)
		}
		for j := range spec.Command {
			spec.Command[j] = strings.ReplaceAll(spec.Command[j], "{{item}}", value)
			spec.Command[j] = strings.ReplaceAll(spec.Command[j], "{{index}}", fmt.Sprint(i))
		}
		spec.Config = map[string]any{}
		for key, value := range template.Config {
			spec.Config[key] = value
		}
		spec.Config["item"] = item
		spec.Config["index"] = i
		child := &nodeState{NodeRun: NodeRun{ID: spec.ID, Name: spec.Name, Type: spec.Type, State: "pending", Command: spec.Command}, spec: spec, parent: n.ID}
		r.nodes = append(r.nodes, child)
		if err := t.insertNode(r, child); err != nil {
			return err
		}
	}
	closed := 0
	state = "open"
	if end == total {
		closed = 1
		state = "closed"
	}
	res, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_map_expansions SET next_index=?,state=? WHERE run_id=? AND node_id=? AND generation=? AND next_index=? AND state='open'`), end, state, r.ID, n.ID, generation, next)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("%w: map expansion boundary changed", ErrConflict)
	}
	if err = t.exec(`UPDATE dag_map_progress SET closed=? WHERE run_id=? AND node_id=?`, closed, r.ID, n.ID); err != nil {
		return err
	}
	if err = t.event(r, n.ID, "map_chunk_materialized", map[string]any{"generation": generation, "from_index": next, "next_index": end, "total": total, "closed": closed == 1}); err != nil {
		return err
	}
	if closed == 0 {
		return t.putWake(r, n, "expansion", t.now)
	}
	if err = t.exec(`DELETE FROM dag_wakeups WHERE run_id=? AND node_id=? AND kind='expansion'`, r.ID, n.ID); err != nil {
		return err
	}
	return t.enqueueNode(r, n.ID)
}
