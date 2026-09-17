package task

import (
	"encoding/json"
	"testing"
	"time"
)

type memoryPersistence struct {
	payloads map[string][]byte
}

func (m *memoryPersistence) LoadTaskSnapshots() ([][]byte, error) {
	out := make([][]byte, 0, len(m.payloads))
	for _, payload := range m.payloads {
		out = append(out, append([]byte(nil), payload...))
	}
	return out, nil
}

func (m *memoryPersistence) SaveTaskSnapshot(id string, payload []byte) error {
	if m.payloads == nil {
		m.payloads = make(map[string][]byte)
	}
	m.payloads[id] = append([]byte(nil), payload...)
	return nil
}

func TestPrepareInputPreservesObjective(t *testing.T) {
	store := NewStore()
	created := store.Create("原始完整任务目标", "")
	store.Finish(created.ID, StatusWaitingInput, []string{"请补充信息"}, nil, "", "")

	updated, err := store.PrepareInput(created.ID, "这是补充信息")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Objective != "原始完整任务目标" {
		t.Fatalf("objective = %q; want original objective", updated.Objective)
	}
	if updated.Status != StatusRunning {
		t.Fatalf("status = %q; want running", updated.Status)
	}
	if got := updated.Messages[len(updated.Messages)-1].Content; got != "这是补充信息" {
		t.Fatalf("last message = %q", got)
	}
}

func TestPersistentStoreRestoresCompletedTask(t *testing.T) {
	persistence := &memoryPersistence{payloads: make(map[string][]byte)}
	store, err := NewPersistentStore(persistence)
	if err != nil {
		t.Fatal(err)
	}
	created := store.Create("durable objective", "")
	store.UpdatePlan(created.ID, Plan{Objective: "durable objective", Revision: 1, Steps: []PlanStep{{ID: "one", Goal: "one", SuccessCriteria: "one done", Status: "completed", Evidence: "proof"}}})
	store.Finish(created.ID, StatusCompleted, []string{"done"}, nil, "result", "")

	restored, err := NewPersistentStore(persistence)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := restored.Get(created.ID)
	if !ok {
		t.Fatal("restored task not found")
	}
	if snapshot.Status != StatusCompleted || snapshot.Objective != "durable objective" || snapshot.Result == nil || snapshot.Result.Summary != "result" {
		t.Fatalf("restored snapshot = %#v", snapshot)
	}
	if snapshot.Plan == nil || snapshot.Plan.Revision != 1 || snapshot.Plan.Steps[0].Evidence != "proof" {
		t.Fatalf("restored plan = %#v", snapshot.Plan)
	}
}

func TestPersistentStoreExpiresApprovalAfterRestart(t *testing.T) {
	now := time.Now().UTC()
	snapshot := Snapshot{
		ID:              "task-1",
		Objective:       "dangerous task",
		Status:          StatusWaitingApproval,
		CreatedAt:       now,
		UpdatedAt:       now,
		PendingApproval: &Approval{ID: "approval-1", Status: "pending"},
		Steps:           []Step{{ID: "step-1", Status: "waiting_approval"}},
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	persistence := &memoryPersistence{payloads: map[string][]byte{snapshot.ID: payload}}

	store, err := NewPersistentStore(persistence)
	if err != nil {
		t.Fatal(err)
	}
	restored, _ := store.Get(snapshot.ID)
	if restored.Status != StatusWaitingInput {
		t.Fatalf("status = %q; want waiting_input", restored.Status)
	}
	if restored.PendingApproval != nil {
		t.Fatal("stale approval was restored")
	}
	if restored.Steps[0].Status != "waiting_input" {
		t.Fatalf("step status = %q", restored.Steps[0].Status)
	}
}
