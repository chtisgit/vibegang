package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/db"
	"github.com/chtisgit/vibegang/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "vibegang",
	Short: "Vibegang is a multi-agent orchestration harness",
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactively setup your agent team TUI",
	Run: func(cmd *cobra.Command, args []string) {
		runSetup()
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent harness and stream logs",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(cfgFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		orch, err := orchestrator.NewOrchestrator(cfg)
		if err != nil {
			log.Fatalf("Failed to initialize orchestrator: %v", err)
		}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			log.Println("Received interrupt signal. Stopping all containers...")
			orch.StopSystem()
			cancel()
			os.Exit(0)
		}()

		if err := orch.StartSystem(ctx); err != nil {
			if ctx.Err() == nil {
				log.Fatalf("Failed to start system: %v", err)
			}
		}
	},
}

var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Check the inbox for unread mail",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig(cfgFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		dbConnStr := getDBConnStr()
		dbClient, err := db.NewDB(dbConnStr)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer dbClient.Close()

		summaries, err := dbClient.GetUnreadSummary(cfg.UserEmail)
		if err != nil {
			log.Fatalf("Failed to fetch inbox: %v", err)
		}

		if len(summaries) == 0 {
			fmt.Println("Inbox is empty.")
			return
		}

		fmt.Printf("You have %d unread emails:\n\n", len(summaries))
		for _, s := range summaries {
			fmt.Printf("From: %s\nSubject: %s\n", s.From, s.Subject)
			mail, err := dbClient.ReadMail(cfg.UserEmail, s.ID)
			if err != nil {
				log.Printf("Failed to read mail ID %d: %v", s.ID, err)
				continue
			}
			fmt.Printf("Body:\n%s\n", mail.Body)
			fmt.Println("--------------------------------------------------")
		}
	},
}

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Interactively manage your mailbox in a TUI",
	Run: func(cmd *cobra.Command, args []string) {
		runMailTUI(cfgFile)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "vibegang.yaml", "Path to config file")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(inboxCmd)
	rootCmd.AddCommand(mailCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
