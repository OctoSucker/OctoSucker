// Package types holds executor task/plan state, tool I/O, and dispatcher event/payload types.
package types

// Event 在 dispatcher 队列中传递；各 Type 对应 Task 状态上的分段注释。
// EvPlanProgressed：PlanExec 选取下一步并同步调用工具，直接产生 EvObservationReady。
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
	EvPlanProgressed   = "PlanProgressed"   // Planner 追加步骤后：PlanExec 执行下一 runnable 步并产生 ObservationReady
	EvObservationReady = "ObservationReady" // StepCritic：写入观测；成功后 → TrajectoryCheck
	EvTrajectoryCheck  = "TrajectoryCheck"  // TrajectoryCritic：完成 / 中止 / 续规划 / 截断重规划 → nil 或 EvUserInput
)
