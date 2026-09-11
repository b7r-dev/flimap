package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	httpAddr := flag.String("http", "", "HTTP address to listen on (e.g. :8080). If empty, uses stdio transport (default)")
	flag.Parse()

	cfg, err := LoadConfig()
	if err != nil {
		os.Stderr.WriteString("Configuration error: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Logger writes to stderr — stdout is reserved for the stdio MCP protocol
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	svc := NewMailService(cfg, logger)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "flimap",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Instructions: "IMAP/SMTP mail server for Protonmail Bridge. " +
			"Mail folders are hierarchical with '/' delimiters (e.g. 'INBOX', 'Folders/ben@b7r.dev', 'Labels/personal'). " +
			"Use list_folders to discover all available folders including nested subfolders. " +
			"Container folders (selectable=false) cannot hold messages — only their children can. " +
			"Messages are identified by UID within a folder. " +
			"Tools: list_folders, list_messages, search_messages, get_message, send_message, " +
			"move_message, mark_read, mark_unread, delete_message, create_folder, rename_folder, delete_folder.",
	})

	registerTools(server, svc)

	ctx := context.Background()

	if *httpAddr != "" {
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
		logger.Info("flimap MCP server listening", "addr", *httpAddr)
		if err := http.ListenAndServe(*httpAddr, handler); err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("flimap MCP server starting (stdio transport)")
		if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
}
