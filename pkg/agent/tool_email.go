package agent

import (
	"fmt"
	"log"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/invopop/jsonschema"
)

func (a *Agent) defineCheckMailboxTool(g *genkit.Genkit) ai.ToolRef {
	return genkit.DefineTool[checkMailboxInput, string](g, "check_mailbox", "Check the agent's mailbox for unread mail", func(ctx *ai.ToolContext, i checkMailboxInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, "Checked mailbox"); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		summaries, err := a.DB.GetUnreadSummary(a.Config.Email)
		if err != nil {
			return fmt.Sprintf("Error checking mailbox: %v", err), nil
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
	return genkit.DefineTool[readMailInput, string](g, "read_mail", "Read the full body of a specific email by ID", func(ctx *ai.ToolContext, i readMailInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Read email ID %d", i.ID)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		mail, err := a.DB.ReadMail(a.Config.Email, i.ID)
		if err != nil {
			return fmt.Sprintf("Error reading mail: %v", err), nil
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
	return genkit.DefineTool[sendMailInput, string](g, "send_mail", "Send an email to another agent", func(ctx *ai.ToolContext, i sendMailInput) (string, error) {
		if err := a.DB.LogAction(a.Config.Email, fmt.Sprintf("Sent email to %s with subject '%s'", i.To, i.Subject)); err != nil {
			log.Printf("Failed to log action: %v", err)
		}
		err := a.DB.SendMail(a.Config.Email, i.To, i.Subject, i.Body)
		if err != nil {
			return fmt.Sprintf("Error sending mail: %v", err), nil
		}
		return "Email sent successfully", nil
	})
}

// -- Tool Input Structs --

type checkMailboxInput struct{}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (checkMailboxInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type readMailInput struct {
	ID int `json:"id"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (readMailInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}

type sendMailInput struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// JSONSchemaExtend allows additional properties in the generated schema to prevent Genkit validation failures
// if LLM agents pass extra parameters.
func (sendMailInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	schema.AdditionalProperties = nil
}
