package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/schwab"
	"github.com/asoliman1/money-pies/internal/pkg/pies"
)

func main() {
	clientConfigFile := os.Getenv("SCHWAB_CLIENT_CONFIG")
	if clientConfigFile == "" {
		fmt.Println("Schwab Client Config not specified")
		return
	}

	rawClientConfig, err := os.ReadFile(clientConfigFile)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var clientConfig schwab.Config
	if err := json.Unmarshal(rawClientConfig, &clientConfig); err != nil {
		fmt.Printf("failed to unmarshal config: %v", err)
		return
	}

	timeoutInSeconds := 30
	schwabClient := schwab.
		NewClient(clientConfig, timeoutInSeconds).
		GetAccessTokenFromFile()

	investor := pies.Investor{
		BrokerageClient: schwabClient,
	}

	investor.GetPieStatus(context.Background(), pies.Pie{})
}
