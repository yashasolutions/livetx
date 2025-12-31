package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/livetx/livetx/internal/gui"
)

func main() {
	godotenv.Load()

	apiKey := os.Getenv("SONIOX_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: SONIOX_API_KEY not set. Set it in .env or environment.\n")
		os.Exit(1)
	}

	g := gui.New(apiKey)
	g.Run()
}
