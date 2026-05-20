package main

import (
	"strings"
	"testing"
)

func TestComputeBid_IdleAgent(t *testing.T) {
	bid := computeBid("test-agent", 0, false)

	if bid.AgentID != "test-agent" {
		t.Errorf("AgentID: want test-agent, got %s", bid.AgentID)
	}
	if bid.Cost != baseCost {
		t.Errorf("idle Cost: want %.1f, got %.1f", baseCost, bid.Cost)
	}
	if bid.Capacity != 3 {
		t.Errorf("idle Capacity: want 3, got %d", bid.Capacity)
	}
	if bid.EtaMs != 100 {
		t.Errorf("idle EtaMs: want 100, got %d", bid.EtaMs)
	}
}

func TestComputeBid_BusyAgent(t *testing.T) {
	bid := computeBid("busy-agent", 0, true)

	if bid.Capacity != 1 {
		t.Errorf("busy Capacity: want 1, got %d", bid.Capacity)
	}
}

func TestComputeBid_QueueDepthScalesCost(t *testing.T) {
	for _, depth := range []int64{0, 1, 5, 10} {
		bid := computeBid("a", depth, false)
		wantCost := float64(depth)*10.0 + baseCost
		if bid.Cost != wantCost {
			t.Errorf("depth=%d: Cost want %.1f, got %.1f", depth, wantCost, bid.Cost)
		}
	}
}

func TestComputeBid_QueueDepthScalesETA(t *testing.T) {
	bid0 := computeBid("a", 0, false)
	bid3 := computeBid("a", 3, false)
	wantEta := bid0.EtaMs + 3*50
	if bid3.EtaMs != wantEta {
		t.Errorf("EtaMs: want %d, got %d", wantEta, bid3.EtaMs)
	}
}

func TestComputeBid_BusyDoesNotAffectCostOrETA(t *testing.T) {
	idle := computeBid("a", 2, false)
	busy := computeBid("a", 2, true)

	if idle.Cost != busy.Cost {
		t.Errorf("busy flag should not change Cost: idle=%.1f busy=%.1f", idle.Cost, busy.Cost)
	}
	if idle.EtaMs != busy.EtaMs {
		t.Errorf("busy flag should not change EtaMs: idle=%d busy=%d", idle.EtaMs, busy.EtaMs)
	}
}

func TestNewAgentID_NotEmpty(t *testing.T) {
	id := newAgentID()
	if id == "" {
		t.Error("agentID must not be empty")
	}
}

func TestNewAgentID_ContainsDash(t *testing.T) {
	id := newAgentID()
	if !strings.Contains(id, "-") {
		t.Errorf("agentID should contain a dash separator, got %q", id)
	}
}

func TestNewAgentID_HasFourDigitSuffix(t *testing.T) {
	for range 20 {
		id := newAgentID()
		parts := strings.Split(id, "-")
		suffix := parts[len(parts)-1]
		if len(suffix) != 4 {
			t.Errorf("suffix should be 4 digits, got %q in %q", suffix, id)
		}
	}
}

func TestNewAgentID_UniqueAcrossCalls(t *testing.T) {
	ids := make(map[string]bool, 100)
	for range 100 {
		ids[newAgentID()] = true
	}
	// With 9000 possible suffixes, 100 draws should give >1 unique value
	if len(ids) < 2 {
		t.Error("newAgentID should produce multiple distinct values across 100 calls")
	}
}
