package planning

import types "github.com/OctoSucker/octosucker/internal/runtime/model"

func phaseForPlannedTool(tool string) types.TaskPhase {
	switch tool {
	case "list_tool_providers", "list_tools_for_provider", "list_skills", "read_skill", "reload_skills":
		return types.TaskPhaseDiscovery
	case "natural_language_reply":
		return types.TaskPhaseSynthesis
	default:
		return types.TaskPhaseExecution
	}
}
