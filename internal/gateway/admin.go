package gateway

import (
	"net/http"

	"github.com/OctoSucker/octosucker/internal/ingress/adminhttp"
	"github.com/OctoSucker/octosucker/internal/storage"
)

// adminHTTPHandler serves the admin shell and JSON APIs via [adminhttp.Handler].
func adminHTTPHandler(agent Agent) (http.Handler, error) {
	return adminhttp.Handler(adminhttp.Options{
		RunChat:               agent.RunTurn,
		PlanInteraction:       agent.PlanInteraction,
		SubmitAssistantInput:  agent.SubmitAssistantInput,
		SubmitTaskInteraction: agent.SubmitInteraction,
		SubmitTaskApproval:    agent.SubmitApproval,
		GetTask:               agent.Task,
		Graph: func() storage.KnowledgeGraphReader {
			return agent.WorkspaceDB()
		},
	})
}
