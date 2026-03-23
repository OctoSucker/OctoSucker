package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	execmcp "github.com/OctoSucker/mcpsvc/exec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetOutput(os.Stderr)
	listen := flag.String("listen", "", "streamable MCP HTTP address (e.g. :8766); empty = stdio")
	flag.Parse()

	cfg, err := execmcp.LoadFromEnv()
	if err != nil {
		log.Fatal(err)
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
