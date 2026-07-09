package storage

// KnowledgeGraphReader reads persisted KG rows for admin HTTP (e.g. GET /api/graph).
type KnowledgeGraphReader interface {
	KnowledgeGraphNodesSelectAll() ([]KnowledgeGraphNodeRow, error)
	KnowledgeGraphEdgesSelectAll() ([]KnowledgeGraphEdgeRow, error)
}
