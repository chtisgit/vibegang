package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
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

func (a *Agent) saveHistory(history []*ai.Message) {
	serialized, err := json.Marshal(history)
	if err != nil {
		log.Printf("Failed to marshal history for agent %s: %v", a.Config.Name, err)
		return
	}
	if err := a.DB.SaveAgentHistory(a.Config.Email, serialized); err != nil {
		log.Printf("Failed to save history for agent %s to DB: %v", a.Config.Name, err)
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
				a.defineUpdateTodoBlockedStateTool(g),
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
	historyBytes, err := a.DB.LoadAgentHistory(a.Config.Email)
	if err != nil {
		log.Printf("Failed to load agent history from DB for %s: %v", a.Config.Email, err)
	} else if len(historyBytes) > 0 {
		if err := json.Unmarshal(historyBytes, &history); err != nil {
			log.Printf("Failed to unmarshal agent history for %s: %v", a.Config.Email, err)
		} else {
			log.Printf("Loaded %d historic messages for agent %s from DB", len(history), a.Config.Name)
		}
	}
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

		unblockedTodos, err := a.DB.GetTodoItems(a.Config.Email, true)
		if err != nil {
			log.Printf("Error getting todo items: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(summaries) == 0 && len(unblockedTodos) == 0 {
			log.Printf("Agent %s waiting for mail...", a.Config.Name)
			err := a.DB.WaitForMail(a.Config.Email)
			if err != nil {
				log.Printf("Error waiting for mail: %v", err)
			}
			continue
		}

		var prompt string

		if len(summaries) > 0 {
			if len(unblockedTodos) > 0 {
				prompt = fmt.Sprintf("You have %d unread emails in your mailbox and %d unblocked todo items. Please check them and take necessary actions.", len(summaries), len(unblockedTodos))
			} else {
				prompt = fmt.Sprintf("You have %d unread emails in your mailbox. Please check them and take necessary actions.", len(summaries))
			}
			log.Printf("Agent %s received mail, invoking LLM...", a.Config.Name)
		} else {
			prompt = fmt.Sprintf("You have no unread emails, but you have %d unblocked todo items. Please review your todo list and take necessary actions.", len(unblockedTodos))
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
		a.saveHistory(history)

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
				a.saveHistory(history)
			}
		}
	}
}

