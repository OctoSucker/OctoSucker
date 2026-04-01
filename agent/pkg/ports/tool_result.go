package ports

import (
	"encoding/json"
	"fmt"

	"github.com/OctoSucker/agent/utils"
)

type ToolResult struct {
	Output any
	Err    error
}

// CompactForLLM returns Output squeezed for LLM context: structured values are walked
// so string leaves lose decorative CLI borders, then the result is JSON (indented when
// possible) or plain text, truncated to PrimaryTextMaxRunes. Err is ignored.
// Nil Output yields "", nil.
func (res *ToolResult) CompactForLLM() string {
	if res == nil {
		return ""
	}
	if res.Err != nil {
		return res.Err.Error()
	}
	v := utils.CompactStructuredForLLM(res.Output)
	if s, ok := v.(string); ok {
		return utils.TruncateRunes(s, utils.PrimaryTextMaxRunes)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		var errCompact error
		b, errCompact = json.Marshal(v)
		if errCompact != nil {
			return fmt.Sprintf("json marshal error: %v", res.Output)
		}
	}
	return utils.TruncateRunes(string(b), utils.PrimaryTextMaxRunes)
}
