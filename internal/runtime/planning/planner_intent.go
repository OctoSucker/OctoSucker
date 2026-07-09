package planning

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	rt "github.com/OctoSucker/octosucker/internal/runtime/toolrouting"
	"github.com/OctoSucker/octosucker/internal/tools/builtin/cronjob"
	execbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/exec"
	skillsbuiltin "github.com/OctoSucker/octosucker/internal/tools/builtin/skills"
	"github.com/OctoSucker/octosucker/internal/tools/builtin/thinker"
)

const marketIntelAnalyzeTool = "analyze_us_market_intel"

type deterministicIntentRule struct {
	name  string
	match func(string) (string, map[string]any, bool)
}

func (p *Planner) deterministicIntentStep(input string) (*types.PlanStep, bool) {
	for _, rule := range p.deterministicIntentRules() {
		tool, args, ok := rule.match(input)
		if !ok {
			continue
		}
		return newPendingPlanStep(rule.name, rt.Node{Tool: tool}, args), true
	}
	return nil, false
}

func (p *Planner) deterministicIntentRules() []deterministicIntentRule {
	return []deterministicIntentRule{
		{name: "发送文本到飞书", match: feishuWebhookSendIntent},
		{name: "执行安全本地命令", match: safeCommandIntent},
		{name: "列出定时任务", match: cronListIntent},
		{name: "查看 skills 根目录", match: skillsRootIntent},
		{name: "提取实体相关性", match: entityCorrelationExtractionIntent},
	}
}

func (p *Planner) usMarketAnalysisWorkflowStep(task *types.Task) (*types.PlanStep, bool) {
	if task == nil || !isUSMarketIntelFeishuRequest(task.UserInput) {
		return nil, false
	}
	if task.Plan == nil || !task.Plan.HasSteps() {
		return usMarketScanStep(task.UserInput), true
	}
	last := task.Plan.LastDoneStep()
	if last == nil {
		return nil, false
	}
	if last.Node.Tool == execbuiltin.ToolName && isUSMarketScanCommand(last.Arguments) {
		stdout := commandStdout(last)
		if strings.TrimSpace(stdout) == "" {
			return nil, false
		}
		return newPendingPlanStep("分析美股扫描结果的交易价值", rt.Node{Tool: marketIntelAnalyzeTool}, map[string]any{
			"market_json": stdout,
			"user_goal":   task.UserInput,
		}), true
	}
	if last.Node.Tool == marketIntelAnalyzeTool {
		shouldSend, message := marketIntelAnalysisDecision(last)
		if shouldSend && strings.TrimSpace(message) != "" {
			return newPendingPlanStep("发送美股情报分析到飞书", rt.Node{Tool: execbuiltin.ToolName}, map[string]any{
				"program": feishuSendProgram(),
				"args": []any{
					"text",
					"--message",
					message,
				},
				"timeout_sec": float64(30),
			}), true
		}
	}
	return nil, false
}

func usMarketScanStep(input string) *types.PlanStep {
	return newPendingPlanStep("抓取美股市场结构化信号", rt.Node{Tool: execbuiltin.ToolName}, map[string]any{
		"program": usMarketProgram(),
		"args": []any{
			"scan",
			"--tickers",
			usMarketTickers(input),
			"--forms",
			"8-K,10-Q,10-K,S-1,S-3,424B5,424B3,13D,SC 13D,13G,SC 13G,4,144",
			"--limit",
			"5",
			"--macro",
		},
		"timeout_sec": float64(60),
	})
}

func isUSMarketIntelFeishuRequest(input string) bool {
	s := strings.ToLower(strings.TrimSpace(input))
	hasMarket := strings.Contains(s, "美股") || strings.Contains(s, "us market") || strings.Contains(s, "market")
	hasIntel := strings.Contains(s, "情报") || strings.Contains(s, "新闻") || strings.Contains(s, "异动") || strings.Contains(s, "scan") || strings.Contains(s, "report")
	hasFeishu := strings.Contains(s, "飞书") || strings.Contains(s, "feishu") || strings.Contains(s, "lark")
	hasSend := strings.Contains(s, "发送") || strings.Contains(s, "推送") || strings.Contains(s, "send")
	return hasMarket && hasIntel && hasFeishu && hasSend
}

func isUSMarketScanCommand(args map[string]any) bool {
	if args == nil {
		return false
	}
	program, _ := args["program"].(string)
	if !strings.Contains(filepath.Base(program), "us-market") {
		return false
	}
	rawArgs, ok := args["args"].([]any)
	if !ok || len(rawArgs) == 0 {
		return false
	}
	first, _ := rawArgs[0].(string)
	return first == "scan"
}

func commandStdout(step *types.PlanStep) string {
	if step == nil {
		return ""
	}
	m, ok := step.ToolResult.Output.(map[string]any)
	if !ok {
		return ""
	}
	stdout, _ := m["stdout"].(string)
	return stdout
}

func marketIntelAnalysisDecision(step *types.PlanStep) (bool, string) {
	if step == nil {
		return false, ""
	}
	m, ok := step.ToolResult.Output.(map[string]any)
	if !ok {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(step.PrimaryText()), &decoded); err == nil {
			m = decoded
			ok = true
		}
	}
	if !ok {
		return false, ""
	}
	shouldSend, _ := m["should_send"].(bool)
	message, _ := m["message"].(string)
	return shouldSend, strings.TrimSpace(message)
}

func usMarketReportToFeishuIntent(input string) (string, map[string]any, bool) {
	s := strings.TrimSpace(input)
	if s == "" || utf8.RuneCountInString(s) > 4000 {
		return "", nil, false
	}
	lower := strings.ToLower(s)
	hasMarket := strings.Contains(lower, "美股") || strings.Contains(lower, "us market") || strings.Contains(lower, "market")
	hasIntel := strings.Contains(lower, "情报") || strings.Contains(lower, "新闻") || strings.Contains(lower, "异动") || strings.Contains(lower, "scan") || strings.Contains(lower, "report")
	hasFeishu := strings.Contains(lower, "飞书") || strings.Contains(lower, "feishu") || strings.Contains(lower, "lark")
	hasSend := strings.Contains(lower, "发送") || strings.Contains(lower, "推送") || strings.Contains(lower, "send")
	if !hasMarket || !hasIntel || !hasFeishu || !hasSend {
		return "", nil, false
	}
	tickers := usMarketTickers(input)
	forms := "8-K,10-Q,10-K,S-1,424B5,13D,13G,4"
	script := strings.Join([]string{
		"set -e",
		shellQuote(usMarketProgram()) + " report --tickers " + shellQuote(tickers) + " --forms " + shellQuote(forms) + " --limit 3 | " + shellQuote(feishuSendProgram()) + " text --stdin",
	}, "\n")
	return execbuiltin.ToolName, map[string]any{
		"program": "/bin/sh",
		"args": []any{
			"-c",
			script,
		},
		"timeout_sec": float64(60),
	}, true
}

func usMarketProgram() string {
	if p := strings.TrimSpace(os.Getenv("US_MARKET_BIN")); p != "" {
		return p
	}
	if p, err := exec.LookPath("us-market"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, "go", "bin", "us-market")
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return "us-market"
}

func usMarketTickers(input string) string {
	for _, sep := range []string{"tickers=", "tickers:", "tickers：", "股票=", "股票:", "股票："} {
		lower := strings.ToLower(input)
		if idx := strings.Index(lower, sep); idx >= 0 {
			raw := strings.TrimSpace(input[idx+len(sep):])
			fields := strings.Fields(raw)
			if len(fields) == 0 {
				return "NVDA,AAPL,MSFT,GOOGL,AMZN,META,TSLA,AVGO,AMD,SMCI"
			}
			raw = fields[0]
			raw = strings.Trim(raw, "，,。.;；")
			if raw != "" {
				return strings.ToUpper(strings.ReplaceAll(raw, "，", ","))
			}
		}
	}
	return "NVDA,AAPL,MSFT,GOOGL,AMZN,META,TSLA,AVGO,AMD,SMCI"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func feishuWebhookSendIntent(input string) (string, map[string]any, bool) {
	s := strings.TrimSpace(input)
	if s == "" || utf8.RuneCountInString(s) > 8000 {
		return "", nil, false
	}
	lower := strings.ToLower(s)
	hasFeishu := strings.Contains(lower, "飞书") || strings.Contains(lower, "feishu") || strings.Contains(lower, "lark")
	hasSend := strings.Contains(lower, "发送") || strings.Contains(lower, "推送") || strings.Contains(lower, "send")
	if !hasFeishu || !hasSend {
		return "", nil, false
	}
	message := feishuMessagePayload(s)
	if message == "" {
		return "", nil, false
	}
	return execbuiltin.ToolName, map[string]any{
		"program": feishuSendProgram(),
		"args": []any{
			"text",
			"--message",
			message,
		},
	}, true
}

func feishuSendProgram() string {
	if p := strings.TrimSpace(os.Getenv("FEISHU_SEND_BIN")); p != "" {
		return p
	}
	if p, err := exec.LookPath("feishu-send"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, "go", "bin", "feishu-send")
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return "feishu-send"
}

func feishuMessagePayload(input string) string {
	for _, sep := range []string{":", "："} {
		if _, after, ok := strings.Cut(input, sep); ok {
			return strings.TrimSpace(after)
		}
	}
	lower := strings.ToLower(input)
	for _, marker := range []string{"发送到飞书群", "发送到飞书", "推送到飞书群", "推送到飞书", "send to feishu", "send to lark"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			return strings.TrimSpace(input[idx+len(marker):])
		}
	}
	return ""
}

func entityCorrelationExtractionIntent(input string) (string, map[string]any, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" || utf8.RuneCountInString(s) > 1200 {
		return "", nil, false
	}
	if isCapabilityListingRequest(s) {
		return "", nil, false
	}
	hasExtractIntent := strings.Contains(s, "提取") || strings.Contains(s, "抽取") || strings.Contains(s, "解析") || strings.Contains(s, "extract") || strings.Contains(s, "parse")
	hasRelationIntent := strings.Contains(s, "关系") || strings.Contains(s, "相关性") || strings.Contains(s, "entity") || strings.Contains(s, "relation") || strings.Contains(s, "correlation")
	if !hasExtractIntent || !hasRelationIntent {
		return "", nil, false
	}
	text := extractionPayload(input)
	if text == "" {
		text = strings.TrimSpace(input)
	}
	tool := thinker.ToolExtractEntityCorrelations
	if looksSingleRelation(text) {
		tool = thinker.ToolParseSingleEntityCorrelation
	}
	return tool, map[string]any{"text": text}, true
}

func isCapabilityListingRequest(s string) bool {
	hasList := strings.Contains(s, "列出") || strings.Contains(s, "有哪些") || strings.Contains(s, "list")
	hasTool := strings.Contains(s, "工具") || strings.Contains(s, "tool")
	return hasList && hasTool
}

func extractionPayload(input string) string {
	for _, sep := range []string{":", "："} {
		if before, after, ok := strings.Cut(input, sep); ok {
			b := strings.ToLower(before)
			if strings.Contains(b, "提取") || strings.Contains(b, "抽取") || strings.Contains(b, "解析") || strings.Contains(b, "extract") || strings.Contains(b, "parse") {
				return strings.TrimSpace(after)
			}
		}
	}
	return ""
}

func looksSingleRelation(text string) bool {
	for _, marker := range []string{"，", ",", "；", ";", "\n", " and ", "以及", "并且"} {
		if strings.Contains(text, marker) {
			return false
		}
	}
	return utf8.RuneCountInString(text) <= 80
}

func cronListIntent(input string) (string, map[string]any, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	hasList := strings.Contains(s, "列出") || strings.Contains(s, "有哪些") || strings.Contains(s, "查看") || strings.Contains(s, "list")
	hasCron := strings.Contains(s, "cron") || strings.Contains(s, "定时") || strings.Contains(s, "计划任务")
	if hasList && hasCron {
		return cronjob.ToolListJobs, map[string]any{}, true
	}
	return "", nil, false
}

func skillsRootIntent(input string) (string, map[string]any, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	hasRoot := strings.Contains(s, "根目录") || strings.Contains(s, "root")
	hasSkill := strings.Contains(s, "skill") || strings.Contains(s, "技能")
	if hasRoot && hasSkill {
		return skillsbuiltin.ToolGetRootDir, map[string]any{}, true
	}
	return "", nil, false
}

func safeCommandIntent(input string) (string, map[string]any, bool) {
	s := strings.TrimSpace(input)
	lower := strings.ToLower(s)
	for _, prefix := range []string{"运行", "执行", "run", "exec"} {
		cmd, ok := strings.CutPrefix(lower, prefix)
		if !ok {
			continue
		}
		cmd = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cmd), "命令"))
		cmd = strings.TrimSpace(strings.TrimPrefix(cmd, ":"))
		cmd = strings.TrimSpace(strings.TrimPrefix(cmd, "："))
		program, args, ok := parseSafeCommand(cmd)
		if !ok {
			return "", nil, false
		}
		argv := make([]any, len(args))
		for i := range args {
			argv[i] = args[i]
		}
		return execbuiltin.ToolName, map[string]any{"program": program, "args": argv}, true
	}
	return "", nil, false
}

func parseSafeCommand(cmd string) (string, []string, bool) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil, false
	}
	switch fields[0] {
	case "pwd", "date":
		if len(fields) == 1 {
			return fields[0], []string{}, true
		}
	case "ls":
		if len(fields) <= 2 {
			if len(fields) == 2 && strings.ContainsAny(fields[1], ";|&><`$") {
				return "", nil, false
			}
			return fields[0], fields[1:], true
		}
	}
	return "", nil, false
}
