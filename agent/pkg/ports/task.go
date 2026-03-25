package ports

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

type RouteType string

const (
	RouteTypePlanner                 RouteType = "planner"
	RouteTypeEmbeddingSkill          RouteType = "embedding_skill"
	RouteTypeKeywordSkill            RouteType = "keyword_skill"
	RouteTypeGraphConfidence         RouteType = "graph_confidence"
	RouteTypeHeuristicComplexRequest RouteType = "heuristic_complex_request"
)

type RoutePolicyDecision struct {
	Type       RouteType `json:"type"`
	Confidence float64   `json:"confidence"`
}

func (d *RoutePolicyDecision) UnmarshalJSON(b []byte) error {
	type v2 struct {
		Type       RouteType `json:"type"`
		Confidence float64   `json:"confidence"`
	}
	var x2 v2
	if err := json.Unmarshal(b, &x2); err == nil && x2.Type != "" {
		d.Type = x2.Type
		d.Confidence = x2.Confidence
		return nil
	}
	var legacy struct {
		Mode       RouteMode `json:"mode"`
		Confidence float64   `json:"confidence"`
		Reason     RouteType `json:"reason,omitempty"`
	}
	if err := json.Unmarshal(b, &legacy); err != nil {
		return err
	}
	d.Type = legacy.Reason
	if d.Type == "" {
		switch legacy.Mode {
		case RouteSkill:
			d.Type = RouteTypeEmbeddingSkill
		case RouteGraph:
			d.Type = RouteTypeGraphConfidence
		default:
			d.Type = RouteTypePlanner
		}
	}
	d.Confidence = legacy.Confidence
	return nil
}

type StepTrace struct {
	StepID  string `json:"step_id"`
	Tool    string `json:"tool"`
	OK      bool   `json:"ok"`
	Summary string `json:"summary"`
}

type Decision struct {
	Action ActionType `json:"action"`
	Reason string     `json:"reason,omitempty"`
}

type ActionType string

const (
	ActionAccept           ActionType = "accept"
	ActionRetry            ActionType = "retry"
	ActionAbort            ActionType = "abort"
	ActionSwitchCapability ActionType = "switch_capability"
)

type TransitionStep struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type RouteSnap struct {
	LastCap       string
	LastOut       int
	UserInput     string
	SkillPrior    []string
	Preferred     []string
	RouteType     RouteType
	GraphPathMode GraphPathMode
}

// Task 是单次用户消息触发的、贯穿 Planner → 执行 → 评判 的可变状态，随 Ev* 处理链读写并持久化。
// ID 为任务主键 UUID（SQLite PRIMARY KEY）；事件 Payload.task_id 与它保持一致。
// UserInput 内同时记录文本与消息来源（TelegramChatID/IngressChannel）。
// 下列分组按「主要读写方」对应 event.go 中的事件名；部分字段会被多个阶段读写（标注「贯穿」）。
// EvToolsBound 仅有 Payload 类型，dispatcher 未注册 handler，Task 无专用字段。
type Task struct {
	// --- 贯穿多阶段：标识、本轮用户原文、当前计划（Planner 写入 Plan；执行与评判改写步骤状态）---
	ID        string `json:"id"` // UUID；SQLite PRIMARY KEY
	UserInput struct {
		Text           string `json:"text"`
		TelegramChatID int64  `json:"telegram_chat_id,omitempty"`
		IngressChannel string `json:"ingress_channel,omitempty"`
	}

	Plan *Plan `json:"plan,omitempty"`

	// --- EvUserInput（Planner）：路由与技能先验；之后 PlanExec（RouteSnap）与 StepCritic 读取 ---
	RoutePolicy          *RoutePolicyDecision `json:"route_policy,omitempty"`    // mode/confidence/reason；RouteSnap 取 Mode
	GraphPathMode        GraphPathMode        `json:"graph_path_mode,omitempty"` // greedy vs global；见 GraphPathMode
	SkillPriorCaps       []string             `json:"skill_prior_caps,omitempty"`
	SkillPreferredPath   []string             `json:"skill_preferred_path,omitempty"`
	ActiveSkillName      string               `json:"active_skill_name,omitempty"`       // EvTurnFinalized 学习归因 RecordTurn
	ActiveSkillVariantID string               `json:"active_skill_variant_id,omitempty"` // 同上

	// --- EvPlanProgressed / EvStepCapabilityRetry（PlanExec）：当前步、待执行工具、能力多工具链 ---
	StepID         string   `json:"step_id,omitempty"`
	PendingTool    string   `json:"pending_tool,omitempty"`
	CapChainStepID string   `json:"cap_chain_step_id,omitempty"`
	CapChainTools  []string `json:"cap_chain_tools,omitempty"`
	CapChainNext   int      `json:"cap_chain_next,omitempty"`

	// --- EvObservationReady（StepCritic）：工具观测后的重试/换能力决策；Planner 每轮开始会清空其中多数 ---
	// Trace：StepCritic 追加；PlanExec 在批量步进时也可能追加。LastCapability/LastOutcome：StepCritic 与 PlanExec 均会更新。
	Trace               []StepTrace    `json:"trace,omitempty"`
	ToolFailCount       map[string]int `json:"tool_fail_count,omitempty"`
	CapabilityFailCount map[string]int `json:"capability_fail_count,omitempty"` // key: CapabilityFailCountKey(step, cap)
	LastStepDecision    *Decision      `json:"last_step_decision,omitempty"`
	LastCapability      string         `json:"last_capability"`
	LastOutcome         int            `json:"last_outcome"`

	// --- EvTrajectoryCheck（TrajectoryCritic）：合成对用户回复、轨迹打分；失败且允许时可能再次入队 EvUserInput（AutoReplan）---
	Reply             string  `json:"reply"`
	TrajectoryScore   float64 `json:"trajectory_score"`
	TrajectorySummary string  `json:"trajectory_summary,omitempty"`
	ReplanAllowed     bool    `json:"replan_allowed,omitempty"` // Planner 与 TrajectoryCritic 均会写
	ReplanCount       int     `json:"replan_count,omitempty"`

	// --- EvTurnFinalized：路由图学习；StepCritic / PlanExec 在执行过程中追加边 ---
	TransitionPath []TransitionStep `json:"transition_path,omitempty"`
}

// RouteSnap 供执行层解析能力路径使用；要求 RoutePolicy 已由 Planner 写入（与 Plan 同时存在）。
func (t *Task) RouteSnap() (RouteSnap, error) {
	if t == nil {
		return RouteSnap{}, fmt.Errorf("ports: RouteSnap: nil task")
	}
	if t.RoutePolicy == nil {
		return RouteSnap{}, fmt.Errorf("ports: RouteSnap: nil RoutePolicy (expected after successful planning)")
	}
	gpm := t.GraphPathMode
	if gpm == "" {
		gpm = GraphPathGreedy
	}
	return RouteSnap{
		LastCap:       t.LastCapability,
		LastOut:       t.LastOutcome,
		UserInput:     t.UserInput.Text,
		SkillPrior:    append([]string(nil), t.SkillPriorCaps...),
		Preferred:     append([]string(nil), t.SkillPreferredPath...),
		RouteType:     t.RoutePolicy.Type,
		GraphPathMode: gpm,
	}, nil
}

const IngressHTTP = "http"
const IngressTelegram = "telegram"

// NewTaskID returns a random UUID v4 string for Task.ID (storage primary key).
func NewTaskID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("ports: NewTaskID: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
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
