package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/invopop/jsonschema"
)

func (a *Agent) defineListTodoTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[listTodoInput, string](g, "list_todo_items", "List outstanding todo items for the agent", func(ctx *ai.ToolContext, i listTodoInput) (string, error) {
		items, err := a.DB.GetTodoItems(a.Config.Email, false)
		if err != nil {
			if err := a.DB.LogAction(a.Config.Email, "Listed todo items (error)"); err != nil {
				log.Printf("Failed to log action: %v", err)
			}
			return fmt.Sprintf("Error listing todo items: %v", err), nil
		}
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Listed todo items (%d)", len(items))); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		if len(items) == 0 {
			return "Your todo list is empty.", nil
		}
		var sb strings.Builder
		sb.WriteString("Outstanding Todo Items:\n")
		for _, item := range items {
			status := ""
			if item.TaskBlocked {
				status = " [BLOCKED]"
			}
			sb.WriteString(fmt.Sprintf("- ID: %d | Item: %s%s\n", item.ID, item.Item, status))
		}
		return sb.String(), nil
	})
}

func (a *Agent) defineAddTodoTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[addTodoInput, string](g, "add_todo_item", "Add a new item to the agent's todo list", func(ctx *ai.ToolContext, i addTodoInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Added todo item: %s (Details: %s, Blocked: %t)", i.Item, i.Details, i.TaskBlocked)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.AddTodoItem(a.Config.Email, i.Item, i.Details, i.TaskBlocked)
		if err != nil {
			return fmt.Sprintf("Error adding todo item: %v", err), nil
		}
		return "Todo item added successfully", nil
	})
}

func (a *Agent) defineRemoveTodoTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[removeTodoInput, string](g, "remove_todo_item", "Remove a todo item from the agent's list by ID", func(ctx *ai.ToolContext, i removeTodoInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Removed todo item ID %d", i.ID)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.RemoveTodoItem(a.Config.Email, i.ID)
		if err != nil {
			return fmt.Sprintf("Error removing todo item: %v", err), nil
		}
		return "Todo item removed successfully", nil
	})
}

func (a *Agent) defineViewTodoDetailsTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[viewTodoDetailsInput, string](g, "view_todo_item_details", "View the detailed description of a specific todo item by ID", func(ctx *ai.ToolContext, i viewTodoDetailsInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Viewed todo item details for ID %d", i.ID)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		item, err := a.DB.GetTodoItem(a.Config.Email, i.ID)
		if err != nil {
			return fmt.Sprintf("Error viewing todo item details: %v", err), nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Todo Item ID: %d\n", item.ID))
		sb.WriteString(fmt.Sprintf("Item: %s\n", item.Item))
		sb.WriteString(fmt.Sprintf("Blocked: %t\n", item.TaskBlocked))
		sb.WriteString(fmt.Sprintf("Details:\n%s\n", item.Details))
		return sb.String(), nil
	})
}

func (a *Agent) defineUpdateTodoBlockedStateTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[updateTodoBlockedStateInput, string](g, "update_todo_blocked_state", "Update the blocked state of a todo item (task_blocked can be true or false)", func(ctx *ai.ToolContext, i updateTodoBlockedStateInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Updated todo item ID %d blocked state to %t", i.ID, i.TaskBlocked)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.UpdateTodoBlockedState(a.Config.Email, i.ID, i.TaskBlocked)
		if err != nil {
			return fmt.Sprintf("Error updating todo blocked state: %v", err), nil
		}
		return "Todo item blocked state updated successfully", nil
	})
}

// -- Tool Input Structs --

type listTodoInput struct{}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (listTodoInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type addTodoInput struct {
	Item        string `json:"item"`
	Details     string `json:"details"`
	TaskBlocked bool   `json:"task_blocked"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (addTodoInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type removeTodoInput struct {
	ID int `json:"id"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (removeTodoInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type viewTodoDetailsInput struct {
	ID int `json:"id"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (viewTodoDetailsInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type updateTodoBlockedStateInput struct {
	ID          int  `json:"id"`
	TaskBlocked bool `json:"task_blocked"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (updateTodoBlockedStateInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}
