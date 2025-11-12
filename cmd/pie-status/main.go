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

	if !schwabClient.IsAuthenticated() {
		fmt.Println("not authenticated. please run oauth flow")
	}

	fmt.Println(schwabClient.GetAccounts(context.Background()))
}
