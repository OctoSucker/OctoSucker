package store

// Table names — single source of truth for migrations and queries.
const (
	TableRoutingEdges        = "routing_edges"
	TableRoutingTransitions  = "routing_transitions" // append-only recent (intent, from→to, outcome) for routegraph; pruned to cap
	TableKnowledgeGraphNodes = "kg_nodes"
	TableKnowledgeGraphEdges = "kg_edges"
)
