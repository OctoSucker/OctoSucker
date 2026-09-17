package adminhttp

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OctoSucker/octosucker/internal/task"
)

func TestAPIOnlyHandler(t *testing.T) {
	handler, err := Handler(Options{
		RunChat:               func(context.Context, string, string) ([]string, error) { return nil, nil },
		SubmitAssistantInput:  func(string, string) (task.InputResult, error) { return task.InputResult{}, nil },
		SubmitTaskInteraction: func(string, string, map[string]any) (task.InputResult, error) { return task.InputResult{}, nil },
		SubmitTaskApproval:    func(string, string, string) (task.Snapshot, error) { return task.Snapshot{}, nil },
		GetTask:               func(id string) (task.Snapshot, bool) { return task.Snapshot{ID: id, Status: task.StatusRunning}, true },
	})
	if err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest("GET", "/", nil))
	if page.Code != 404 {
		t.Fatalf("root must not serve a frontend: status %d", page.Code)
	}
	api := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/tasks/test", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	handler.ServeHTTP(api, request)
	if api.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal("independent frontend CORS header missing")
	}
	if api.Code != 200 || !strings.Contains(api.Body.String(), `"id":"test"`) {
		t.Fatalf("task API unavailable: %s", api.Body.String())
	}
}
