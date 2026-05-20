package main

import (
	"strings"
	"testing"
	"time"
)

func TestDetermineTarget(t *testing.T) {
	tests := []struct {
		name       string
		req        EscalationRequest
		wantTeam   string
		wantNotify string
	}{
		// ── critical rules ──────────────────────────────────────────────────────
		{
			name:       "critical + attempts=2 → L3",
			req:        EscalationRequest{Priority: "critical", Attempts: 2},
			wantTeam:   "L3_TEAM",
			wantNotify: "immediate",
		},
		{
			name:       "critical + attempts=5 still L3",
			req:        EscalationRequest{Priority: "critical", Attempts: 5},
			wantTeam:   "L3_TEAM",
			wantNotify: "immediate",
		},
		{
			name:       "critical + attempts=1 falls through (threshold not met)",
			req:        EscalationRequest{Priority: "critical", Attempts: 1},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "critical + attempts=0 falls through",
			req:        EscalationRequest{Priority: "critical", Attempts: 0},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		// ── high rules ───────────────────────────────────────────────────────────
		{
			name:       "high + attempts=3 → L2",
			req:        EscalationRequest{Priority: "high", Attempts: 3},
			wantTeam:   "L2_TEAM",
			wantNotify: "1h",
		},
		{
			name:       "high + attempts=10 still L2",
			req:        EscalationRequest{Priority: "high", Attempts: 10},
			wantTeam:   "L2_TEAM",
			wantNotify: "1h",
		},
		{
			name:       "high + attempts=2 falls through",
			req:        EscalationRequest{Priority: "high", Attempts: 2},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "high + attempts=0 falls through",
			req:        EscalationRequest{Priority: "high", Attempts: 0},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		// ── no_solution_found + network ──────────────────────────────────────────
		{
			name:       "no_solution_found + network → NETWORK_OPS",
			req:        EscalationRequest{Reason: "no_solution_found", Category: "network"},
			wantTeam:   "NETWORK_OPS",
			wantNotify: "2h",
		},
		{
			name:       "no_solution_found + hardware → L1 (wrong category)",
			req:        EscalationRequest{Reason: "no_solution_found", Category: "hardware"},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "no_solution_found + account → L1",
			req:        EscalationRequest{Reason: "no_solution_found", Category: "account"},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "other reason + network → L1 (wrong reason)",
			req:        EscalationRequest{Reason: "user_request", Category: "network"},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "empty reason + network → L1",
			req:        EscalationRequest{Category: "network"},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		// ── priority precedence over no_solution_found ───────────────────────────
		{
			name:       "critical+attempts=2 beats no_solution_found+network → L3",
			req:        EscalationRequest{Priority: "critical", Attempts: 2, Reason: "no_solution_found", Category: "network"},
			wantTeam:   "L3_TEAM",
			wantNotify: "immediate",
		},
		{
			name:       "high+attempts=3 beats no_solution_found+network → L2",
			req:        EscalationRequest{Priority: "high", Attempts: 3, Reason: "no_solution_found", Category: "network"},
			wantTeam:   "L2_TEAM",
			wantNotify: "1h",
		},
		// ── default / fallthrough ────────────────────────────────────────────────
		{
			name:       "medium priority → L1",
			req:        EscalationRequest{Priority: "medium", Attempts: 5},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "low priority → L1",
			req:        EscalationRequest{Priority: "low", Attempts: 99},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
		{
			name:       "empty request → L1",
			req:        EscalationRequest{},
			wantTeam:   "L1_TEAM",
			wantNotify: "4h",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			team, notify := determineTarget(tc.req)
			if team != tc.wantTeam {
				t.Errorf("team: got %q, want %q", team, tc.wantTeam)
			}
			if notify != tc.wantNotify {
				t.Errorf("notify: got %q, want %q", notify, tc.wantNotify)
			}
		})
	}
}

func TestEscalate(t *testing.T) {
	fixedTime := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	const fixedID = "esc-test-id-001"

	t.Run("ticket_id preserved in result and audit", func(t *testing.T) {
		result, audit := escalate(EscalationRequest{TicketID: "T-42", Priority: "low"}, fixedTime, fixedID)
		if result.TicketID != "T-42" {
			t.Errorf("result.TicketID: got %q, want T-42", result.TicketID)
		}
		if audit.TicketID != "T-42" {
			t.Errorf("audit.TicketID: got %q, want T-42", audit.TicketID)
		}
	})

	t.Run("escalation_id matches supplied id", func(t *testing.T) {
		result, audit := escalate(EscalationRequest{Priority: "low"}, fixedTime, fixedID)
		if result.EscalationID != fixedID {
			t.Errorf("result.EscalationID: got %q, want %q", result.EscalationID, fixedID)
		}
		if audit.EscalationID != fixedID {
			t.Errorf("audit.EscalationID: got %q, want %q", audit.EscalationID, fixedID)
		}
	})

	t.Run("timestamp is RFC3339 UTC", func(t *testing.T) {
		result, audit := escalate(EscalationRequest{Priority: "low"}, fixedTime, fixedID)
		const want = "2026-05-20T12:00:00Z"
		if result.Timestamp != want {
			t.Errorf("result.Timestamp: got %q, want %q", result.Timestamp, want)
		}
		if audit.Timestamp != want {
			t.Errorf("audit.Timestamp: got %q, want %q", audit.Timestamp, want)
		}
	})

	t.Run("result and audit share escalation_id and timestamp", func(t *testing.T) {
		result, audit := escalate(EscalationRequest{Priority: "critical", Attempts: 2}, fixedTime, fixedID)
		if result.EscalationID != audit.EscalationID {
			t.Error("result and audit must share the same escalation_id")
		}
		if result.Timestamp != audit.Timestamp {
			t.Error("result and audit must share the same timestamp")
		}
	})

	t.Run("result team and notify are consistent with determineTarget", func(t *testing.T) {
		req := EscalationRequest{Priority: "critical", Attempts: 3}
		result, audit := escalate(req, fixedTime, fixedID)
		wantTeam, wantNotify := determineTarget(req)
		if result.EscalatedTo != wantTeam {
			t.Errorf("result.EscalatedTo: got %q, want %q", result.EscalatedTo, wantTeam)
		}
		if result.NotificationTime != wantNotify {
			t.Errorf("result.NotificationTime: got %q, want %q", result.NotificationTime, wantNotify)
		}
		if audit.EscalatedTo != wantTeam {
			t.Errorf("audit.EscalatedTo: got %q, want %q", audit.EscalatedTo, wantTeam)
		}
		if audit.NotificationTime != wantNotify {
			t.Errorf("audit.NotificationTime: got %q, want %q", audit.NotificationTime, wantNotify)
		}
	})

	t.Run("audit preserves all request fields", func(t *testing.T) {
		req := EscalationRequest{
			TicketID: "T-99",
			Category: "network",
			Priority: "high",
			Reason:   "no_solution_found",
			Attempts: 4,
		}
		_, audit := escalate(req, fixedTime, fixedID)
		if audit.Category != "network" {
			t.Errorf("audit.Category: got %q, want network", audit.Category)
		}
		if audit.Priority != "high" {
			t.Errorf("audit.Priority: got %q, want high", audit.Priority)
		}
		if audit.Reason != "no_solution_found" {
			t.Errorf("audit.Reason: got %q, want no_solution_found", audit.Reason)
		}
		if audit.Attempts != 4 {
			t.Errorf("audit.Attempts: got %d, want 4", audit.Attempts)
		}
	})

	t.Run("audit action is always 'escalated'", func(t *testing.T) {
		for _, priority := range []string{"critical", "high", "medium", "low"} {
			_, audit := escalate(EscalationRequest{Priority: priority}, fixedTime, fixedID)
			if audit.Action != "escalated" {
				t.Errorf("priority=%s: audit.Action got %q, want escalated", priority, audit.Action)
			}
		}
	})

	t.Run("non-UTC time is normalised to UTC in output", func(t *testing.T) {
		est := time.FixedZone("EST", -5*60*60)
		localTime := time.Date(2026, 5, 20, 7, 0, 0, 0, est) // same instant as fixedTime
		result, _ := escalate(EscalationRequest{Priority: "low"}, localTime, fixedID)
		const want = "2026-05-20T12:00:00Z"
		if result.Timestamp != want {
			t.Errorf("Timestamp: got %q, want %q (should be UTC)", result.Timestamp, want)
		}
	})
}

func TestNewUUID_Format(t *testing.T) {
	id := newUUID()
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 dash-separated parts, got %d: %q", len(parts), id)
	}
	lengths := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != lengths[i] {
			t.Errorf("part[%d]: expected length %d, got %d (%q)", i, lengths[i], len(p), p)
		}
	}
}

func TestNewUUID_Version4Bits(t *testing.T) {
	id := newUUID()
	parts := strings.Split(id, "-")
	// version nibble is the first hex digit of part[2] — must be '4'
	if parts[2][0] != '4' {
		t.Errorf("version nibble: got %c, want 4 (UUID v4)", parts[2][0])
	}
	// variant nibble is the first hex digit of part[3] — must be '8','9','a', or 'b'
	v := parts[3][0]
	if v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("variant nibble: got %c, want one of 8/9/a/b", v)
	}
}

func TestNewUUID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 200)
	for i := 0; i < 200; i++ {
		id := newUUID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate UUID after %d calls: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
