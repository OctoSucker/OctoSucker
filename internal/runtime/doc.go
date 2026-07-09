// Package runtime owns the workspace-scoped agent runtime.
//
// Layout:
//   - runtime.go / dispatcher.go: top-level runtime lifecycle and event loop wiring
//   - model: task, plan, event, payload, and tool result types
//   - planning: next-step selection and tool-argument generation
//   - execution: synchronous plan-step execution
//   - judge: step-level and trajectory-level feedback
//
// See DESIGN.md in this directory for the runtime state-machine contract.
package runtime
