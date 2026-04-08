package httpserver

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

func (s *server) serveGraph(w http.ResponseWriter, r *http.Request) {
	var kg KnowledgeGraphReader
	if s.opts.Graph != nil {
		kg = s.opts.Graph()
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
	payload := graphPayload{
		Nodes: make([]nodePayload, 0, len(nrows)),
		Edges: make([]edgePayload, 0, len(erows)),
	}
	for _, row := range nrows {
		payload.Nodes = append(payload.Nodes, nodePayload{ID: row.ID, Label: row.ID})
	}
	for _, row := range erows {
		payload.Edges = append(payload.Edges, edgePayload{
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
		log.Printf("httpserver /api/graph write: %v", err)
	}
}
