package main

import "time"

type EscalationRequest struct {
	TicketID string `json:"ticket_id"`
	Category string `json:"category"`
	Priority string `json:"priority"`
	Reason   string `json:"reason"`
	Attempts int    `json:"attempts"`
}

type EscalationResult struct {
	TicketID         string `json:"ticket_id"`
	EscalatedTo      string `json:"escalated_to"`
	NotificationTime string `json:"notification_time"`
	EscalationID     string `json:"escalation_id"`
	Timestamp        string `json:"timestamp"`
}

type AuditEntry struct {
	TicketID         string `json:"ticket_id"`
	EscalationID     string `json:"escalation_id"`
	EscalatedTo      string `json:"escalated_to"`
	NotificationTime string `json:"notification_time"`
	Reason           string `json:"reason"`
	Priority         string `json:"priority"`
	Category         string `json:"category"`
	Attempts         int    `json:"attempts"`
	Timestamp        string `json:"timestamp"`
	Action           string `json:"action"`
}

// determineTarget returns the team and notification deadline for the given request.
// Rules are evaluated in priority order; the first match wins.
func determineTarget(req EscalationRequest) (team, notify string) {
	switch {
	case req.Priority == "critical" && req.Attempts >= 2:
		return "L3_TEAM", "immediate"
	case req.Priority == "high" && req.Attempts >= 3:
		return "L2_TEAM", "1h"
	case req.Reason == "no_solution_found" && req.Category == "network":
		return "NETWORK_OPS", "2h"
	default:
		return "L1_TEAM", "4h"
	}
}

// escalate is a pure function: given a request, a wall-clock time, and a pre-generated
// ID it produces the escalation result and audit entry with no side effects.
func escalate(req EscalationRequest, now time.Time, id string) (EscalationResult, AuditEntry) {
	team, notify := determineTarget(req)
	ts := now.UTC().Format(time.RFC3339)

	result := EscalationResult{
		TicketID:         req.TicketID,
		EscalatedTo:      team,
		NotificationTime: notify,
		EscalationID:     id,
		Timestamp:        ts,
	}

	audit := AuditEntry{
		TicketID:         req.TicketID,
		EscalationID:     id,
		EscalatedTo:      team,
		NotificationTime: notify,
		Reason:           req.Reason,
		Priority:         req.Priority,
		Category:         req.Category,
		Attempts:         req.Attempts,
		Timestamp:        ts,
		Action:           "escalated",
	}

	return result, audit
}
