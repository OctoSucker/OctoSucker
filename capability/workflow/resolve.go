package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OctoSucker/octosucker-utils/graph"
	"github.com/OctoSucker/octosucker/capability/registry"
)

const DefaultBindingCachePath = "workspace/workflow_bindings.json"

func BindingCachePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return DefaultBindingCachePath
	}
	return path
}

type bindingCache struct {
	Signature string            `json:"signature"`
	Bindings  map[string]string `json:"bindings"`
}

func ResolveTemplateTools(
	ctx context.Context,
	templates []WorkflowTemplate,
	reg *registry.Registry,
	embedder interface {
		Embed(ctx context.Context, text string) ([]float32, error)
	},
	similarity func(a, b []float32) float32,
	cachePath string,
) {
	if len(templates) == 0 || reg == nil || embedder == nil || similarity == nil {
		return
	}
	toolNodes := make([]*graph.Node, 0, len(reg.List()))
	for _, node := range reg.List() {
		if node == nil || node.Name == "" || node.Tool == "" {
			continue
		}
		if strings.HasPrefix(node.Name, "skill_") || node.Name == registry.EntryNodeName || node.Name == "finish" {
			continue
		}
		toolNodes = append(toolNodes, node)
	}
	if len(toolNodes) == 0 {
		return
	}

	nodeEmbeddings := make(map[string][]float32, len(toolNodes))
	for _, node := range toolNodes {
		text := strings.TrimSpace(node.Name + "\n" + node.Description + "\n" + node.Tool)
		vec, err := embedder.Embed(ctx, text)
		if err != nil || len(vec) == 0 {
			continue
		}
		nodeEmbeddings[node.Name] = vec
	}
	if len(nodeEmbeddings) == 0 {
		return
	}

	type sigStep struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
		Tool        string   `json:"tool"`
	}
	type sigTpl struct {
		Name        string    `json:"name"`
		Description string    `json:"description"`
		Steps       []sigStep `json:"steps"`
	}
	type sigNode struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Tool        string `json:"tool"`
	}
	payload := struct {
		Templates []sigTpl  `json:"templates"`
		Tools     []sigNode `json:"tools"`
	}{}
	payload.Templates = make([]sigTpl, 0, len(templates))
	for _, tpl := range templates {
		steps := make([]sigStep, 0, len(tpl.Steps))
		for _, step := range tpl.Steps {
			keywords := append([]string(nil), step.Keywords...)
			sort.Strings(keywords)
			steps = append(steps, sigStep{
				Name:        strings.TrimSpace(step.Name),
				Description: strings.TrimSpace(step.Description),
				Keywords:    keywords,
				Tool:        strings.TrimSpace(step.Tool),
			})
		}
		payload.Templates = append(payload.Templates, sigTpl{
			Name:        strings.TrimSpace(tpl.Name),
			Description: strings.TrimSpace(tpl.Description),
			Steps:       steps,
		})
	}
	sort.SliceStable(payload.Templates, func(i, j int) bool {
		return payload.Templates[i].Name < payload.Templates[j].Name
	})
	payload.Tools = make([]sigNode, 0, len(toolNodes))
	for _, n := range toolNodes {
		if n == nil {
			continue
		}
		payload.Tools = append(payload.Tools, sigNode{
			Name:        strings.TrimSpace(n.Name),
			Description: strings.TrimSpace(n.Description),
			Tool:        strings.TrimSpace(n.Tool),
		})
	}
	sort.SliceStable(payload.Tools, func(i, j int) bool {
		if payload.Tools[i].Tool == payload.Tools[j].Tool {
			return payload.Tools[i].Name < payload.Tools[j].Name
		}
		return payload.Tools[i].Tool < payload.Tools[j].Tool
	})
	sigB, _ := json.Marshal(payload)
	sigSum := sha256.Sum256(sigB)
	sig := hex.EncodeToString(sigSum[:])

	var cache *bindingCache
	if data, err := os.ReadFile(cachePath); err == nil {
		var c bindingCache
		if json.Unmarshal(data, &c) == nil {
			if c.Bindings == nil {
				c.Bindings = make(map[string]string)
			}
			cache = &c
		} else {
			backupPath := cachePath + ".corrupt." + time.Now().Format("20060102150405")
			if renameErr := os.Rename(cachePath, backupPath); renameErr != nil {
				log.Printf("[workflow] binding_cache_corrupt_backup_failed path=%s err=%v", cachePath, renameErr)
			} else {
				log.Printf("[workflow] binding_cache_corrupt_backed_up from=%s to=%s", cachePath, backupPath)
			}
		}
	}
	if cache != nil && cache.Signature == sig {
		for i := range templates {
			for j := range templates[i].Steps {
				step := &templates[i].Steps[j]
				if strings.TrimSpace(step.Tool) != "" {
					continue
				}
				key := strings.TrimSpace(templates[i].Name) + "/" + strings.TrimSpace(step.Name)
				if tool := strings.TrimSpace(cache.Bindings[key]); tool != "" {
					step.Tool = tool
				}
			}
		}
		return
	}

	for i := range templates {
		for j := range templates[i].Steps {
			step := &templates[i].Steps[j]
			if strings.TrimSpace(step.Tool) != "" {
				continue
			}
			query := strings.TrimSpace(
				templates[i].Name + "\n" +
					templates[i].Description + "\n" +
					step.Name + "\n" +
					step.Description + "\n" +
					strings.Join(step.Keywords, " "),
			)
			qVec, err := embedder.Embed(ctx, query)
			if err != nil || len(qVec) == 0 {
				continue
			}

			bestTool := ""
			bestScore := float32(-2.0)
			for _, node := range toolNodes {
				nVec, ok := nodeEmbeddings[node.Name]
				if !ok || len(nVec) != len(qVec) {
					continue
				}
				score := similarity(qVec, nVec)
				if score > bestScore {
					bestScore = score
					bestTool = node.Tool
				}
			}
			if bestTool != "" {
				step.Tool = bestTool
			}
		}
	}

	bindings := make(map[string]string)
	for i := range templates {
		for j := range templates[i].Steps {
			step := templates[i].Steps[j]
			if strings.TrimSpace(step.Tool) == "" {
				continue
			}
			key := strings.TrimSpace(templates[i].Name) + "/" + strings.TrimSpace(step.Name)
			bindings[key] = step.Tool
		}
	}
	cacheToSave := &bindingCache{Signature: sig, Bindings: bindings}
	dir := filepath.Dir(cachePath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return
		}
	}
	data, err := json.MarshalIndent(cacheToSave, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".workflow_bindings_*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return
	}
	cleanup = false
}
