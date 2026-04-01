package ports

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"strings"
)

type RouteType string

const (
	RouteTypePlanner                 RouteType = "planner"
	RouteTypeGraphConfidence         RouteType = "graph_confidence"
	RouteTypeHeuristicComplexRequest RouteType = "heuristic_complex_request"
)

// RouteSnap carries routing context for execution-time decisions.
type RouteSnap struct {
	RouteType  RouteType `json:"route_type"`
	Confidence float64   `json:"confidence"`
}

// Task 是单次用户消息触发的、贯穿 Planner → 执行 → 评判 的可变状态，随 Ev* 处理链读写并持久化。
// ID 为任务主键 UUID（SQLite PRIMARY KEY）；事件 Payload.task_id 与它保持一致。
// UserInput 内同时记录文本与消息来源（TelegramChatID/IngressChannel）。
// 下列分组按「主要读写方」对应 event.go 中的事件名；部分字段会被多个阶段读写（标注「贯穿」）。
type Task struct {
	// --- 贯穿多阶段：标识、本轮用户原文、当前计划（Planner 写入 Plan；执行与评判改写步骤状态）---
	ID        string `json:"id"` // UUID；SQLite PRIMARY KEY
	UserInput string `json:"user_input"`

	Plan *Plan `json:"plan,omitempty"`

	// --- EvTrajectoryCheck（TrajectoryCritic）：LLM 判定是否达成用户目标；达成则写 Reply / 分数字段、recall、total_runs；未达成则清空计划并 EvUserInput 续写重规划。ReplanCount 与 StepCritic 触发的 PlannerContinuation 共享同一「每回合重规划次数」上限。---
	// Reply：由各 done 步 PlanStep 的观测合成（UserReplyFromPlan），表示工具侧产出，不是评判文案。
	Reply string `json:"reply"`
	// TrajectorySummary：轨迹评判正文——规则生成的 baseMsg（步数/成功率等）+ LLM 点评；与用户可见「执行结果」Reply 分离。
	TrajectorySummary string `json:"trajectory_summary,omitempty"`
	ReplanCount       int    `json:"replan_count,omitempty"`
}

const IngressTelegram = "telegram"

// NewTaskIDFromSeed returns deterministic UUID v5-like value for a stable source key.
func NewTaskIDFromSeed(seed string) string {
	h := sha1.Sum([]byte(seed))
	b := h[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	var hi [8]byte
	copy(hi[2:], b[10:16])
	lo6 := binary.BigEndian.Uint64(hi[:])
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		lo6)
}

// TruncatePlanFromStep adjusts plan for replanning. Two modes:
//   - failedStepID non-empty (StepCritic): remove that step and all following; keep prefix. Empty prefix clears plan and resets RouteSnap to entry.
//   - failedStepID empty (TrajectoryCritic): discard the entire plan and reset RouteSnap to entry (full replan). StepCritic must pass a concrete step id.
func (t *Task) TruncatePlanFromStep(failedStepID string) error {
	if failedStepID == "" {
		t.Plan = &Plan{
			Steps: make([]*PlanStep, 0),
		}
		return nil
	} else {
		if t.Plan == nil || len(t.Plan.Steps) == 0 {
			return fmt.Errorf("task: cannot truncate plan (failed step %q)", failedStepID)
		}
		cut := -1
		for i := range t.Plan.Steps {
			if t.Plan.Steps[i].ID == failedStepID {
				cut = i
				break
			}
		}
		if cut < 0 {
			return fmt.Errorf("task: failed step %q not found in plan", failedStepID)
		}
		t.Plan.Steps = t.Plan.Steps[:cut]
		if len(t.Plan.Steps) == 0 {
			t.Plan = &Plan{
				Steps: make([]*PlanStep, 0),
			}
			return nil
		}
		last := t.Plan.Steps[len(t.Plan.Steps)-1]
		n := &last.Node
		if !n.IsValid() {
			return fmt.Errorf("task: last plan step has invalid capability/tool")
		}
		return nil
	}
}

// UserFacingTurnMessages returns chat bubbles: trace-derived Reply first, then TrajectorySummary when present.
// Raw field values are appended when the trimmed form is non-empty (same as historical Telegram behavior).
func (t *Task) UserFacingTurnMessages() ([]string, error) {
	reply := strings.ReplaceAll(t.Reply, `\n`, "\n")
	summary := strings.ReplaceAll(t.TrajectorySummary, `\n`, "\n")
	r := strings.TrimSpace(reply)
	s := strings.TrimSpace(summary)
	if r == "" && s == "" {
		return nil, fmt.Errorf("task has empty reply")
	}
	var out []string
	if r != "" {
		out = append(out, reply)
	}
	if s != "" {
		out = append(out, summary)
	}
	return out, nil
}

// UserFacingRecallDocument merges Reply and TrajectorySummary for recall (trimmed, "\n\n" between).
func (t *Task) UserFacingRecallDocument() string {
	r := strings.TrimSpace(t.Reply)
	s := strings.TrimSpace(t.TrajectorySummary)
	switch {
	case r == "" && s == "":
		return ""
	case s == "":
		return r
	case r == "":
		return s
	default:
		return r + "\n\n" + s
	}
}

// RecallPlannerCorpusDocument builds recall text aligned with PlannerUserContent: user request, plan outline,
// tool outputs, and trajectory rationale so retrieval matches future user queries at plan time.
func (t *Task) RecallPlannerCorpusDocument(p *Plan) string {
	if t == nil {
		return ""
	}
	var b strings.Builder
	if ut := strings.TrimSpace(t.UserInput); ut != "" {
		b.WriteString("用户请求：\n")
		b.WriteString(ut)
	}
	if p != nil && len(p.Steps) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("计划步骤：\n")
		for _, st := range p.Steps {
			fmt.Fprintf(&b, "- %s %s [%s]\n", st.ID, st.Goal, st.Node.String())
		}
	}
	r := strings.TrimSpace(t.Reply)
	s := strings.TrimSpace(t.TrajectorySummary)
	if r != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("工具输出：\n")
		b.WriteString(r)
	}
	if s != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("轨迹评判：\n")
		b.WriteString(s)
	}
	return strings.TrimSpace(b.String())
}
