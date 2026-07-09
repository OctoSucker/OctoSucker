package marketintel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	types "github.com/OctoSucker/octosucker/internal/runtime/model"
	"github.com/OctoSucker/octosucker/pkg/llmclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolAnalyzeUSMarketIntel = "analyze_us_market_intel"

type Runner struct {
	llm *llmclient.OpenAI
}

func NewRunner(llm *llmclient.OpenAI) (*Runner, error) {
	if llm == nil {
		return nil, fmt.Errorf("marketintel builtin: llm client is required")
	}
	return &Runner{llm: llm}, nil
}

func (r *Runner) Name() (string, string) {
	return "market_intel", "LLM analysis of structured market data into trade-relevant intelligence."
}

func (r *Runner) HasTool(name string) bool {
	return strings.TrimSpace(name) == ToolAnalyzeUSMarketIntel
}

func (r *Runner) Tool(tool string) (*mcp.Tool, error) {
	if strings.TrimSpace(tool) != ToolAnalyzeUSMarketIntel {
		return nil, fmt.Errorf("marketintel builtin: unknown tool %q", tool)
	}
	return &mcp.Tool{
		Name:        ToolAnalyzeUSMarketIntel,
		Description: "Analyze us-market scan JSON and decide what is worth sending as concise trade-relevant intelligence.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market_json": map[string]any{
					"type":        "string",
					"description": "Raw JSON output from `us-market scan`.",
				},
				"user_goal": map[string]any{
					"type":        "string",
					"description": "Original user goal or reporting instruction.",
				},
			},
			"required":             []string{"market_json"},
			"additionalProperties": false,
		},
	}, nil
}

func (r *Runner) ToolList(ctx context.Context) ([]*mcp.Tool, error) {
	t, err := r.Tool(ToolAnalyzeUSMarketIntel)
	if err != nil {
		return nil, err
	}
	return []*mcp.Tool{t}, nil
}

type analysisOutput struct {
	ShouldSend bool `json:"should_send"`
	Signals    []struct {
		Ticker         string `json:"ticker"`
		Importance     string `json:"importance"`
		Event          string `json:"event"`
		WhyItMatters   string `json:"why_it_matters"`
		TradeRelevance string `json:"trade_relevance"`
		NextTrigger    string `json:"next_trigger"`
		SourceURL      string `json:"source_url"`
	} `json:"signals"`
	Message   string `json:"message"`
	Rationale string `json:"rationale"`
}

const analyzeSystem = `You are a trading-oriented U.S. market intelligence analyst.

Input is structured JSON from a market data collector. Your job is NOT to summarize everything.
Your job is to decide what is worth interrupting a trading/research group chat.

Rules:
- Use Chinese for message and explanations.
- Do not invent financial facts, price moves, earnings surprises, or market reactions not present in the input.
- Prefer "no send" when the data is routine, stale, weak, or not decision-useful.
- Ordinary 10-Q/10-K cover filings, Form 4, Form 144, and generic 9.01 attachments are usually not worth sending unless paired with a clearly meaningful event.
- Trading halts, 8-K material items, financing/registration filings, 13D activist-style disclosures, delisting/compliance risk, management changes, asset impairments, restructuring, M&A, and guidance/earnings releases can be worth sending.
- Every sent signal must include: ticker, event, why it matters, trade relevance, next trigger, and source if available.
- Trade relevance should be disciplined: say "watch/risk control/no chase" when direction is unclear.
- Keep the final Feishu message short. Max 6 signals.

Return ONLY valid JSON:
{
  "should_send": true|false,
  "signals": [
    {
      "ticker": "AAPL",
      "importance": "high|medium|low",
      "event": "...",
      "why_it_matters": "...",
      "trade_relevance": "...",
      "next_trigger": "...",
      "source_url": "..."
    }
  ],
  "message": "Feishu-ready concise Chinese message. If should_send=false, explain no high-value signal in one short sentence.",
  "rationale": "Brief internal reason for send/no-send."
}`

func (r *Runner) Invoke(ctx context.Context, localTool string, arguments map[string]any) (types.ToolResult, error) {
	if localTool != ToolAnalyzeUSMarketIntel {
		err := fmt.Errorf("marketintel builtin: unknown tool %q", localTool)
		return types.ToolResult{Err: err}, err
	}
	marketJSON, err := requiredString(arguments, "market_json")
	if err != nil {
		return types.ToolResult{Err: err}, err
	}
	userGoal, _ := optionalString(arguments, "user_goal")
	user := fmt.Sprintf("USER GOAL:\n%s\n\nUS-MARKET SCAN JSON:\n%s", userGoal, marketJSON)
	var out analysisOutput
	if err := r.llm.CompleteJSON(ctx, analyzeSystem, user, &out); err != nil {
		err = fmt.Errorf("marketintel builtin: analyze_us_market_intel: %w", err)
		return types.ToolResult{Err: err}, err
	}
	out.Message = strings.TrimSpace(out.Message)
	out.Rationale = strings.TrimSpace(out.Rationale)
	if out.Message == "" {
		if out.ShouldSend {
			out.Message = formatFallbackMessage(out)
		} else {
			out.Message = "本次扫描没有足够高价值、可用于交易判断的美股情报。"
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return types.ToolResult{Err: err}, err
	}
	var shaped map[string]any
	if err := json.Unmarshal(b, &shaped); err != nil {
		return types.ToolResult{Err: err}, err
	}
	return types.ToolResult{Output: shaped}, nil
}

func requiredString(args map[string]any, key string) (string, error) {
	if args == nil {
		return "", fmt.Errorf("marketintel builtin: arguments required")
	}
	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("marketintel builtin: %s is required", key)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("marketintel builtin: %s must be string", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("marketintel builtin: %s must be non-empty", key)
	}
	return s, nil
}

func optionalString(args map[string]any, key string) (string, bool) {
	if args == nil {
		return "", false
	}
	raw, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func formatFallbackMessage(out analysisOutput) string {
	var b strings.Builder
	b.WriteString("美股情报\n")
	for _, sig := range out.Signals {
		ticker := strings.TrimSpace(sig.Ticker)
		if ticker == "" {
			ticker = "UNKNOWN"
		}
		fmt.Fprintf(&b, "\n- %s｜%s\n", ticker, strings.TrimSpace(sig.Event))
		if sig.WhyItMatters != "" {
			fmt.Fprintf(&b, "  重要性: %s\n", strings.TrimSpace(sig.WhyItMatters))
		}
		if sig.TradeRelevance != "" {
			fmt.Fprintf(&b, "  交易相关: %s\n", strings.TrimSpace(sig.TradeRelevance))
		}
		if sig.NextTrigger != "" {
			fmt.Fprintf(&b, "  下一触发: %s\n", strings.TrimSpace(sig.NextTrigger))
		}
		if sig.SourceURL != "" {
			fmt.Fprintf(&b, "  来源: %s\n", strings.TrimSpace(sig.SourceURL))
		}
	}
	return strings.TrimSpace(b.String())
}
