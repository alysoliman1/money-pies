package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/asoliman1/money-pies/internal/pkg/brokerages/schwab"
)

func main() {
	configLocation := os.Getenv("CONFIG_FILE_LOCATION")
	if configLocation == "" {
		fmt.Println("config location not found")
		return
	}

	rawConfig, err := os.ReadFile(configLocation)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var config schwab.Config
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		fmt.Printf("failed to unmarshal config: %v", err)
		return
	}

	timeoutInSeconds := 30
	schwabClient := schwab.NewClient(config, timeoutInSeconds)

	ctx := context.Background()

	schwabClient.SetTokenFromFile()

	if !schwabClient.IsAuthenticated() {
		if err := schwabClient.Authenticate(ctx); err != nil {
			fmt.Printf("failed to authenticate: %v", err)
			return
		}

		if !schwabClient.IsAuthenticated() {
			fmt.Println("failed to authenticate")
			return
		}
	}

	fmt.Println(schwabClient.GetQuote(ctx, "HLAL"))
}
