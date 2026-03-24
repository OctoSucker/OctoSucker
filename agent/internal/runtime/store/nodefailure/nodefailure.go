package nodefailure

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// NodeFailureStats aggregates deduplicated node-level (capability + tool + context) failures for planning hints.
type NodeFailureStats struct {
	mu sync.Mutex
	db *sql.DB
	// MinFailuresForHint: rows with failure_count below this are omitted from planner hints (default 2).
	MinFailuresForHint int
	// MaxHintLines: max bullet lines appended to the planner system message (default 12).
	MaxHintLines int
}

func NewNodeFailureStats(db *sql.DB) *NodeFailureStats {
	return &NodeFailureStats{db: db, MinFailuresForHint: 2, MaxHintLines: 12}
}

func (n *NodeFailureStats) RecordFailure(capability, tool, fromCap, errMsg string) error {
	if n == nil || n.db == nil {
		return nil
	}
	capability = strings.TrimSpace(capability)
	tool = strings.TrimSpace(tool)
	fromCap = strings.TrimSpace(fromCap)
	if capability == "" || tool == "" {
		return nil
	}
	sig := normalizeErrorSignature(errMsg)
	if sig == "" {
		sig = "(empty)"
	}
	key := dedupKey(capability, tool, fromCap, sig)
	now := time.Now().Unix()
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.dbUpsertFailure(key, capability, tool, fromCap, sig, now)
}

func (n *NodeFailureStats) HintForCapabilities(caps []string, minCount, maxLines int) string {
	if n == nil || n.db == nil || len(caps) == 0 || maxLines <= 0 {
		return ""
	}
	if minCount < 1 {
		minCount = 1
	}
	uniq := map[string]struct{}{}
	var list []string
	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := uniq[c]; ok {
			continue
		}
		uniq[c] = struct{}{}
		list = append(list, c)
	}
	if len(list) == 0 {
		return ""
	}
	sort.Strings(list)
	n.mu.Lock()
	defer n.mu.Unlock()
	rows, err := n.dbQueryHintRows(list, minCount, maxLines)
	if err != nil {
		return ""
	}
	return formatHintBlock(rows, maxLines)
}

func (n *NodeFailureStats) PlannerHint(caps []string) string {
	if n == nil {
		return ""
	}
	min := n.MinFailuresForHint
	if min < 1 {
		min = 2
	}
	max := n.MaxHintLines
	if max <= 0 {
		max = 12
	}
	return n.HintForCapabilities(caps, min, max)
}

func normalizeErrorSignature(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		lastSpace = false
		b.WriteRune(unicode.ToLower(r))
	}
	out := b.String()
	runes := []rune(out)
	if len(runes) > 512 {
		out = string(runes[:512]) + "…"
	}
	return out
}

func dedupKey(capability, tool, fromCap, errSig string) string {
	h := sha256.Sum256([]byte(capability + "\x1f" + tool + "\x1f" + fromCap + "\x1f" + errSig))
	return hex.EncodeToString(h[:])
}

type hintRow struct {
	capID   string
	tool    string
	fromCap string
	errSig  string
	cnt     int
}

func formatHintBlock(rows []hintRow, maxLines int) string {
	var b strings.Builder
	nLines := 0
	for _, r := range rows {
		if nLines == 0 {
			b.WriteString("[历史节点失败统计（同一条目已去重累计次数）]\n")
			b.WriteString("规划时请尽量避免重复下列易失败组合；若必须处理，请换工具、换能力或调整参数。\n")
		}
		from := r.fromCap
		if from == "" {
			from = "∅"
		}
		shortErr := r.errSig
		if len([]rune(shortErr)) > 120 {
			rr := []rune(shortErr)
			shortErr = string(rr[:120]) + "…"
		}
		fmt.Fprintf(&b, "- 次数=%d：from=%s → capability=%s tool=%s | %s\n", r.cnt, from, r.capID, r.tool, shortErr)
		nLines++
		if nLines >= maxLines {
			break
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}
