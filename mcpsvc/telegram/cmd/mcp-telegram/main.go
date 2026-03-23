package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/OctoSucker/mcpsvc/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	log.SetOutput(os.Stderr)
	listen := flag.String("listen", "", "streamable MCP HTTP address (e.g. :8765); empty = stdio")
	flag.Parse()

	cfg, err := telegram.LoadFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	api, err := telegram.NewBotAPI(cfg)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}
	srv := telegram.NewMCPServer(cfg, api)

	if *listen == "" {
		if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return srv
	}, nil)
	log.Printf("mcp-telegram streamable HTTP on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, h))
}
