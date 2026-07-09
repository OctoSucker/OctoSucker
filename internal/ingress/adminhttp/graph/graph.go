package graph

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"github.com/OctoSucker/octosucker/internal/storage"
)

// Register adds GET /api/graph. get may be nil or return nil Reader → 500 "workspace db not available".
func Register(mux *http.ServeMux, get func() storage.KnowledgeGraphReader) {
	mux.Handle("GET /api/graph", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var kg storage.KnowledgeGraphReader
		if get != nil {
			kg = get()
		}
		if kg == nil {
			http.Error(w, "workspace db not available", http.StatusInternalServerError)
			return
		}
		nrows, err := kg.KnowledgeGraphNodesSelectAll()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		erows, err := kg.KnowledgeGraphEdgesSelectAll()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		payload := payload{
			Nodes: make([]node, 0, len(nrows)),
			Edges: make([]edge, 0, len(erows)),
		}
		for _, row := range nrows {
			payload.Nodes = append(payload.Nodes, node{ID: row.ID, Label: row.ID})
		}
		for _, row := range erows {
			payload.Edges = append(payload.Edges, edge{
				From: row.FromID, To: row.ToID, Positive: row.Positive,
			})
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(true)
		if err := enc.Encode(payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(buf.Bytes()); err != nil {
			log.Printf("adminhttp graph /api/graph write: %v", err)
		}
	}))
}

type payload struct {
	Nodes []node `json:"nodes"`
	Edges []edge `json:"edges"`
}

type node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type edge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Positive bool   `json:"positive"`
}
