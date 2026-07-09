// Package ingress holds peer user ingress adapters: stdin (terminal), telegram (Bot long-poll),
// adminhttp (embedded web admin). internal/gateway selects and runs them; none is a parent of the others.
package ingress
