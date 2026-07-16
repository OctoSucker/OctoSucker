// Package tasks exposes the single-assistant task API used by the desktop UI.
package tasks

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp/jsonresp"
	"github.com/OctoSucker/octosucker/internal/task"
)

const maxBodyBytes = 1 << 20

type SubmitInput func(activeTaskID, content string) (task.InputResult, error)
type SubmitInteraction func(taskID, interactionID string, values map[string]any) (task.InputResult, error)
type SubmitApproval func(taskID, approvalID, decision string) (task.Snapshot, error)
type GetTask func(taskID string) (task.Snapshot, bool)

func Register(mux *http.ServeMux, submitInput SubmitInput, submitInteraction SubmitInteraction, submitApproval SubmitApproval, getTask GetTask) {
	mux.Handle("POST /api/assistant/input", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req inputRequest
		if !decode(w, r, &req, "/api/assistant/input") {
			return
		}
		result, err := submitInput(req.ActiveTaskID, req.Content)
		if err != nil {
			writeError(w, err, "/api/assistant/input")
			return
		}
		jsonresp.Write(w, http.StatusAccepted, result, "/api/assistant/input")
	}))

	mux.Handle("GET /api/tasks/{taskID}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, ok := getTask(r.PathValue("taskID"))
		if !ok {
			jsonresp.Write(w, http.StatusNotFound, errorResponse{Error: "task not found"}, "/api/tasks/{taskID}")
			return
		}
		jsonresp.Write(w, http.StatusOK, result, "/api/tasks/{taskID}")
	}))

	mux.Handle("POST /api/tasks/{taskID}/interactions/{interactionID}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req interactionRequest
		if !decode(w, r, &req, "/api/tasks/{taskID}/interactions/{interactionID}") {
			return
		}
		result, err := submitInteraction(r.PathValue("taskID"), r.PathValue("interactionID"), req.Values)
		if err != nil {
			writeError(w, err, "/api/tasks/{taskID}/interactions/{interactionID}")
			return
		}
		jsonresp.Write(w, http.StatusAccepted, result, "/api/tasks/{taskID}/interactions/{interactionID}")
	}))

	mux.Handle("POST /api/tasks/{taskID}/approvals/{approvalID}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req approvalRequest
		if !decode(w, r, &req, "/api/tasks/{taskID}/approvals/{approvalID}") {
			return
		}
		result, err := submitApproval(r.PathValue("taskID"), r.PathValue("approvalID"), req.Decision)
		if err != nil {
			writeError(w, err, "/api/tasks/{taskID}/approvals/{approvalID}")
			return
		}
		jsonresp.Write(w, http.StatusAccepted, result, "/api/tasks/{taskID}/approvals/{approvalID}")
	}))
}

type inputRequest struct {
	ActiveTaskID string `json:"active_task_id,omitempty"`
	Content      string `json:"content"`
}

type interactionRequest struct {
	Values map[string]any `json:"values"`
}

type approvalRequest struct {
	Decision string `json:"decision"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func decode(w http.ResponseWriter, r *http.Request, out any, route string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		jsonresp.Write(w, http.StatusBadRequest, errorResponse{Error: err.Error()}, route)
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error, route string) {
	message := err.Error()
	status := http.StatusConflict
	if strings.Contains(message, "not found") {
		status = http.StatusNotFound
	} else if strings.Contains(message, "empty") || strings.Contains(message, "does not match") {
		status = http.StatusBadRequest
	}
	jsonresp.Write(w, status, errorResponse{Error: message}, route)
}
