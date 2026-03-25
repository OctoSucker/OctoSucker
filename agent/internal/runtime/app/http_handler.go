package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	rtutils "github.com/OctoSucker/agent/utils"
)

type runRequest struct {
	TaskID string `json:"task_id"`
	Text   string `json:"text"`
}

type runResponse struct {
	Reply             string  `json:"reply"`
	TaskID            string  `json:"task_id"`                     // routing key (same as request); use for subsequent /run
	CanonicalTaskID   string  `json:"canonical_task_id,omitempty"` // Task.ID (UUID) — storage row primary key
	TrajectoryScore   float64 `json:"trajectory_score"`
	TrajectorySummary string  `json:"trajectory_summary"`
}

type errResponse struct {
	Error string `json:"error"`
}

func (a *App) HTTPHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("http health write: %v", err)
		}
	})
	m.HandleFunc("POST /task/{id}/rerun", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" && strings.TrimSpace(a.ConversationID) == "" {
			if err := rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "task id required"}); err != nil {
				log.Printf("http write error response: %v", err)
			}
			return
		}
		reply, err := a.RerunTaskPlan(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrRerunNoPlan) {
				if werr := rtutils.WriteJSON(w, http.StatusNotFound, errResponse{Error: err.Error()}); werr != nil {
					log.Printf("http write error response: %v", werr)
				}
				return
			}
			if werr := rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: err.Error()}); werr != nil {
				log.Printf("http write error response: %v", werr)
			}
			return
		}
		tid, _, _ := a.effectiveTaskID(id, 0)
		taskState, ok := a.Dispatcher.Planner.Tasks.Get(tid)
		if !ok {
			if err := rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: "task missing after rerun"}); err != nil {
				log.Printf("http write error response: %v", err)
			}
			return
		}
		if err := rtutils.WriteJSON(w, http.StatusOK, map[string]any{
			"reply": reply, "task_id": tid, "canonical_task_id": taskState.ID,
			"trajectory_score": float64(taskState.TrajectoryScore), "trajectory_summary": taskState.TrajectorySummary,
		}); err != nil {
			log.Printf("http write ok response: %v", err)
		}
	})
	m.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if werr := rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "invalid json"}); werr != nil {
				log.Printf("http write error response: %v", werr)
			}
			return
		}
		if req.TaskID == "" && strings.TrimSpace(a.ConversationID) == "" {
			if err := rtutils.WriteJSON(w, http.StatusBadRequest, errResponse{Error: "task_id required (or set workspace conversation_id)"}); err != nil {
				log.Printf("http write error response: %v", err)
			}
			return
		}
		reply, err := a.RunInput(r.Context(), req.TaskID, req.Text, 0)
		if err != nil {
			if werr := rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: err.Error()}); werr != nil {
				log.Printf("http write error response: %v", werr)
			}
			return
		}
		tid, _, _ := a.effectiveTaskID(req.TaskID, 0)
		taskState, ok := a.Dispatcher.Planner.Tasks.Get(tid)
		if !ok {
			if err := rtutils.WriteJSON(w, http.StatusInternalServerError, errResponse{Error: "task missing"}); err != nil {
				log.Printf("http write error response: %v", err)
			}
			return
		}
		if err := rtutils.WriteJSON(w, http.StatusOK, runResponse{
			Reply:             reply,
			TaskID:            tid,
			CanonicalTaskID:   taskState.ID,
			TrajectoryScore:   float64(taskState.TrajectoryScore),
			TrajectorySummary: taskState.TrajectorySummary,
		}); err != nil {
			log.Printf("http write ok response: %v", err)
		}
	})
	return m
}
