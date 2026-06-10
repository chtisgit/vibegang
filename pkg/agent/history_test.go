package agent

import (
	"encoding/json"
	"testing"

	"github.com/firebase/genkit/go/ai"
)

func TestHistorySerialization(t *testing.T) {
	history := []*ai.Message{
		ai.NewUserTextMessage("Hello"),
		ai.NewModelTextMessage("Hi there"),
	}

	serialized, err := json.Marshal(history)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var deserialized []*ai.Message
	err = json.Unmarshal(serialized, &deserialized)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(deserialized) != len(history) {
		t.Fatalf("Expected length %d, got %d", len(history), len(deserialized))
	}

	if deserialized[0].Role != "user" || deserialized[0].Content[0].Text != "Hello" {
		t.Errorf("First message incorrect: %+v", deserialized[0])
	}
	if deserialized[1].Role != "model" || deserialized[1].Content[0].Text != "Hi there" {
		t.Errorf("Second message incorrect: %+v", deserialized[1])
	}
}
