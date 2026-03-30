package graph

import (
	"strings"
	"unicode"
)

func routingIntentWordSet(t string) map[string]struct{} {
	m := make(map[string]struct{})
	f := func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '.' }
	for _, w := range strings.FieldsFunc(strings.ToLower(t), f) {
		if len(w) >= 2 {
			m[w] = struct{}{}
		}
	}
	return m
}

func routingWordOverlapRatio(a, b map[string]struct{}) float64 {
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
