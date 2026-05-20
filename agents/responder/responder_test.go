package main

import (
	"strings"
	"testing"
)

func TestBuildGreeting(t *testing.T) {
	tests := []struct {
		priority   string
		wantPrefix string
	}{
		{"critical", "URGENT: "},
		{"high", "We have received"},
		{"medium", "Thank you"},
		{"low", "Thank you"},
		{"unknown", "Thank you"},
	}
	for _, tc := range tests {
		t.Run(tc.priority, func(t *testing.T) {
			got := buildGreeting(tc.priority)
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("buildGreeting(%q) = %q, want prefix %q", tc.priority, got, tc.wantPrefix)
			}
		})
	}
}

func TestETAForPriority(t *testing.T) {
	tests := []struct {
		priority string
		want     string
	}{
		{"critical", "1 hour"},
		{"high", "4 hours"},
		{"medium", "24 hours"},
		{"low", "72 hours"},
		{"unknown", "72 hours"},
	}
	for _, tc := range tests {
		t.Run(tc.priority, func(t *testing.T) {
			got := etaForPriority(tc.priority)
			if got != tc.want {
				t.Errorf("etaForPriority(%q) = %q, want %q", tc.priority, got, tc.want)
			}
		})
	}
}

func TestGenerateResponse(t *testing.T) {
	highScoreArticle := ArticleRef{ID: "KB001", Title: "VPN not connecting", Solution: "Check credentials", Score: 0.85}
	borderArticle := ArticleRef{ID: "KB002", Title: "WiFi issue", Solution: "Restart router", Score: 0.5}
	lowScoreArticle := ArticleRef{ID: "KB003", Title: "Slow internet", Solution: "Run speed test", Score: 0.4}

	tests := []struct {
		name  string
		req   ResponseRequest
		check func(*testing.T, ResponseResult)
	}{
		{
			name: "ticket_id preserved in result",
			req:  ResponseRequest{TicketID: "T-42", Priority: "low"},
			check: func(t *testing.T, r ResponseResult) {
				if r.TicketID != "T-42" {
					t.Errorf("TicketID: got %q, want %q", r.TicketID, "T-42")
				}
			},
		},
		{
			name: "ticket_id appears in response text",
			req:  ResponseRequest{TicketID: "T-42", Priority: "low"},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.Contains(r.ResponseText, "T-42") {
					t.Error("response text should contain the ticket ID")
				}
			},
		},
		{
			name: "critical response starts with URGENT",
			req:  ResponseRequest{TicketID: "T-1", Priority: "critical"},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.HasPrefix(r.ResponseText, "URGENT:") {
					t.Errorf("critical response must start with URGENT:, got: %.60s", r.ResponseText)
				}
			},
		},
		{
			name: "high priority does not get URGENT prefix",
			req:  ResponseRequest{TicketID: "T-2", Priority: "high"},
			check: func(t *testing.T, r ResponseResult) {
				if strings.HasPrefix(r.ResponseText, "URGENT:") {
					t.Error("high priority response must not start with URGENT:")
				}
			},
		},
		{
			name: "medium priority does not get URGENT prefix",
			req:  ResponseRequest{TicketID: "T-3", Priority: "medium"},
			check: func(t *testing.T, r ResponseResult) {
				if strings.HasPrefix(r.ResponseText, "URGENT:") {
					t.Error("medium priority response must not start with URGENT:")
				}
			},
		},
		{
			name: "critical ETA is 1 hour",
			req:  ResponseRequest{TicketID: "T-1", Priority: "critical"},
			check: func(t *testing.T, r ResponseResult) {
				if r.EstimatedResolution != "1 hour" {
					t.Errorf("critical ETA: got %q, want %q", r.EstimatedResolution, "1 hour")
				}
			},
		},
		{
			name: "high ETA is 4 hours",
			req:  ResponseRequest{TicketID: "T-1", Priority: "high"},
			check: func(t *testing.T, r ResponseResult) {
				if r.EstimatedResolution != "4 hours" {
					t.Errorf("high ETA: got %q, want %q", r.EstimatedResolution, "4 hours")
				}
			},
		},
		{
			name: "medium ETA is 24 hours",
			req:  ResponseRequest{TicketID: "T-1", Priority: "medium"},
			check: func(t *testing.T, r ResponseResult) {
				if r.EstimatedResolution != "24 hours" {
					t.Errorf("medium ETA: got %q, want %q", r.EstimatedResolution, "24 hours")
				}
			},
		},
		{
			name: "low ETA is 72 hours",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low"},
			check: func(t *testing.T, r ResponseResult) {
				if r.EstimatedResolution != "72 hours" {
					t.Errorf("low ETA: got %q, want %q", r.EstimatedResolution, "72 hours")
				}
			},
		},
		{
			name: "ETA string appears in response text",
			req:  ResponseRequest{TicketID: "T-1", Priority: "critical"},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.Contains(r.ResponseText, r.EstimatedResolution) {
					t.Errorf("response text should contain ETA %q", r.EstimatedResolution)
				}
			},
		},
		{
			name: "article with score > 0.5 added to kb_references",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{highScoreArticle}},
			check: func(t *testing.T, r ResponseResult) {
				if len(r.KBReferences) != 1 || r.KBReferences[0] != "KB001" {
					t.Errorf("expected [KB001], got %v", r.KBReferences)
				}
			},
		},
		{
			name: "article with score exactly 0.5 excluded (threshold is strictly >)",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{borderArticle}},
			check: func(t *testing.T, r ResponseResult) {
				if len(r.KBReferences) != 0 {
					t.Errorf("score=0.5 should be excluded, got %v", r.KBReferences)
				}
			},
		},
		{
			name: "article with score < 0.5 excluded",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{lowScoreArticle}},
			check: func(t *testing.T, r ResponseResult) {
				if len(r.KBReferences) != 0 {
					t.Errorf("score<0.5 should be excluded, got %v", r.KBReferences)
				}
			},
		},
		{
			name: "only high-score articles in kb_references when mixed",
			req: ResponseRequest{
				TicketID: "T-1", Priority: "low",
				Articles: []ArticleRef{highScoreArticle, borderArticle, lowScoreArticle},
			},
			check: func(t *testing.T, r ResponseResult) {
				if len(r.KBReferences) != 1 || r.KBReferences[0] != "KB001" {
					t.Errorf("expected only KB001, got %v", r.KBReferences)
				}
			},
		},
		{
			name: "kb_references order matches articles order",
			req: ResponseRequest{
				TicketID: "T-1", Priority: "low",
				Articles: []ArticleRef{
					{ID: "KB009", Title: "A", Solution: "S", Score: 0.9},
					{ID: "KB001", Title: "B", Solution: "S", Score: 0.8},
				},
			},
			check: func(t *testing.T, r ResponseResult) {
				if len(r.KBReferences) != 2 {
					t.Fatalf("expected 2 refs, got %d", len(r.KBReferences))
				}
				if r.KBReferences[0] != "KB009" || r.KBReferences[1] != "KB001" {
					t.Errorf("wrong order: %v", r.KBReferences)
				}
			},
		},
		{
			name: "high-score article solution appears in response text",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{highScoreArticle}},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.Contains(r.ResponseText, highScoreArticle.Solution) {
					t.Error("response text should include high-score article solution")
				}
			},
		},
		{
			name: "high-score article ID appears in response text",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{highScoreArticle}},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.Contains(r.ResponseText, "KB001") {
					t.Error("response text should include high-score article ID")
				}
			},
		},
		{
			name: "low-score article solution absent from response text",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{lowScoreArticle}},
			check: func(t *testing.T, r ResponseResult) {
				if strings.Contains(r.ResponseText, lowScoreArticle.Solution) {
					t.Error("response text should not include low-score article solution")
				}
			},
		},
		{
			name: "no articles gives non-nil empty kb_references",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Articles: []ArticleRef{}},
			check: func(t *testing.T, r ResponseResult) {
				if r.KBReferences == nil {
					t.Error("KBReferences must never be nil")
				}
				if len(r.KBReferences) != 0 {
					t.Errorf("expected empty slice, got %v", r.KBReferences)
				}
			},
		},
		{
			name: "nil articles gives non-nil empty kb_references",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low"},
			check: func(t *testing.T, r ResponseResult) {
				if r.KBReferences == nil {
					t.Error("KBReferences must never be nil even when request has no articles")
				}
			},
		},
		{
			name: "title appears in response text when set",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Title: "Printer not working"},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.Contains(r.ResponseText, "Printer not working") {
					t.Error("response text should include the issue title")
				}
			},
		},
		{
			name: "category appears in response text",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low", Category: "hardware"},
			check: func(t *testing.T, r ResponseResult) {
				if !strings.Contains(r.ResponseText, "hardware") {
					t.Error("response text should include the category")
				}
			},
		},
		{
			name: "response text is non-empty",
			req:  ResponseRequest{TicketID: "T-1", Priority: "low"},
			check: func(t *testing.T, r ResponseResult) {
				if strings.TrimSpace(r.ResponseText) == "" {
					t.Error("response text must not be empty")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, generateResponse(tc.req))
		})
	}
}
