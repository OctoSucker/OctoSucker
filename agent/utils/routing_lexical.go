package utils

import (
	"strings"
	"unicode"
)

// RoutingIntentWordSet tokenizes intent text into a set of words (len>=2).
func RoutingIntentWordSet(t string) map[string]struct{} {
	m := make(map[string]struct{})
	f := func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '.' }
	for _, w := range strings.FieldsFunc(strings.ToLower(t), f) {
		if len(w) >= 2 {
			m[w] = struct{}{}
		}
	}
	return m
}

// RoutingWordOverlapRatio is |a∩b|/|a| for non-empty a (0 if a empty).
func RoutingWordOverlapRatio(a, b map[string]struct{}) float64 {
	if len(a) == 0 {
		return 0
	}
	n := 0
	for k := range a {
		if _, ok := b[k]; ok {
			n++
		}
	}
	return float64(n) / float64(len(a))
}
