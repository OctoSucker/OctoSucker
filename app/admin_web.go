package app

import (
	_ "embed"
	"net/http"

	"github.com/OctoSucker/octosucker/app/httpserver"
)

//go:embed admin/index.html
var adminIndexHTML []byte

// AdminHandler serves the web admin UI (chat + knowledge graph). Bind to loopback in untrusted environments.
func (a *App) AdminHandler() (http.Handler, error) {
	return httpserver.Handler(httpserver.Options{
		IndexHTML: adminIndexHTML,
		RunChat:   a.RunInputFromLocal,
		Graph: func() httpserver.KnowledgeGraphReader {
			return a.data
		},
	})
}
