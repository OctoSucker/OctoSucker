package judge

const (
	// maxReplansPerTurn caps EvUserInput PlannerContinuation cycles from both
	// StepCritic (tool failure after retries) and TrajectoryCritic (goal not met).
	maxReplansPerTurn = 5

	// maxTotalToolFailuresPerTurn counts every failed tool observation (including retries).
	// When exceeded, the turn stops with an explicit Reply instead of replanning.
	maxTotalToolFailuresPerTurn = 24
)
