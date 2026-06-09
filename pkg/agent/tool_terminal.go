package agent

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/invopop/jsonschema"
)

func (a *Agent) defineTerminalTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[terminalInput, string](g, "run_terminal_command", "Run a bash command in the workspace. Do not use interactive commands. Git is available.", func(ctx *ai.ToolContext, i terminalInput) (string, error) {
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

// -- Tool Input Structs --

type terminalInput struct {
	Command string `json:"command"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (terminalInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}
