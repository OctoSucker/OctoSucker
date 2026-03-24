package ports

type Session struct {
	ID                 string   `json:"id"`
	StepID             string   `json:"step_id,omitempty"`
	UserInput          string   `json:"user_input"`
	Plan               *Plan    `json:"plan,omitempty"`
	Reply              string   `json:"reply"`
	LastCapability     string   `json:"last_capability"`
	LastOutcome        int      `json:"last_outcome"`
	SkillPriorCaps     []string `json:"skill_prior_caps,omitempty"`
	SkillPreferredPath []string `json:"skill_preferred_path,omitempty"`
	// ActiveSkillName / ActiveSkillVariantID attribute a turn to a learned skill variant for RecordTurn.
	ActiveSkillName      string    `json:"active_skill_name,omitempty"`
	ActiveSkillVariantID string    `json:"active_skill_variant_id,omitempty"`
	RouteMode            RouteMode `json:"route_mode,omitempty"`
	// GraphPathMode: greedy (Frontier) vs global (Dijkstra toward finish); see ports.GraphPathMode.
	GraphPathMode       GraphPathMode        `json:"graph_path_mode,omitempty"`
	RoutePolicy         *RoutePolicyDecision `json:"route_policy,omitempty"`
	PendingTool         string               `json:"pending_tool,omitempty"`
	CapChainStepID      string               `json:"cap_chain_step_id,omitempty"`
	CapChainTools       []string             `json:"cap_chain_tools,omitempty"`
	CapChainNext        int                  `json:"cap_chain_next,omitempty"`
	ToolFailCount       map[string]int       `json:"tool_fail_count,omitempty"`
	CapabilityFailCount map[string]int       `json:"capability_fail_count,omitempty"` // key: StepCritic CapabilityFailCountKey(step, cap)
	Trace               []StepTrace          `json:"trace,omitempty"`
	TrajectoryScore     float64              `json:"trajectory_score"`
	TrajectorySummary   string               `json:"trajectory_summary,omitempty"`
	RecallContext       string               `json:"recall_context,omitempty"`
	ReplanAllowed       bool                 `json:"replan_allowed,omitempty"`
	ReplanCount         int                  `json:"replan_count,omitempty"`
	LastStepDecision    *Decision            `json:"last_step_decision,omitempty"`
	TransitionPath      []TransitionStep     `json:"transition_path,omitempty"`
}

type RoutePolicyDecision struct {
	Mode       RouteMode `json:"mode"`
	Confidence float64   `json:"confidence"`
	Reason     string    `json:"reason,omitempty"`
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
	RouteMode     RouteMode
	GraphPathMode GraphPathMode
}

func (s *Session) RouteSnap() RouteSnap {
	if s == nil {
		return RouteSnap{}
	}
	gpm := s.GraphPathMode
	if gpm == "" {
		gpm = GraphPathGreedy
	}
	return RouteSnap{
		LastCap:       s.LastCapability,
		LastOut:       s.LastOutcome,
		UserInput:     s.UserInput,
		SkillPrior:    append([]string(nil), s.SkillPriorCaps...),
		Preferred:     append([]string(nil), s.SkillPreferredPath...),
		RouteMode:     s.RouteMode,
		GraphPathMode: gpm,
	}
}
