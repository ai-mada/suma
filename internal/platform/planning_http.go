package platform

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) planning(w http.ResponseWriter, r *http.Request, p Principal, project, path string, m map[string]any) bool {
	parts := strings.Split(path, "/")
	ctx := r.Context()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if path == "planning" && r.Method == "GET" {
		out, e := s.Service.Planning(ctx, p, project, limit, r.URL.Query().Get("cursor"), r.URL.Query().Get("week_start"), r.URL.Query().Get("task_id"))
		respond(w, 200, out, e)
		return true
	}
	if parts[0] == "iterations" || parts[0] == "milestones" {
		if len(parts) == 1 && r.Method == "GET" {
			out, e := s.Service.PlanningPeriods(ctx, p, project, parts[0])
			respondList(w, out, e)
			return true
		}
		if len(parts) == 1 && r.Method == "POST" {
			out, e := s.Service.SavePlanningPeriod(ctx, p, project, parts[0], "", m)
			respond(w, 201, out, e)
			return true
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			out, e := s.Service.SavePlanningPeriod(ctx, p, project, parts[0], parts[1], m)
			respond(w, 200, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	if path == "planning/schedule" && r.Method == "POST" {
		out, e := s.Service.ScheduleTasks(ctx, p, project, m)
		respondList(w, out, e)
		return true
	}
	if path == "planning/capacity" {
		if r.Method == "GET" {
			out, e := s.Service.PlanningCapacity(ctx, p, project, r.URL.Query().Get("week_start"))
			respondList(w, out, e)
			return true
		}
		if r.Method == "POST" {
			out, e := s.Service.SaveCapacity(ctx, p, project, m)
			respond(w, 200, out, e)
			return true
		}
	}
	if path == "planning/baselines" {
		if r.Method == "GET" {
			out, e := s.Service.Baselines(ctx, p, project)
			respondList(w, out, e)
			return true
		}
		if r.Method == "POST" {
			out, e := s.Service.CaptureBaseline(ctx, p, project, m)
			respond(w, 201, out, e)
			return true
		}
	}
	if len(parts) == 3 && parts[0] == "planning" && parts[1] == "baselines" && r.Method == "GET" {
		out, e := s.Service.Baseline(ctx, p, project, parts[2], r.URL.Query().Get("cursor"), limit)
		respond(w, 200, out, e)
		return true
	}
	return false
}
