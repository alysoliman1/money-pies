package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages"
	"github.com/asoliman1/money-pies/internal/pkg/investor"
	"github.com/pkg/browser"
)

// This command serves a local web page that visualizes the connected trading account.
// It is read-only: it holds the account as an investor.ReadOnlyTradingAccount, which has
// no methods for placing or cancelling orders.
//
// The account is chosen with environment variables, see brokerages.NewTradingAccountFromEnv.

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "loopback address to listen on")
	noOpen := flag.Bool("no-open", false, "do not open the page in the browser")
	flag.Parse()

	// The page shows account data without any login, so it must not be reachable from other machines.
	if err := requireLoopback(*addr); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Narrowing to the read-only interface keeps the order methods out of reach of the viewer.
	var account investor.ReadOnlyTradingAccount
	account, err := brokerages.NewTradingAccountFromEnv(ctx)
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *addr, err)
	}

	server := &http.Server{
		Handler:           newHandler(account, os.Getenv("BROKERAGE")),
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://%s", listener.Addr())
	log.Printf("account viewer running at %s (read-only)", url)
	if !*noOpen {
		if err := browser.OpenURL(url); err != nil {
			log.Printf("open %s in your browser", url)
		}
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("account viewer stopped: %v", err)
	}
}

// requireLoopback rejects listen addresses that other machines could connect to.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address %q: %v", addr, err)
	}
	if !isLoopbackHost(host) {
		return fmt.Errorf("address %q is not a loopback address, use 127.0.0.1 or localhost", addr)
	}
	return nil
}
