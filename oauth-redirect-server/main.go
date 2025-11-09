package main

import (
	"fmt"
	"log"
	"net/http"
)

func handleCallback(w http.ResponseWriter, r *http.Request) {
	// Parse the query parameters
	code := r.URL.Query().Get("code")

	if code == "" {
		fmt.Fprintf(w, "No code parameter found in the request")
		log.Println("Request received but no code parameter found")
		return
	}

	// Print the code to console
	fmt.Printf("\n=================================\n")
	fmt.Printf("OAuth Code Received: %s\n", code)
	fmt.Printf("=================================\n\n")

	// Send response to browser
	fmt.Fprintf(w, "Authorization code received successfully!\n\nCode: %s\n\nYou can close this window.", code)
}

func main() {
	// Register the handler for all paths
	http.HandleFunc("/", handleCallback)

	port := "8080"
	addr := "127.0.0.1:" + port

	fmt.Printf("Starting OAuth callback server on https://127.0.0.1:%s\n", port)
	fmt.Println("Waiting for OAuth callback...")
	fmt.Println("\nNote: Using self-signed certificate - your browser will show a security warning.")
	fmt.Println("This is normal for local development. Click 'Advanced' and proceed to continue.\n")

	// Start the HTTPS server with self-signed certificate
	if err := http.ListenAndServeTLS(addr, "cert.pem", "key.pem", nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
