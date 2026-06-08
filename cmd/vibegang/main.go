package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"strings"
	"time"

	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/db"
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

var inboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "Interactively manage your inbox in a TUI",
	Run: func(cmd *cobra.Command, args []string) {
		runMailTUI(cfgFile)
	},
}

var listTodosCmd = &cobra.Command{
	Use:   "list-todos",
	Short: "List todos from the database",
	Run: func(cmd *cobra.Command, args []string) {
		dbConnStr := getDBConnStr()
		dbClient, err := db.NewDB(dbConnStr)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer dbClient.Close()

		emailFilter, _ := cmd.Flags().GetString("email")
		items, err := dbClient.ListAllTodos(emailFilter)
		if err != nil {
			log.Fatalf("Failed to query todos: %v", err)
		}

		if len(items) == 0 {
			fmt.Println("No todos found.")
			return
		}

		fmt.Printf("%-5s | %-30s | %-30s | %-7s | %s\n", "ID", "Email Account", "Item", "Blocked", "Details")
		fmt.Println(strings.Repeat("-", 100))
		for _, item := range items {
			blockedStr := "no"
			if item.TaskBlocked {
				blockedStr = "yes"
			}
			fmt.Printf("%-5d | %-30s | %-30s | %-7s | %s\n", item.ID, item.Email, item.Item, blockedStr, item.Details)
		}
	},
}

var listEmailsCmd = &cobra.Command{
	Use:   "list-emails",
	Short: "List emails from the database (metadata only)",
	Run: func(cmd *cobra.Command, args []string) {
		dbConnStr := getDBConnStr()
		dbClient, err := db.NewDB(dbConnStr)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer dbClient.Close()

		emailFilter, _ := cmd.Flags().GetString("email")
		onlyUnread, _ := cmd.Flags().GetBool("unread")
		sinceStr, _ := cmd.Flags().GetString("since")

		var sinceTime time.Time
		if sinceStr != "" {
			dur, err := time.ParseDuration(sinceStr)
			if err == nil {
				sinceTime = time.Now().Add(-dur)
			} else {
				t, err := time.Parse(time.RFC3339, sinceStr)
				if err == nil {
					sinceTime = t
				} else {
					t, err = time.Parse("2006-01-02 15:04:05", sinceStr)
					if err == nil {
						sinceTime = t
					} else {
						log.Fatalf("Failed to parse --since flag '%s'. Use a duration (e.g. 24h, 1h) or a timestamp (e.g. RFC3339 like 2006-01-02T15:04:05Z or '2006-01-02 15:04:05')", sinceStr)
					}
				}
			}
		}

		emails, err := dbClient.ListAllEmails(emailFilter, onlyUnread, sinceTime)
		if err != nil {
			log.Fatalf("Failed to query emails: %v", err)
		}

		if len(emails) == 0 {
			fmt.Println("No emails found.")
			return
		}

		fmt.Printf("%-5s | %-25s | %-25s | %-30s | %-6s | %-19s\n", "ID", "From", "To", "Subject", "Read", "Timestamp")
		fmt.Println(strings.Repeat("-", 120))
		for _, email := range emails {
			readStr := "yes"
			if !email.IsRead {
				readStr = "no"
			}
			fmt.Printf("%-5d | %-25s | %-25s | %-30s | %-6s | %-19s\n",
				email.ID, email.From, email.To, email.Subject, readStr, email.Timestamp.Format("2006-01-02 15:04:05"))
		}
	},
}

var showEmailCmd = &cobra.Command{
	Use:   "show-email [id]",
	Short: "Show a specific email with its body by ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dbConnStr := getDBConnStr()
		dbClient, err := db.NewDB(dbConnStr)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer dbClient.Close()

		var id int
		if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
			log.Fatalf("Invalid email ID: %s. ID must be an integer.", args[0])
		}

		email, err := dbClient.GetEmailByID(id)
		if err != nil {
			log.Fatalf("Failed to retrieve email with ID %d: %v", id, err)
		}

		fmt.Printf("Email ID:  %d\n", email.ID)
		fmt.Printf("From:      %s\n", email.From)
		fmt.Printf("To:        %s\n", email.To)
		fmt.Printf("Subject:   %s\n", email.Subject)
		fmt.Printf("Sent:      %s\n", email.Timestamp.Format("2006-01-02 15:04:05"))
		readStr := "yes"
		if !email.IsRead {
			readStr = "no"
		}
		fmt.Printf("Read:      %s\n", readStr)
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println("Body:")
		fmt.Println(email.Body)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "vibegang.yaml", "Path to config file")
	startCmd.Flags().BoolVar(&resetFlag, "reset", false, "Clear all database tables before starting")

	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(inboxCmd)

	listTodosCmd.Flags().StringP("email", "e", "", "List only todos for this specific e-mail account")
	rootCmd.AddCommand(listTodosCmd)

	listEmailsCmd.Flags().StringP("email", "e", "", "List only e-mails involving this specific account (from/to)")
	listEmailsCmd.Flags().BoolP("unread", "u", false, "List only unread e-mails")
	listEmailsCmd.Flags().StringP("since", "s", "", "Limit results based on time sent (e.g. 24h, 1h, or a timestamp)")
	rootCmd.AddCommand(listEmailsCmd)

	rootCmd.AddCommand(showEmailCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
