package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/chtisgit/vibegang/pkg/agent"
	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/db"
)

func main() {
	name := os.Getenv("AGENT_NAME")
	email := os.Getenv("AGENT_EMAIL")
	role := os.Getenv("AGENT_ROLE")
	prompt := os.Getenv("AGENT_PROMPT")
	toolsStr := os.Getenv("AGENT_TOOLS")
	dbConnStr := os.Getenv("DB_CONN_STR")
	model := os.Getenv("AGENT_MODEL")

	if name == "" || email == "" || dbConnStr == "" {
		log.Fatal("Missing required environment variables")
	}

	cfg := config.AgentConfig{
		Name:         name,
		Email:        email,
		Role:         role,
		SystemPrompt: prompt,
		Tools:        strings.Split(toolsStr, ","),
		Model:        model,
	}

	dbClient, err := db.NewDB(dbConnStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling to log exit state
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, initiating shutdown...", sig)
		if err := dbClient.LogAction(email, fmt.Sprintf("Exited (Signal: %v)", sig)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		cancel()
	}()

	agentObj := agent.NewAgent(cfg, dbClient)

	log.Printf("Starting agent %s (%s)...", name, role)
	if err := agentObj.Start(ctx); err != nil {
		if err == context.Canceled {
			log.Println("Agent shutdown complete.")
		} else {
			log.Fatalf("Agent error: %v", err)
		}
	}
}
