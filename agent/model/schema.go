// Package model holds persistence schema names, row DTOs, AgentDB (open + migrate), and SQLite methods on AgentDB.
package model

// Table names — single source of truth for migrations and queries.
const (
	TableTasks        = "tasks"
	TableRoutingEdges = "routing_edges"
	TableRoutingMeta  = "routing_meta"
	TableRecallChunks = "recall_chunks"
)
