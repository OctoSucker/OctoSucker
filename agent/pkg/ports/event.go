package ports

// Event 在 dispatcher 队列中传递；各 Type 对应 Task 字段分组见 Task 结构体上的分段注释。
// EvToolCall：由 PlanExec 入队，ToolExec 执行 MCP；一般不单独对应 Task 持久字段，结果经 EvObservationReady 写回。
// EvToolsBound：预留常量，当前无 handler、Task 无对应字段。
type Event struct {
	Type    string
	Payload any
}

func EventPtr(e Event) *Event { return &e }

const (
	EvUserInput              = "UserInput"              // Planner：路由、计划、技能先验等 → Task 分段「EvUserInput」与「贯穿」
	EvProcedurePlanRequested = "ProcedurePlanRequested" // Planner：技能路线生成计划
	EvLLMPlanRequested       = "LLMPlanRequested"       // Planner：LLM 路线生成计划
	EvPlanProgressed         = "PlanProgressed"         // PlanExec：推进计划（初始 Plan 已就绪 或 任一步完成后继续步进）
	EvToolsBound             = "ToolsBound"             // 预留
	EvToolCall               = "ToolCall"               // ToolExec → 随后 ObservationReady
	EvObservationReady       = "ObservationReady"       // StepCritic：Trace、重试/换能力、Plan 步骤 → Task 分段「ObservationReady」
	EvStepCapabilityRetry    = "StepCapabilityRetry"    // PlanExec：换能力重试同一步
	EvTrajectoryCheck        = "TrajectoryCheck"        // TrajectoryCritic：Reply、轨迹分、可能 EvUserInput 重规划
	EvTurnFinalized          = "TurnFinalized"          // 收尾学习：TransitionPath、ActiveProcedure*、UserInput 等
)
