package recall

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"

	"github.com/OctoSucker/agent/pkg/llmclient"
	rtutils "github.com/OctoSucker/agent/utils"
)

type RecallCorpus struct {
	mu         sync.RWMutex
	texts      []string
	embeddings [][]float32
	Embedder   *llmclient.OpenAI
	db         *sql.DB
}

func NewRecallCorpus(embedder *llmclient.OpenAI, db *sql.DB) (*RecallCorpus, error) {
	m := &RecallCorpus{
		texts:      make([]string, 0),
		embeddings: make([][]float32, 0),
		Embedder:   embedder,
		db:         db,
	}
	if db != nil {
		if err := m.loadAll(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *RecallCorpus) Write(ctx context.Context, text string) error {
	t := text
	if t == "" {
		return nil
	}
	var vec []float32
	if m.Embedder != nil {
		v, err := m.Embedder.Embed(ctx, t)
		if err != nil {
			return err
		}
		if len(v) > 0 {
			vec = v
		}
	}
	m.mu.Lock()
	m.texts = append(m.texts, t)
	if len(vec) > 0 {
		m.embeddings = append(m.embeddings, vec)
	} else {
		m.embeddings = append(m.embeddings, nil)
	}
	m.mu.Unlock()
	return m.persistInsert(t, vec)
}

func (m *RecallCorpus) Recall(ctx context.Context, query string, k int) ([]string, error) {
	if k <= 0 {
		k = 5
	}
	m.mu.RLock()
	texts := append([]string(nil), m.texts...)
	vectors := make([][]float32, len(m.embeddings))
	for i := range m.embeddings {
		if m.embeddings[i] != nil {
			vectors[i] = append([]float32(nil), m.embeddings[i]...)
		}
	}
	m.mu.RUnlock()
	if len(texts) == 0 {
		return nil, nil
	}
	if query == "" {
		var out []string
		for i := len(texts) - 1; i >= 0 && len(out) < k; i-- {
			out = append(out, texts[i])
		}
		return out, nil
	}
	if m.Embedder != nil {
		qvec, err := m.Embedder.Embed(ctx, query)
		if err != nil {
			return nil, err
		}
		if len(qvec) == 0 {
			return nil, nil
		}
		type scored struct {
			idx   int
			text  string
			score float64
		}
		var cand []scored
		for i, vec := range vectors {
			if vec == nil {
				continue
			}
			sim := rtutils.CosineFloat32(qvec, vec)
			cand = append(cand, scored{i, texts[i], sim})
		}
		sort.SliceStable(cand, func(a, b int) bool {
			if cand[a].score != cand[b].score {
				return cand[a].score > cand[b].score
			}
			return cand[a].idx > cand[b].idx
		})
		var out []string
		seen := make(map[string]struct{})
		for i := 0; i < len(cand) && len(out) < k; i++ {
			ch := cand[i].text
			if _, dup := seen[ch]; dup {
				continue
			}
			seen[ch] = struct{}{}
			out = append(out, ch)
		}
		return out, nil
	}
	ql := strings.ToLower(query)
	terms := rtutils.RecallLexicalTerms(query)
	type row struct {
		idx   int
		text  string
		score int
	}
	var cand []row
	for i, c := range texts {
		cl := strings.ToLower(c)
		sc := rtutils.RecallLexicalScoreChunk(cl, terms, ql)
		if sc > 0 {
			cand = append(cand, row{i, c, sc})
		}
	}
	sort.SliceStable(cand, func(a, b int) bool {
		if cand[a].score != cand[b].score {
			return cand[a].score > cand[b].score
		}
		return cand[a].idx > cand[b].idx
	})
	var out []string
	seen := make(map[string]struct{})
	for i := 0; i < len(cand) && len(out) < k; i++ {
		ch := cand[i].text
		if _, dup := seen[ch]; dup {
			continue
		}
		seen[ch] = struct{}{}
		out = append(out, ch)
	}
	return out, nil
}
