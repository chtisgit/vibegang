package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/db"

	"github.com/firebase/genkit/go/ai"
	coreapi "github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/firebase/genkit/go/plugins/compat_oai/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

type Agent struct {
	Config config.AgentConfig
	DB     *db.DB
}

func NewAgent(cfg config.AgentConfig, dbClient *db.DB) *Agent {
	return &Agent{
		Config: cfg,
		DB:     dbClient,
	}
}

func (a *Agent) Start(ctx context.Context) error {
	modelName := a.Config.Model

	parts := strings.Split(modelName, "/")
	provider := modelName
	if len(parts) > 0 {
		provider = parts[0]
	}

	var plugins []coreapi.Plugin

	switch provider {
	case "googleai":
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("GEMINI_API_KEY environment variable is required for model %s", modelName)
		}
		plugins = append(plugins, &googlegenai.GoogleAI{APIKey: apiKey})
	case "vertexai":
		plugins = append(plugins, &googlegenai.VertexAI{})
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("OPENAI_API_KEY environment variable is required for model %s", modelName)
		}
		plugins = append(plugins, &openai.OpenAI{APIKey: apiKey})
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY environment variable is required for model %s", modelName)
		}
		plugins = append(plugins, &anthropic.Anthropic{})
	case "togetherai":
		apiKey := os.Getenv("TOGETHER_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("TOGETHER_API_KEY environment variable is required for model %s", modelName)
		}
		plugins = append(plugins, &compat_oai.OpenAICompatible{
			Provider: "togetherai",
			APIKey:   apiKey,
			BaseURL:  "https://api.together.xyz/v1",
		})
	case "custom":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("CUSTOM_API_KEY")
		}
		if apiKey == "" {
			return fmt.Errorf("OPENAI_API_KEY or CUSTOM_API_KEY environment variable is required when model is set to 'custom'")
		}
		providerName := os.Getenv("CUSTOM_PROVIDER")
		if providerName == "" {
			return fmt.Errorf("CUSTOM_PROVIDER environment variable is required when model is set to 'custom'")
		}
		modelName = os.Getenv("CUSTOM_MODEL")
		if modelName == "" {
			return fmt.Errorf("CUSTOM_MODEL environment variable is required when model is set to 'custom'")
		}
		baseURL := os.Getenv("CUSTOM_BASE_URL")
		if baseURL == "" {
			return fmt.Errorf("CUSTOM_BASE_URL environment variable is required when model is set to 'custom'")
		}
		plugins = append(plugins, &compat_oai.OpenAICompatible{
			Provider: providerName,
			APIKey:   apiKey,
			BaseURL:  baseURL,
		})
	}

	g := genkit.Init(ctx, genkit.WithPlugins(plugins...))

	// Register tools
	var allowedTools []ai.ToolRef

	for _, toolGroup := range a.Config.Tools {
		switch toolGroup {
		case "email":
			allowedTools = append(allowedTools,
				a.defineCheckMailboxTool(g),
				a.defineReadMailTool(g),
				a.defineSendMailTool(g),
			)
		case "todo":
			allowedTools = append(allowedTools,
				a.defineListTodoTool(g),
				a.defineAddTodoTool(g),
				a.defineRemoveTodoTool(g),
				a.defineViewTodoDetailsTool(g),
			)
		case "filesystem":
			allowedTools = append(allowedTools,
				a.defineReadFileTool(g),
				a.defineWriteFileTool(g),
			)
		case "terminal_commands":
			allowedTools = append(allowedTools,
				a.defineTerminalTool(g),
			)
		}
	}

	if err := a.DB.LogAction(a.Config.Email, "Ready"); err != nil {
		log.Printf("Failed to log ready state: %v", err)
	}

	var history []*ai.Message
	b := NewBackoff([]time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		20 * time.Minute,
	})
	for {
		summaries, err := a.DB.GetUnreadSummary(a.Config.Email)
		if err != nil {
			log.Printf("Error getting summaries: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		todos, err := a.DB.GetTodoItems(a.Config.Email)
		if err != nil {
			log.Printf("Error getting todo items: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(summaries) == 0 && len(todos) == 0 {
			log.Printf("Agent %s waiting for mail...", a.Config.Name)
			err := a.DB.WaitForMail(a.Config.Email)
			if err != nil {
				log.Printf("Error waiting for mail: %v", err)
			}
			continue
		}

		var prompt string

		if len(summaries) > 0 {
			prompt = fmt.Sprintf("You have %d unread emails in your mailbox. Please check them and take necessary actions.", len(summaries))
			log.Printf("Agent %s received mail, invoking LLM...", a.Config.Name)
		} else {
			prompt = fmt.Sprintf("You have no unread emails, but you have %d pending todo items. Please review your todo list and take necessary actions", len(todos))
			log.Printf("Agent %s has pending todos, invoking LLM...", a.Config.Name)
		}

		resp, err := genkit.Generate(ctx, g,
			ai.WithModelName(modelName),
			ai.WithSystem(a.Config.SystemPrompt),
			ai.WithMessages(history...),
			ai.WithPrompt(prompt),
			ai.WithTools(allowedTools...),
			ai.WithMaxTurns(64),
		)

		if err != nil {
			log.Printf("Generation error: %v", err)
			b.Increment()
			if wErr := b.Wait(ctx); wErr != nil {
				return wErr
			}
			continue
		}
		b.Reset()
		log.Printf("Agent %s action completed: %s", a.Config.Name, resp.Text())

		history = resp.History()

		inputLength := resp.Usage.InputTokens + resp.Usage.CachedContentTokens
		if inputLength > 200_000 {
			resp2, err := genkit.Generate(ctx, g,
				ai.WithModelName(modelName),
				ai.WithSystem(a.Config.SystemPrompt),
				ai.WithMessages(history...),
				ai.WithPrompt("Summarize what you were doing up until now from your own pov. Be sure to include essential information like local file paths and repository URLs of projects you are working on. Keep the summary as short as possible."),
			)

			if err != nil {
				log.Printf("Generation error: %v", err)
				b.Increment()
				if wErr := b.Wait(ctx); wErr != nil {
					return wErr
				}
			} else {
				b.Reset()
				log.Printf("Agent %s has performed auto-compaction: %s", a.Config.Name, resp2.Text())
				history = []*ai.Message{
					ai.NewModelTextMessage(resp2.Text()),
				}
			}
		}
	}
}

// -- Tool Definitions --

func (a *Agent) defineCheckMailboxTool(g *genkit.Genkit) ai.ToolRef {
	type input struct{}
	return genkit.DefineTool[input, string](g, "check_mailbox", "Check the agent's mailbox for unread mail", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, "Checked mailbox"); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		summaries, err := a.DB.GetUnreadSummary(a.Config.Email)
		if err != nil {
			return "", err
		}
		if len(summaries) == 0 {
			return "You have 0 unread emails.", nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("You have unread emails (%d):\n", len(summaries)))
		for _, s := range summaries {
			sb.WriteString(fmt.Sprintf("- ID: %d | From: %s | Subject: %s | Received: %s\n", s.ID, s.From, s.Subject, s.Timestamp.Format("2006-01-02 15:04:05")))
		}
		return sb.String(), nil
	})
}

func (a *Agent) defineReadMailTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		ID int `json:"id"`
	}
	return genkit.DefineTool[input, string](g, "read_mail", "Read the full body of a specific email by ID", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Read email ID %d", i.ID)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		mail, err := a.DB.ReadMail(a.Config.Email, i.ID)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Email ID: %d\n", mail.ID))
		sb.WriteString(fmt.Sprintf("From: %s\n", mail.From))
		sb.WriteString(fmt.Sprintf("To: %s\n", mail.To))
		sb.WriteString(fmt.Sprintf("Subject: %s\n", mail.Subject))
		sb.WriteString(fmt.Sprintf("Date: %s\n", mail.Timestamp.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("Body:\n%s\n", mail.Body))
		return sb.String(), nil
	})
}

func (a *Agent) defineSendMailTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	return genkit.DefineTool[input, string](g, "send_mail", "Send an email to another agent", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Sent email to %s with subject '%s'", i.To, i.Subject)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.SendMail(a.Config.Email, i.To, i.Subject, i.Body)
		if err != nil {
			return "", err
		}
		return "Email sent successfully", nil
	})
}

func (a *Agent) defineReadFileTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		Path string `json:"path"`
	}
	return genkit.DefineTool[input, string](g, "read_file", "Read a file from the workspace", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Read file '%s'", i.Path)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		b, err := os.ReadFile(i.Path)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
}

func (a *Agent) defineWriteFileTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	return genkit.DefineTool[input, string](g, "write_file", "Write or overwrite a file in the workspace", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Wrote file '%s' (content length: %d)", i.Path, len(i.Content))); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := os.WriteFile(i.Path, []byte(i.Content), 0644)
		if err != nil {
			return "", err
		}
		return "File written successfully", nil
	})
}

func (a *Agent) defineTerminalTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		Command string `json:"command"`
	}
	return genkit.DefineTool[input, string](g, "run_terminal_command", "Run a bash command in the workspace. Do not use interactive commands. Git is available.", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Ran terminal command: %s", i.Command)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		cmd := exec.CommandContext(ctx, "bash", "-c", i.Command)
		cmd.Dir = "/workspace"
		out, err := cmd.CombinedOutput()
		outputStr := string(out)
		if err != nil {
			return fmt.Sprintf("Error: %v\nOutput: %s", err, outputStr), nil
		}
		return outputStr, nil
	})
}

func (a *Agent) defineListTodoTool(g *genkit.Genkit) ai.ToolRef {
	type input struct{}
	return genkit.DefineTool[input, string](g, "list_todo_items", "List outstanding todo items for the agent", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, "Listed todo items"); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		items, err := a.DB.GetTodoItems(a.Config.Email)
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "Your todo list is empty.", nil
		}
		var sb strings.Builder
		sb.WriteString("Outstanding Todo Items:\n")
		for _, item := range items {
			sb.WriteString(fmt.Sprintf("- ID: %d | Item: %s\n", item.ID, item.Item))
		}
		return sb.String(), nil
	})
}

func (a *Agent) defineAddTodoTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		Item    string `json:"item"`
		Details string `json:"details"`
	}
	return genkit.DefineTool[input, string](g, "add_todo_item", "Add a new item to the agent's todo list", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Added todo item: %s (Details: %s)", i.Item, i.Details)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.AddTodoItem(a.Config.Email, i.Item, i.Details)
		if err != nil {
			return "", err
		}
		return "Todo item added successfully", nil
	})
}

func (a *Agent) defineRemoveTodoTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		ID int `json:"id"`
	}
	return genkit.DefineTool[input, string](g, "remove_todo_item", "Remove a todo item from the agent's list by ID", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Removed todo item ID %d", i.ID)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.RemoveTodoItem(a.Config.Email, i.ID)
		if err != nil {
			return "", err
		}
		return "Todo item removed successfully", nil
	})
}

func (a *Agent) defineViewTodoDetailsTool(g *genkit.Genkit) ai.ToolRef {
	type input struct {
		ID int `json:"id"`
	}
	return genkit.DefineTool[input, string](g, "view_todo_item_details", "View the detailed description of a specific todo item by ID", func(ctx *ai.ToolContext, i input) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Viewed todo item details for ID %d", i.ID)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		item, err := a.DB.GetTodoItem(a.Config.Email, i.ID)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Todo Item ID: %d\n", item.ID))
		sb.WriteString(fmt.Sprintf("Item: %s\n", item.Item))
		sb.WriteString(fmt.Sprintf("Details:\n%s\n", item.Details))
		return sb.String(), nil
	})
}
