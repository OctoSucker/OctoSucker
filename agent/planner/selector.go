package planner

import (
	"context"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OctoSucker/octosucker/agent/llm"
	capgraph "github.com/OctoSucker/octosucker-utils/graph"
	capregistry "github.com/OctoSucker/octosucker/capability/registry"
)

const DefaultSelectK = 8
const defaultSemanticCandidateLimit = 24
const defaultEmbeddingCacheTTL = 10 * time.Minute

type cachedEmbedding struct {
	vec       []float32
	textHash  uint64
	updatedAt time.Time
}

type Selector struct {
	registry               *capregistry.Registry
	maxCandidates          int
	semanticCandidateLimit int
	embeddingCacheTTL      time.Duration
	embedder               embedder
	mu                     sync.RWMutex
	nodeEmbeddings         map[string]cachedEmbedding
}

type embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

func NewSelector(registry *capregistry.Registry, emb embedder, semanticCandidateLimit int) *Selector {
	max := DefaultSelectK
	if max < 3 {
		max = 3
	}
	if semanticCandidateLimit <= 0 {
		semanticCandidateLimit = defaultSemanticCandidateLimit
	}
	if semanticCandidateLimit < max {
		semanticCandidateLimit = max
	}
	return &Selector{
		registry:               registry,
		maxCandidates:          max,
		semanticCandidateLimit: semanticCandidateLimit,
		embeddingCacheTTL:      defaultEmbeddingCacheTTL,
		embedder:               emb,
		nodeEmbeddings:         make(map[string]cachedEmbedding),
	}
}

func (s *Selector) SetEmbeddingCacheTTL(ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.mu.Lock()
	s.embeddingCacheTTL = ttl
	s.mu.Unlock()
}

func (s *Selector) Select(query string, k int) []*capgraph.Node {
	if k <= 0 {
		k = s.maxCandidates
	}
	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryWords := tokenize(queryLower)

	var candidates []*capgraph.Node
	for _, name := range s.registry.EntryCapabilities() {
		if name == capregistry.EntryNodeName {
			continue
		}
		node, ok := s.registry.Get(name)
		if !ok {
			continue
		}
		candidates = append(candidates, node)
	}

	if len(queryWords) == 0 || len(candidates) == 0 {
		var out []*capgraph.Node
		for _, name := range s.registry.EntryCapabilities() {
			if name == capregistry.EntryNodeName || name == "finish" {
				continue
			}
			node, ok := s.registry.Get(name)
			if !ok {
				continue
			}
			out = append(out, node)
			if len(out) >= k {
				break
			}
		}
		finish, _ := s.registry.Get("finish")
		if finish != nil && len(out) < k {
			out = append(out, finish)
		}
		return out
	}

	pruned := candidates
	if limit := s.semanticCandidateLimit; limit > 0 && len(candidates) > limit {
		type item struct {
			node  *capgraph.Node
			score int
		}
		scored := make([]item, 0, len(candidates))
		for _, node := range candidates {
			if node == nil {
				continue
			}
			scored = append(scored, item{node: node, score: scoreNode(queryWords, node)})
		}
		sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
		scored = scored[:limit]
		pruned = make([]*capgraph.Node, 0, len(scored))
		for _, it := range scored {
			pruned = append(pruned, it.node)
		}
	}

	if s.embedder != nil {
		qVec, err := s.embedder.Embed(context.Background(), queryLower)
		if err == nil && len(qVec) > 0 {
			type scoreItem struct {
				node *capgraph.Node
				sem  float64
				lex  int
			}
			scored := make([]scoreItem, 0, len(pruned))
			now := time.Now()
			for _, node := range pruned {
				if node == nil || node.Name == "" {
					continue
				}
				text := strings.TrimSpace(node.Name + "\n" + node.Description)
				textHash := hashText(text)
				var nVec []float32
				s.mu.RLock()
				if cache, ok := s.nodeEmbeddings[node.Name]; ok {
					expired := s.embeddingCacheTTL > 0 && now.Sub(cache.updatedAt) > s.embeddingCacheTTL
					if !expired && cache.textHash == textHash {
						nVec = cache.vec
						s.mu.RUnlock()
					} else {
						s.mu.RUnlock()
						vec, embedErr := s.embedder.Embed(context.Background(), text)
						if embedErr != nil || len(vec) == 0 {
							continue
						}
						nVec = vec
						s.mu.Lock()
						s.nodeEmbeddings[node.Name] = cachedEmbedding{vec: vec, textHash: textHash, updatedAt: now}
						s.mu.Unlock()
					}
				} else {
					s.mu.RUnlock()
					vec, embedErr := s.embedder.Embed(context.Background(), text)
					if embedErr != nil || len(vec) == 0 {
						continue
					}
					nVec = vec
					s.mu.Lock()
					s.nodeEmbeddings[node.Name] = cachedEmbedding{vec: vec, textHash: textHash, updatedAt: now}
					s.mu.Unlock()
				}
				if len(nVec) != len(qVec) {
					continue
				}
				sem := float64(llm.CosineSimilarity(qVec, nVec))
				scored = append(scored, scoreItem{node: node, sem: sem, lex: scoreNode(queryWords, node)})
			}
			if len(scored) > 0 {
				sort.SliceStable(scored, func(i, j int) bool {
					if scored[i].sem == scored[j].sem {
						return scored[i].lex > scored[j].lex
					}
					return scored[i].sem > scored[j].sem
				})
				if len(scored) > k {
					scored = scored[:k]
				}
				out := make([]*capgraph.Node, 0, len(scored))
				for _, it := range scored {
					out = append(out, it.node)
				}
				hasFinish := false
				for _, n := range out {
					if n.Name == "finish" {
						hasFinish = true
						break
					}
				}
				if !hasFinish {
					finish, _ := s.registry.Get("finish")
					if finish != nil && len(out) < k {
						out = append(out, finish)
					}
				}
				return out
			}
		}
	}

	scored := make([]struct {
		node  *capgraph.Node
		score int
	}, len(candidates))
	for i, node := range candidates {
		scored[i] = struct {
			node  *capgraph.Node
			score int
		}{node: node, score: scoreNode(queryWords, node)}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	if len(scored) > k {
		scored = scored[:k]
	}
	out := make([]*capgraph.Node, 0, len(scored))
	for _, it := range scored {
		if it.score >= 0 {
			out = append(out, it.node)
		}
	}
	if len(out) == 0 {
		out = nil
		for _, name := range s.registry.EntryCapabilities() {
			if name == capregistry.EntryNodeName || name == "finish" {
				continue
			}
			node, ok := s.registry.Get(name)
			if !ok {
				continue
			}
			out = append(out, node)
			if len(out) >= k {
				break
			}
		}
		finish, _ := s.registry.Get("finish")
		if finish != nil && len(out) < k {
			out = append(out, finish)
		}
		return out
	}
	hasFinish := false
	for _, n := range out {
		if n.Name == "finish" {
			hasFinish = true
			break
		}
	}
	if !hasFinish {
		finish, _ := s.registry.Get("finish")
		if finish != nil && len(out) < k {
			out = append(out, finish)
		}
	}
	return out
}

func hashText(text string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return h.Sum64()
}

func scoreNode(queryWords []string, node *capgraph.Node) int {
	text := strings.ToLower(node.Name + " " + node.Description)
	var score int
	for _, w := range queryWords {
		if len(w) < 2 {
			continue
		}
		if strings.Contains(text, w) {
			score++
		}
	}
	return score
}

func tokenize(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '?' || r == '!'
	}) {
		w = strings.TrimSpace(w)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

func NodeNames(nodes []*capgraph.Node) []string {
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n != nil {
			names = append(names, n.Name)
		}
	}
	return names
}
