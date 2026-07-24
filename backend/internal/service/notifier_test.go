// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for PagerDuty / Slack notification payload builders.
//
package service

import (
	"testing"
	"time"
)

func TestBuildPagerDutyEvent_TriggerAndResolve(t *testing.T) {
	p := WebhookPayload{
		EventType:   "alert.opened",
		Fingerprint: "fp-1",
		ServerName:  "pg-1",
		Severity:    "critical",
		Category:    "Capacity",
		Title:       "connections high",
		FiredAt:     time.Now(),
	}
	ev := buildPagerDutyEvent("rk-test", p)
	if ev.EventAction != "trigger" {
		t.Fatalf("want trigger, got %s", ev.EventAction)
	}
	if ev.DedupKey != "fp-1" || ev.RoutingKey != "rk-test" {
		t.Fatalf("unexpected keys: %+v", ev)
	}
	if ev.Payload == nil || ev.Payload.Severity != "critical" {
		t.Fatalf("missing payload: %+v", ev.Payload)
	}

	p.EventType = "alert.resolved"
	ev = buildPagerDutyEvent("rk-test", p)
	if ev.EventAction != "resolve" {
		t.Fatalf("want resolve, got %s", ev.EventAction)
	}
	if ev.Payload != nil {
		t.Fatalf("resolve should omit payload")
	}
}
