package utils

import (
	"strings"
	"testing"
)

func TestCompactDecorativeLines_collapsesBoxDrawingRuns(t *testing.T) {
	in := "lead\n┌───┬───┐\n│ a │ b │\n├───┼───┤\n│ c │ d │\n└───┴───┘\ntail"
	out := CompactDecorativeLines(in)
	if strings.Count(out, "…") < 1 {
		t.Fatalf("expected at least one collapsed border run, got %q", out)
	}
	if strings.Contains(out, "┌") || strings.Contains(out, "└") {
		t.Fatalf("pure border lines should be removed: %q", out)
	}
	if !strings.Contains(out, "lead") || !strings.Contains(out, "tail") {
		t.Fatalf("expected to keep text lines: %q", out)
	}
	if !strings.Contains(out, "│ a │") {
		t.Fatalf("expected to keep cell row: %q", out)
	}
}

func TestCompactDecorativeLines_keepsContentRows(t *testing.T) {
	row := "│ 2038563188858593371 │ Rational314159  │ 震惊！ │"
	out := CompactDecorativeLines(row)
	if out != row {
		t.Fatalf("content row should stay intact, got %q", out)
	}
}

func TestCompactStructuredForLLM_execShape(t *testing.T) {
	v := map[string]any{
		"exit_code": 0,
		"stderr":    "",
		"stdout": "twitter/timeline\n┌───┐\n│ x │\n└───┘\nfooter line",
	}
	out := CompactStructuredForLLM(v)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatal("expected map")
	}
	if m["exit_code"] != 0 {
		t.Fatalf("exit_code: %v", m["exit_code"])
	}
	stdout, _ := m["stdout"].(string)
	if strings.Contains(stdout, "┌") || strings.Contains(stdout, "└") {
		t.Fatalf("borders should collapse: %q", stdout)
	}
	if !strings.Contains(stdout, "twitter") || !strings.Contains(stdout, "footer") {
		t.Fatalf("expected content preserved: %q", stdout)
	}
}
