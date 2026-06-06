package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/orchestrator"

	"github.com/spf13/cobra"
)

var cfgFile string
var resetFlag bool

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

		orch, err := orchestrator.NewOrchestrator(cfg, resetFlag)
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

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Interactively manage your mailbox in a TUI",
	Run: func(cmd *cobra.Command, args []string) {
		runMailTUI(cfgFile)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "vibegang.yaml", "Path to config file")
	startCmd.Flags().BoolVar(&resetFlag, "reset", false, "Clear all database tables before starting")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(mailCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
