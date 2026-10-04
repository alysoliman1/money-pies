package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages"
	"github.com/asoliman1/money-pies/internal/pkg/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This command serves the trading account of a brokerage over MCP using the stdio transport.
// Besides reading the account, it can place and cancel real orders.
//
// The account is chosen with environment variables, see brokerages.NewTradingAccountFromEnv.

func main() {
	// stdout carries the MCP protocol, so all logging must go to stderr.
	log.SetOutput(os.Stderr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	account, err := brokerages.NewTradingAccountFromEnv(ctx)
	if err != nil {
		log.Fatal(err)
	}

	server := mcpserver.New(account)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("mcp server stopped: %v", err)
	}
}
