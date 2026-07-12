package tools

import (
	"path/filepath"
	"strings"

	"github.com/OctoSucker/octosucker/internal/toolcontract"
)

type PolicyAssessment struct {
	Capabilities []string `json:"capabilities"`
	Risk         string   `json:"risk"`
	Summary      string   `json:"summary"`
	OutputTrust  string   `json:"output_trust"`
}

func AssessToolCall(tool string, arguments map[string]any) PolicyAssessment {
	tool = strings.TrimSpace(tool)
	switch tool {
	case "run_command":
		return assessRunCommand(arguments)
	case "activate_skill", "read_skill_resource":
		return PolicyAssessment{
			Capabilities: []string{"workspace_read", "instructions"},
			Risk:         "low",
			Summary:      "reads workspace-owned agent instructions",
			OutputTrust:  toolcontract.TrustWorkspaceInstruction,
		}
	case "list_skills", "get_skills_root_dir", "list_tool_providers", "list_tools_for_provider", "list_cronjobs":
		return PolicyAssessment{
			Capabilities: []string{"runtime_metadata"},
			Risk:         "low",
			Summary:      "reads runtime-owned metadata",
			OutputTrust:  toolcontract.TrustRuntimeMetadata,
		}
	case "send_telegram_message":
		return PolicyAssessment{
			Capabilities: []string{"external_send"},
			Risk:         "high",
			Summary:      "sends content to Telegram",
			OutputTrust:  toolcontract.TrustRuntimeMetadata,
		}
	default:
		return PolicyAssessment{
			Capabilities: []string{"tool"},
			Risk:         "low",
			Summary:      "structured tool call",
			OutputTrust:  toolcontract.TrustUntrustedData,
		}
	}
}

func assessRunCommand(arguments map[string]any) PolicyAssessment {
	program, _ := arguments["program"].(string)
	base := filepath.Base(strings.TrimSpace(program))
	caps := []string{"process", "filesystem_possible", "network_possible"}
	risk := "high"
	summary := "runs an arbitrary local command"
	switch base {
	case "git":
		caps = append(caps, "git")
		if commandArgsContain(arguments, "push") {
			caps = append(caps, "external_send")
			risk = "high"
			summary = "pushes local repository state to a remote"
		}
	case "curl", "wget":
		caps = append(caps, "network")
		risk = "high"
		summary = "performs arbitrary network I/O"
	case "rm", "mv", "cp":
		caps = append(caps, "fs_write")
		risk = "high"
		summary = "mutates local filesystem state"
	}
	if isShell(base) {
		caps = append(caps, "shell_script")
		if shellCommandMentions(arguments, "curl") || shellCommandMentions(arguments, "wget") {
			caps = append(caps, "network")
			risk = "high"
		}
	}
	return PolicyAssessment{Capabilities: dedupe(caps), Risk: risk, Summary: summary, OutputTrust: toolcontract.TrustUntrustedData}
}

func commandArgsContain(arguments map[string]any, needle string) bool {
	raw, ok := arguments["args"].([]any)
	if !ok {
		return false
	}
	for _, item := range raw {
		if s, _ := item.(string); strings.TrimSpace(s) == needle {
			return true
		}
	}
	return false
}

func shellCommandMentions(arguments map[string]any, needle string) bool {
	raw, ok := arguments["args"].([]any)
	if !ok || len(raw) < 2 {
		return false
	}
	flag, _ := raw[0].(string)
	script, _ := raw[1].(string)
	return flag == "-c" && strings.Contains(script, needle)
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
