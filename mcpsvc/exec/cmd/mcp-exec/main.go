package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	execmcp "github.com/OctoSucker/mcpsvc/exec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func envLogConfig() bool {
	s := strings.TrimSpace(strings.ToLower(os.Getenv("EXEC_MCP_LOG_CONFIG")))
	return s == "1" || s == "true" || s == "yes"
}

func main() {
	log.SetOutput(os.Stderr)
	listen := flag.String("listen", "", "streamable MCP HTTP address (e.g. :8766); empty = stdio")
	flag.Parse()

	cfg, err := execmcp.LoadFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if envLogConfig() {
		log.Printf("execmcp: env EXEC_HOST_REPO_DIR=%q MCP_EXEC_REPO_MOUNT=%q EXEC_WORKSPACE_DIRS=%q EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX=%q",
			os.Getenv("EXEC_HOST_REPO_DIR"), os.Getenv("MCP_EXEC_REPO_MOUNT"), os.Getenv("EXEC_WORKSPACE_DIRS"),
			os.Getenv("EXEC_NESTED_DOCKER_USE_HOST_MNT_PREFIX"))
		log.Printf("execmcp: workspace_roots=%v nested_docker_bind_sources=%v", cfg.Roots, cfg.HostBindRoots)
	}

	srv := execmcp.NewMCPServer(cfg)

	if *listen == "" {
		if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return srv
	}, nil)
	log.Printf("mcp-exec streamable HTTP on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, h))
}
