package interaction

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type JSONCompleter interface {
	CompleteJSON(ctx context.Context, system, user string, out any) error
}

type Planner struct {
	llm JSONCompleter
}

type Interaction struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	SubmitLabel string  `json:"submit_label,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
}

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder,omitempty"`
	HelpText    string   `json:"help_text,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type Response struct {
	Values map[string]any `json:"values,omitempty"`
}

type planJSON struct {
	Interaction *Interaction `json:"interaction"`
	Reason      string       `json:"reason,omitempty"`
}

type PlanResult struct {
	Interaction *Interaction
	Source      string
	Reason      string
}

func NewPlanner(llm JSONCompleter) (*Planner, error) {
	if llm == nil {
		return nil, fmt.Errorf("interaction planner: llm is required")
	}
	return &Planner{llm: llm}, nil
}

func (p *Planner) PlanResult(ctx context.Context, messages []string) (*PlanResult, error) {
	if p == nil || p.llm == nil {
		return &PlanResult{Source: "disabled", Reason: "planner is nil"}, nil
	}
	text := strings.TrimSpace(strings.Join(messages, "\n\n"))
	if text == "" {
		return &PlanResult{Source: "none", Reason: "empty assistant text"}, nil
	}
	var raw planJSON
	if err := p.llm.CompleteJSON(ctx, plannerSystemPrompt, text, &raw); err == nil {
		if raw.Interaction == nil {
			if fallback := numberedQuestionForm(text); fallback != nil {
				return &PlanResult{Interaction: fallback, Source: "fallback_numbered_questions", Reason: "llm returned null but assistant asked numbered questions"}, nil
			}
			return &PlanResult{Source: "llm_null", Reason: raw.Reason}, nil
		}
		if err := validate(raw.Interaction); err != nil {
			return nil, err
		}
		return &PlanResult{Interaction: raw.Interaction, Source: "llm", Reason: raw.Reason}, nil
	} else {
		if fallback := numberedQuestionForm(text); fallback != nil {
			return &PlanResult{Interaction: fallback, Source: "fallback_numbered_questions", Reason: err.Error()}, nil
		}
		return nil, err
	}
}

const plannerSystemPrompt = `You are a UI interaction planner for a low-friction AI agent product.

Your job:
- Read the assistant response.
- Decide whether the user should answer through structured UI instead of free-form chat.
- Return JSON only.

Return {"interaction": null} when:
- The assistant gave a final answer.
- The assistant only made a simple statement.
- A form would not materially reduce user effort.

Return a form interaction when:
- The assistant asks the user for missing information.
- The assistant asks multiple questions.
- The assistant includes a numbered or bulleted list of information the user must provide.
- The user needs to choose options, provide parameters, confirm details, or fill an intake flow.
- The assistant says anything equivalent to "please provide the following information".

Supported schema:
{
  "interaction": {
    "id": "short_snake_case_id",
    "type": "form",
    "title": "short user-facing title",
    "description": "optional one sentence",
    "submit_label": "short button label",
    "fields": [
      {
        "key": "short_snake_case_key",
        "label": "field label",
        "kind": "text|textarea|select",
        "required": true,
        "placeholder": "optional concrete example",
        "help_text": "optional short hint",
        "options": ["optional", "choices"]
      }
    ]
  },
  "reason": "brief private reason"
}

Rules:
- Generate at most one form.
- Generate 1 to 8 fields.
- If the assistant asks two or more concrete questions, you MUST return a form.
- Do not output HTML, Markdown, CSS, JavaScript, or executable code.
- Do not invent unsupported component types.
- Prefer simple labels and concrete placeholders.
- Include options only when the assistant response implies likely choices or "not sure" is useful.
- Use the same primary language as the assistant response for all user-facing UI text.
- The UI should reduce the user's need to organize a complex reply.`

var keyRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var numberedQuestionRE = regexp.MustCompile(`(?m)^\s*\d+[\.,．、]\s*(?:\*\*)?([^：:\*\n]+)(?:\*\*)?\s*[：:]\s*(.+)$`)

func validate(in *Interaction) error {
	if in == nil {
		return nil
	}
	in.ID = normalizeKey(in.ID, "interaction")
	in.Type = strings.TrimSpace(strings.ToLower(in.Type))
	if in.Type != "form" {
		return fmt.Errorf("unsupported interaction type %q", in.Type)
	}
	in.Title = cleanText(in.Title, 80)
	in.Description = cleanText(in.Description, 180)
	in.SubmitLabel = cleanText(in.SubmitLabel, 24)
	if in.Title == "" {
		return fmt.Errorf("interaction title is required")
	}
	if len(in.Fields) == 0 || len(in.Fields) > 8 {
		return fmt.Errorf("interaction fields must contain 1 to 8 items")
	}
	seen := map[string]struct{}{}
	for i := range in.Fields {
		field := &in.Fields[i]
		field.Key = normalizeKey(field.Key, fmt.Sprintf("field_%d", i+1))
		if _, ok := seen[field.Key]; ok {
			field.Key = fmt.Sprintf("%s_%d", field.Key, i+1)
		}
		seen[field.Key] = struct{}{}
		field.Label = cleanText(field.Label, 60)
		field.Kind = strings.TrimSpace(strings.ToLower(field.Kind))
		field.Placeholder = cleanText(field.Placeholder, 160)
		field.HelpText = cleanText(field.HelpText, 140)
		if field.Label == "" {
			return fmt.Errorf("field %d label is required", i+1)
		}
		switch field.Kind {
		case "text", "textarea", "select":
		default:
			field.Kind = "text"
		}
		field.Options = cleanOptions(field.Options)
		if field.Kind == "select" && len(field.Options) == 0 {
			field.Kind = "text"
		}
	}
	return nil
}

func normalizeKey(value, fallback string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "_")
	if keyRE.MatchString(value) {
		return value
	}
	return fallback
}

func cleanText(value string, limit int) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "<", "")
	value = strings.ReplaceAll(value, ">", "")
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func cleanOptions(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanText(value, 40)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func numberedQuestionForm(text string) *Interaction {
	matches := numberedQuestionRE.FindAllStringSubmatch(text, -1)
	if len(matches) < 2 || len(matches) > 8 {
		return nil
	}
	fields := make([]Field, 0, len(matches))
	for i, match := range matches {
		label := cleanText(match[1], 60)
		question := cleanText(match[2], 160)
		if label == "" || question == "" {
			return nil
		}
		fields = append(fields, Field{
			Key:         fmt.Sprintf("field_%d", i+1),
			Label:       label,
			Kind:        "textarea",
			Required:    true,
			Placeholder: question,
		})
	}
	form := &Interaction{
		ID:          "clarification_form",
		Type:        "form",
		Title:       "补充任务所需信息",
		Description: "按项填写即可，不需要重新组织完整说明。",
		SubmitLabel: "提交并继续",
		Fields:      fields,
	}
	if err := validate(form); err != nil {
		return nil
	}
	return form
}

func RenderResponse(resp Response, form *Interaction) string {
	if len(resp.Values) == 0 {
		return ""
	}
	labels := map[string]string{}
	if form != nil {
		for _, field := range form.Fields {
			labels[field.Key] = field.Label
		}
	}
	keys := make([]string, 0, len(resp.Values))
	for key := range resp.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := []string{"用户已通过表单补充以下信息："}
	for _, key := range keys {
		value := strings.TrimSpace(fmt.Sprint(resp.Values[key]))
		if value == "" {
			value = "未填写"
		}
		label := labels[key]
		if label == "" {
			label = key
		}
		lines = append(lines, fmt.Sprintf("- %s：%s", label, value))
	}
	lines = append(lines, "请基于这些结构化信息继续推进任务；如果仍缺关键信息，请继续返回需要补充的信息。")
	return strings.Join(lines, "\n")
}
