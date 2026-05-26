package service

import (
	"encoding/json"
	"testing"
)

func TestTestPayload_slackHasTextField(t *testing.T) {
	raw, err := json.Marshal(TestPayload("slack"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["text"]; !ok {
		t.Fatalf("slack test payload must include text field, got %s", string(raw))
	}
}

func TestTestPayload_webhookUsesAlertShape(t *testing.T) {
	raw, err := json.Marshal(TestPayload("webhook"))
	if err != nil {
		t.Fatal(err)
	}
	var p WebhookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.EventType != "test" || p.Title == "" {
		t.Fatalf("unexpected webhook test payload: %+v", p)
	}
}
