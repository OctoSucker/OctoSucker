package utils

import (
	"strings"
	"unicode"
)

// RecallLexicalTerms splits a recall query into lowercase terms (CJK-friendly fallback).
func RecallLexicalTerms(q string) []string {
	q = strings.ToLower(q)
	if q == "" {
		return nil
	}
	parts := strings.FieldsFunc(q, func(r rune) bool {
		if unicode.IsSpace(r) {
			return true
		}
		switch r {
		case ',', '.', ';', '?', '!', '，', '。', '、':
			return true
		default:
			return false
		}
	})
	if len(parts) > 0 {
		return parts
	}
	return []string{q}
}

// RecallLexicalScoreChunk scores how well a lowercased chunk matches query terms.
func RecallLexicalScoreChunk(chunkLower string, terms []string, fullLower string) int {
	sc := 0
	for _, t := range terms {
		if t == "" {
			continue
		}
		if strings.Contains(chunkLower, t) {
			sc += len(t)
		}
	}
	if fullLower != "" && strings.Contains(chunkLower, fullLower) {
		sc += len(fullLower) + 2
	}
	return sc
}
