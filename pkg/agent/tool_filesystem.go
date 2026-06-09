package agent

import (
	"fmt"
	"log"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/invopop/jsonschema"
)

func (a *Agent) defineReadFileTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[readFileInput, string](g, "read_file", "Read a file from the workspace", func(ctx *ai.ToolContext, i readFileInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Read file '%s'", i.Path)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		b, err := os.ReadFile(i.Path)
		if err != nil {
			return fmt.Sprintf("Error reading file: %v", err), nil
		}
		return string(b), nil
	})
}

func (a *Agent) defineWriteFileTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[writeFileInput, string](g, "write_file", "Write or overwrite a file in the workspace", func(ctx *ai.ToolContext, i writeFileInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Wrote file '%s' (content length: %d)", i.Path, len(i.Content))); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := os.WriteFile(i.Path, []byte(i.Content), 0644)
		if err != nil {
			return fmt.Sprintf("Error writing file: %v", err), nil
		}
		return "File written successfully", nil
	})
}

// -- Tool Input Structs --

type readFileInput struct {
	Path string `json:"path"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (readFileInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (writeFileInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}
