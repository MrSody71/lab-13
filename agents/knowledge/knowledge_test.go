package main

import (
	"math"
	"testing"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// testKB has 5 articles with 4 keywords each (clean quarter fractions) and no keyword overlap.
var testKB = []Article{
	{ID: "T001", Title: "VPN issue", Category: "network",
		Keywords: []string{"vpn", "connect", "tunnel", "down"}, Solution: "Fix A"},
	{ID: "T002", Title: "Software issue", Category: "software",
		Keywords: []string{"install", "error", "update", "fail"}, Solution: "Fix B"},
	{ID: "T003", Title: "WiFi issue", Category: "network",
		Keywords: []string{"wifi", "signal", "speed", "router"}, Solution: "Fix C"},
	{ID: "T004", Title: "Account issue", Category: "account",
		Keywords: []string{"password", "login", "reset", "locked"}, Solution: "Fix D"},
	{ID: "T005", Title: "Hardware issue", Category: "hardware",
		Keywords: []string{"crash", "boot", "power", "memory"}, Solution: "Fix E"},
}

func TestKeywordScore(t *testing.T) {
	tests := []struct {
		name string
		kws  []string
		text string
		want float64
	}{
		{"all 4 match", []string{"vpn", "connect", "tunnel", "down"}, "vpn connect tunnel down", 1.0},
		{"2 of 4", []string{"vpn", "connect", "tunnel", "down"}, "vpn connect issue today", 0.5},
		{"1 of 4", []string{"vpn", "connect", "tunnel", "down"}, "vpn issue reported", 0.25},
		{"0 of 4", []string{"vpn", "connect", "tunnel", "down"}, "printer paper jam", 0.0},
		{"empty text", []string{"vpn", "connect"}, "", 0.0},
		{"empty keywords", []string{}, "vpn connect issue", 0.0},
		{"uppercase text normalized", []string{"vpn", "connect"}, "VPN CONNECT ISSUE", 1.0},
		{"uppercase keywords normalized", []string{"VPN", "CONNECT"}, "vpn connect issue", 1.0},
		{"multi-word keyword matched", []string{"blue screen", "crash"}, "computer showing blue screen of death", 0.5},
		{"multi-word keyword missed", []string{"blue screen", "crash"}, "display flickering", 0.0},
		{"substring match", []string{"install"}, "installer running slowly", 1.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := Article{Keywords: tc.kws}
			got := keywordScore(a, tc.text)
			if !approxEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSearch_SingleMatch(t *testing.T) {
	results, found := search(testKB, "", "vpn tunnel issue", "", 3)
	if !found {
		t.Fatal("expected found=true")
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].ID != "T001" {
		t.Errorf("expected T001 first, got %s", results[0].ID)
	}
	if !approxEqual(results[0].Score, 0.5) {
		t.Errorf("expected score 0.5 for T001, got %v", results[0].Score)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	results, found := search(testKB, "", "unrelated topic about coffee", "", 3)
	if found {
		t.Error("expected found=false")
	}
	if results == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_TopNLimit(t *testing.T) {
	// One keyword from each of the 5 articles; all tie at 0.25, ID tiebreak → T001,T002,T003,T004,T005
	results, found := search(testKB, "", "vpn install wifi password crash", "", 3)
	if !found {
		t.Fatal("expected found=true")
	}
	if len(results) != 3 {
		t.Fatalf("expected exactly 3 results (topN limit), got %d", len(results))
	}
	for i, wantID := range []string{"T001", "T002", "T003"} {
		if results[i].ID != wantID {
			t.Errorf("results[%d]: got %s, want %s", i, results[i].ID, wantID)
		}
	}
}

func TestSearch_OrderByScore(t *testing.T) {
	// T001 matches vpn+connect (2/4=0.5); T003 matches nothing (0)
	results, _ := search(testKB, "", "vpn connect", "", 3)
	if results[0].ID != "T001" {
		t.Errorf("expected T001 first (score 0.5), got %s", results[0].ID)
	}
	for _, r := range results[1:] {
		if r.Score >= results[0].Score {
			t.Errorf("results not sorted: %s (%.2f) should be after T001 (%.2f)", r.ID, r.Score, results[0].Score)
		}
	}
}

func TestSearch_CategoryBoostAddsResult(t *testing.T) {
	// "vpn" alone → T001 scores 0.25; T003 scores 0 (no keyword hit)
	// with category="network" → T003 gains 0.15 and appears in results
	withoutCat, _ := search(testKB, "", "vpn", "", 3)
	withCat, _ := search(testKB, "network", "vpn", "", 3)

	if len(withCat) <= len(withoutCat) {
		t.Errorf("category boost should add more results: without=%d, with=%d", len(withoutCat), len(withCat))
	}

	foundT003 := false
	for _, r := range withCat {
		if r.ID == "T003" {
			foundT003 = true
		}
	}
	if !foundT003 {
		t.Error("T003 should appear via category boost even with 0 keyword score")
	}
}

func TestSearch_CategoryBoostIncreasesScore(t *testing.T) {
	without, _ := search(testKB, "", "vpn connect", "", 3)
	with, _ := search(testKB, "network", "vpn connect", "", 3)

	scoreWithout, scoreWith := -1.0, -1.0
	for _, r := range without {
		if r.ID == "T001" {
			scoreWithout = r.Score
		}
	}
	for _, r := range with {
		if r.ID == "T001" {
			scoreWith = r.Score
		}
	}
	if scoreWith <= scoreWithout {
		t.Errorf("category boost should raise T001 score: without=%.2f, with=%.2f", scoreWithout, scoreWith)
	}
}

func TestSearch_ScoreCappedAt1(t *testing.T) {
	// All 4 keywords hit → 1.0; category bonus would push to 1.15 → must cap at 1.0
	results, _ := search(testKB, "network", "vpn connect tunnel down", "", 3)
	for _, r := range results {
		if r.ID == "T001" && r.Score > 1.0 {
			t.Errorf("score must be capped at 1.0, got %v", r.Score)
		}
	}
}

func TestSearch_EmptyKB(t *testing.T) {
	results, found := search([]Article{}, "", "vpn connect", "", 3)
	if found {
		t.Error("expected found=false for empty knowledge base")
	}
	if results == nil {
		t.Error("expected non-nil empty slice")
	}
}

func TestSearch_TopNZero(t *testing.T) {
	results, found := search(testKB, "", "vpn connect", "", 0)
	if found {
		t.Error("expected found=false for topN=0")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for topN=0, got %d", len(results))
	}
}

func TestSearch_ResultScoreMatchesArticle(t *testing.T) {
	results, _ := search(testKB, "", "vpn connect tunnel down", "", 3)
	for _, r := range results {
		if r.ID == "T001" && !approxEqual(r.Score, 1.0) {
			t.Errorf("T001 with all 4 keywords matched: expected score 1.0, got %v", r.Score)
		}
	}
}

func TestSearch_RealKB_VPN(t *testing.T) {
	results, found := search(knowledgeBase, "network", "VPN tunnel not connecting", "credentials expired", 3)
	if !found {
		t.Fatal("expected found=true for VPN query against real KB")
	}
	if results[0].ID != "KB001" {
		t.Errorf("expected KB001 first for VPN query, got %s", results[0].ID)
	}
}

func TestSearch_RealKB_Password(t *testing.T) {
	// KB009 keywords: password, reset, expired, credentials — all 4 present in query
	results, found := search(knowledgeBase, "account", "forgot password", "cannot login credentials expired", 3)
	if !found {
		t.Fatal("expected found=true for password query against real KB")
	}
	if results[0].ID != "KB009" {
		t.Errorf("expected KB009 first for password/credentials query, got %s", results[0].ID)
	}
}

func TestSearch_RealKB_BSOD(t *testing.T) {
	results, found := search(knowledgeBase, "hardware", "blue screen crash", "driver memory failure bsod", 3)
	if !found {
		t.Fatal("expected found=true for BSOD query against real KB")
	}
	if results[0].ID != "KB004" {
		t.Errorf("expected KB004 first for BSOD query, got %s", results[0].ID)
	}
}
