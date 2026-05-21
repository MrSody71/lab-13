package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
)

// BidRequest is received from "agents.bid_request" when the orchestrator
// is running an auction.
type BidRequest struct {
	TaskID     string          `json:"task_id"`
	TaskType   string          `json:"task_type"`
	Payload    json.RawMessage `json:"payload"`
	DeadlineMs int             `json:"deadline_ms"`
}

// BidResponse is published to "agents.bid_response.<task_id>".
type BidResponse struct {
	AgentID  string  `json:"agent_id"`
	Cost     float64 `json:"cost"`
	Capacity int     `json:"capacity"`
	EtaMs    int     `json:"eta_ms"`
}

const baseCost = 50.0

// computeBid returns a bid for the given agent state.
// Pure function — no side effects; pass in queueDepth and busy so tests can
// drive any scenario without touching shared state.
func computeBid(agentID string, queueDepth int64, busy bool) BidResponse {
	cost := float64(queueDepth)*10.0 + baseCost
	capacity := 3
	if busy {
		capacity = 1
	}
	return BidResponse{
		AgentID:  agentID,
		Cost:     cost,
		Capacity: capacity,
		EtaMs:    100 + int(queueDepth)*50,
	}
}

// newAgentID returns a unique identifier built from the OS hostname and a
// random 4-digit suffix so each replica is distinguishable in logs.
func newAgentID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "classifier"
	}
	return fmt.Sprintf("%s-%04d", hostname, rand.IntN(9000)+1000)
}
