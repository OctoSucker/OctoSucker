package app

import (
	"encoding/json"
	"errors"
	"net/http"

	rtutils "github.com/OctoSucker/agent/utils"
	"github.com/OctoSucker/agent/pkg/ports"
)

type runRequest struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

type runResponse struct {
	Reply             string  `json:"reply"`
	SessionID         string  `json:"session_id"`
	TrajectoryScore   float64 `json:"trajectory_score"`
	TrajectorySummary string  `json:"trajectory_summary"`
}

type errResponse struct {
	Error string `json:"error"`
}

type sessionStepDTO struct {
	ID         string         `json:"id"`
	Goal       string         `json:"goal"`
	Status     string         `json:"status"`
	Capability string         `json:"capability"`
	DependsOn  []string       `json:"depends_on"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

type sessionDecisionDTO struct {
	Action ports.ActionType `json:"action"`
	Reason string           `json:"reason,omitempty"`
}

type routePolicyDTO struct {
	Mode       ports.RouteMode `json:"mode"`
	Confidence float64         `json:"confidence"`
	Reason     string          `json:"reason,omitempty"`
}

type sessionResponse struct {
	SessionID          string              `json:"session_id"`
	StepID             string              `json:"step_id,omitempty"`
	PendingTool        string              `json:"pending_tool,omitempty"`
	UserInput          string              `json:"user_input"`
	Reply              string              `json:"reply"`
	PlanSteps          []sessionStepDTO    `json:"plan_steps"`
	TrajectoryScore    float64             `json:"trajectory_score"`
	TrajectorySummary  string              `json:"trajectory_summary"`
	LastCapability     string              `json:"last_capability"`
	LastStepDecision   *sessionDecisionDTO `json:"last_step_decision,omitempty"`
	RouteMode          ports.RouteMode     `json:"route_mode,omitempty"`
	RoutePolicy        *routePolicyDTO     `json:"route_policy,omitempty"`
	SkillPreferredPath []string            `json:"skill_preferred_path,omitempty"`
}

func (a *App) HTTPHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	m.HandleFunc("POST /session/{id}/rerun", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "session id required"})
			return
		}
		reply, err := a.RerunSessionPlan(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrRerunNoPlan) {
				rtutils.WriteJSON(w, http.StatusNotFound, errResponse{Error: err.Error()})
				return
			}
			rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: err.Error()})
			return
		}
		sess, _ := a.Dispatcher.Sessions.Get(id)
		rtutils.WriteJSON(w, http.StatusOK, map[string]any{
			"reply": reply, "session_id": id,
			"trajectory_score": float64(sess.TrajectoryScore), "trajectory_summary": sess.TrajectorySummary,
		})
	})
	m.HandleFunc("GET /session/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "session id required"})
			return
		}
		sess, ok := a.Dispatcher.Sessions.Get(id)
		if !ok {
			rtutils.WriteJSON(w, http.StatusNotFound, errResponse{Error: "not found"})
			return
		}
		out := sessionResponse{
			SessionID:          sess.ID,
			StepID:             sess.StepID,
			PendingTool:        sess.PendingTool,
			UserInput:          sess.UserInput,
			Reply:              sess.Reply,
			TrajectoryScore:    float64(sess.TrajectoryScore),
			TrajectorySummary:  sess.TrajectorySummary,
			LastCapability:     sess.LastCapability,
			RouteMode:          sess.RouteMode,
			SkillPreferredPath: append([]string(nil), sess.SkillPreferredPath...),
		}
		if sess.RoutePolicy != nil {
			out.RoutePolicy = &routePolicyDTO{
				Mode: sess.RoutePolicy.Mode, Confidence: sess.RoutePolicy.Confidence, Reason: sess.RoutePolicy.Reason,
			}
		}
		if sess.LastStepDecision != nil {
			out.LastStepDecision = &sessionDecisionDTO{Action: sess.LastStepDecision.Action, Reason: sess.LastStepDecision.Reason}
		}
		if sess.Plan != nil {
			for _, st := range sess.Plan.Steps {
				out.PlanSteps = append(out.PlanSteps, sessionStepDTO{
					ID: st.ID, Goal: st.Goal, Status: st.Status, Capability: st.Capability, DependsOn: st.DependsOn,
					Arguments: st.Arguments,
				})
			}
		}
		rtutils.WriteJSON(w, http.StatusOK, out)
	})
	m.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "invalid json"})
			return
		}
		if req.SessionID == "" {
			rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "session_id required"})
			return
		}
		sid := rtutils.HTTPNormalizeChatSessionID(req.SessionID)
		reply, err := a.RunInput(r.Context(), sid, req.Text)
		if err != nil {
			rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: err.Error()})
			return
		}
		sess, ok := a.Dispatcher.Sessions.Get(sid)
		if !ok {
			rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: "session missing"})
			return
		}
		rtutils.WriteJSON(w, http.StatusOK, runResponse{
			Reply:             reply,
			SessionID:         sid,
			TrajectoryScore:   float64(sess.TrajectoryScore),
			TrajectorySummary: sess.TrajectorySummary,
		})
	})
	return m
}
