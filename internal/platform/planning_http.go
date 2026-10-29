package platform

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) planning(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) bool {
	parts := strings.Split(path, "/")
	ctx := r.Context()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if path == "planning" && r.Method == "GET" {
		out, e := s.Service.Planning(ctx, actor, project, limit, r.URL.Query().Get("cursor"), r.URL.Query().Get("week_start"), r.URL.Query().Get("task_id"))
		respond(w, 200, out, e)
		return true
	}
	if parts[0] == "iterations" || parts[0] == "milestones" {
		if len(parts) == 1 && r.Method == "GET" {
			out, e := s.Service.PlanningPeriods(ctx, actor, project, parts[0])
			respondList(w, out, e)
			return true
		}
		if len(parts) == 1 && r.Method == "POST" {
			out, e := s.Service.SavePlanningPeriod(ctx, actor, project, parts[0], "", m)
			respond(w, 201, out, e)
			return true
		}
		if len(parts) == 2 && r.Method == "PATCH" {
			out, e := s.Service.SavePlanningPeriod(ctx, actor, project, parts[0], parts[1], m)
			respond(w, 200, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	if path == "planning/schedule" && r.Method == "POST" {
		out, e := s.Service.ScheduleTasks(ctx, actor, project, m)
		respondList(w, out, e)
		return true
	}
	if path == "planning/capacity" {
		if r.Method == "GET" {
			out, e := s.Service.PlanningCapacity(ctx, actor, project, r.URL.Query().Get("week_start"))
			respondList(w, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	if len(parts) == 2 && parts[0] == "resource-calendars" {
		if r.Method == http.MethodGet {
			out, e := s.Service.GetResourceCalendar(ctx, actor, project, parts[1])
			respond(w, http.StatusOK, out, e)
			return true
		}
		if r.Method == http.MethodPut {
			req, e := decodeResourceCalendarSave(m)
			if e != nil {
				writeError(w, e)
				return true
			}
			out, e := s.Service.SaveResourceCalendar(ctx, actor, project, parts[1], req)
			respond(w, http.StatusOK, out, e)
			return true
		}
		writeError(w, missing())
		return true
	}
	if path == "planning/baselines" {
		if r.Method == "GET" {
			out, e := s.Service.Baselines(ctx, actor, project)
			respondList(w, out, e)
			return true
		}
		if r.Method == "POST" {
			out, e := s.Service.CaptureBaseline(ctx, actor, project, m)
			respond(w, 201, out, e)
			return true
		}
	}
	if len(parts) == 3 && parts[0] == "planning" && parts[1] == "baselines" && r.Method == "GET" {
		out, e := s.Service.Baseline(ctx, actor, project, parts[2], r.URL.Query().Get("cursor"), limit)
		respond(w, 200, out, e)
		return true
	}
	return false
}
