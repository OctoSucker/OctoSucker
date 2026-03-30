package ports

// Event 在 dispatcher 队列中传递；各 Type 对应 Task 字段分组见 Task 结构体上的分段注释。
// EvToolCall：由 PlanExec 入队，ToolExec 执行 MCP；一般不单独对应 Task 持久字段，结果经 EvObservationReady 写回。
type Event struct {
	Type    string
	Payload any
}

func EventPtr(e Event) *Event { return &e }

// TaskIDFromEvent returns the task id in evt's payload for known Ev* types.
func TaskIDFromEvent(evt Event) (string, bool) {
	switch evt.Type {
	case EvUserInput:
		p, ok := evt.Payload.(PayloadUserInput)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvPlanProgressed:
		p, ok := evt.Payload.(PayloadPlanProgressed)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvToolCall:
		p, ok := evt.Payload.(PayloadToolCall)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvObservationReady:
		p, ok := evt.Payload.(PayloadObservation)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	case EvTrajectoryCheck:
		p, ok := evt.Payload.(PayloadTrajectoryCheck)
		if !ok {
			return "", false
		}
		return p.TaskID, p.TaskID != ""
	default:
		return "", false
	}
}

const (
	EvUserInput        = "UserInput"        // Planner：路由、计划、技能先验等 → Task 分段「EvUserInput」与「贯穿」
	EvPlanProgressed   = "PlanProgressed"   // 尚无 Plan 时 Planner 补全；已有 Plan 时 PlanExec 推进步进
	EvToolCall         = "ToolCall"         // ToolExec → 随后 ObservationReady
	EvObservationReady = "ObservationReady" // StepCritic：Plan 步观测写入、重试、重规划 → Task 分段「ObservationReady」
	EvTrajectoryCheck  = "TrajectoryCheck"  // TrajectoryCritic：LLM 目标达成判定；达成则收尾并 nil 结束 Run，未达成则返回 EvUserInput 重规划
)
