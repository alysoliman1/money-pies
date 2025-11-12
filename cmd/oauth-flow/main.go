package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/schwab"
	"github.com/pkg/browser"
)

func main() {
	configFileLocation := os.Getenv("CONFIG_FILE_LOCATION")
	if configFileLocation == "" {
		fmt.Println("brokerage config file location not found")
		return
	}

	rawConfig, err := os.ReadFile(configFileLocation)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var config schwab.Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		fmt.Printf("failed to unmarshal config: %v", err)
		return
	}

	timeoutInSeconds := 30

	schwabClient := schwab.
		NewClient(config, timeoutInSeconds).
		SetAccessTokenFromFile()

	if schwabClient.IsAuthenticated() {
		fmt.Println("already authenticated")
		return
	}

	ctx := context.Background()

	port := "8080"
	addr := fmt.Sprintf("127.0.0.1:%s", port)
	server := &http.Server{
		Addr: addr,
	}

	authCodeChan := make(chan string)

	go func() {
		authURL := schwabClient.GetAuthURL()
		if err := browser.OpenURL(authURL); err != nil {
			fmt.Println("Please visit the following URL to authorize the application:")
			fmt.Println(authURL)
		}

		authCode := <-authCodeChan
		fmt.Println("Received authorization code", authCode)

		if err := schwabClient.ExchangeAuthCodeForAccessToken(ctx, authCode); err != nil {
			fmt.Println("failed to get access token", err)
			server.Shutdown(ctx)
			return
		}

		if !schwabClient.IsAuthenticated() {
			fmt.Println("failed to authenticate")
			server.Shutdown(ctx)
			return
		}

		fmt.Println("OAuth2.0 flow complete")
		server.Shutdown(ctx)
	}()

	// Register the handler for all paths
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if authCode := r.URL.Query().Get("code"); authCode != "" {
			authCodeChan <- authCode
		}
	})

	// Start the HTTPS server with self-signed certificate
	if err := server.ListenAndServeTLS("cert.pem", "key.pem"); err != nil && err != http.ErrServerClosed {
		fmt.Println("server error", err)
	}
}
