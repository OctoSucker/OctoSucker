// Package gateway reads [config.Workspace] (HTTP listen, Telegram) plus CLI [Options], then wires an
// [Agent] (internal/runtime.Runtime) into the peer ingress adapters under internal/ingress (adminhttp,
// telegram, stdin). No planner or tool logic, only orchestration and shutdown.
package gateway
