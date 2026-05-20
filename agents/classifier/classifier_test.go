package main

import "testing"

func TestClassifyCategory(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantCat    string
		wantConf   float64
	}{
		{"network keyword", "wifi not connecting", "network", 0.85},
		{"network vpn", "vpn tunnel dropped", "network", 0.85},
		{"network internet", "internet outage on floor 3", "network", 0.85},
		{"hardware crash", "machine keeps crashing", "hardware", 0.85},
		{"hardware blue screen", "blue screen on startup", "hardware", 0.85},
		{"hardware reboot", "unexpected reboot every morning", "hardware", 0.85},
		{"software bug", "bug in the accounting app", "software", 0.85},
		{"software install", "need to install new drivers", "software", 0.85},
		{"software error", "getting an error on launch", "software", 0.85},
		{"account password", "forgot my password", "account", 0.85},
		{"account login", "cannot login to portal", "account", 0.85},
		{"account access", "need access to shared drive", "account", 0.85},
		{"other fallback", "my chair is broken", "other", 0.60},
		{"other empty", "", "other", 0.60},
		// category is determined by first match — network beats software even if both present
		{"first match wins", "network error on install", "network", 0.85},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, conf := classifyCategory(tc.text)
			if cat != tc.wantCat {
				t.Errorf("category: got %q, want %q", cat, tc.wantCat)
			}
			if conf != tc.wantConf {
				t.Errorf("confidence: got %v, want %v", conf, tc.wantConf)
			}
		})
	}
}

func TestClassifyPriority(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		want     string
	}{
		{"critical keyword", "system is critical", "critical"},
		{"urgent", "urgent: server down", "critical"},
		{"down", "website is down", "critical"},
		{"not working", "printer not working", "critical"},
		{"medium slow", "computer is slow today", "medium"},
		{"medium issue", "having an issue with email", "medium"},
		{"medium problem", "problem printing", "medium"},
		{"low fallback", "requesting new keyboard", "low"},
		{"low empty", "", "low"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPriority(tc.text)
			if got != tc.want {
				t.Errorf("priority: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		ticket   Ticket
		wantCat  string
		wantPri  string
		wantConf float64
	}{
		{
			name:     "network critical",
			ticket:   Ticket{TicketID: "T-1", Title: "VPN is down", Description: "urgent, nobody can connect"},
			wantCat:  "network",
			wantPri:  "critical",
			wantConf: 0.85,
		},
		{
			name:     "software medium",
			ticket:   Ticket{TicketID: "T-2", Title: "Bug in billing", Description: "there is a problem with invoices"},
			wantCat:  "software",
			wantPri:  "medium",
			wantConf: 0.85,
		},
		{
			name:     "account low",
			ticket:   Ticket{TicketID: "T-3", Title: "New account request", Description: "please create a login for the intern"},
			wantCat:  "account",
			wantPri:  "low",
			wantConf: 0.85,
		},
		{
			name:     "other low",
			ticket:   Ticket{TicketID: "T-4", Title: "Office supplies", Description: "need more paper"},
			wantCat:  "other",
			wantPri:  "low",
			wantConf: 0.60,
		},
		{
			name:     "ticket_id preserved",
			ticket:   Ticket{TicketID: "abc-999", Title: "reboot loop", Description: ""},
			wantCat:  "hardware",
			wantPri:  "low",
			wantConf: 0.85,
		},
		{
			name:     "case insensitive title",
			ticket:   Ticket{TicketID: "T-5", Title: "WIFI DOWN", Description: "URGENT"},
			wantCat:  "network",
			wantPri:  "critical",
			wantConf: 0.85,
		},
		{
			name:     "description only match",
			ticket:   Ticket{TicketID: "T-6", Title: "Help needed", Description: "cannot install the update"},
			wantCat:  "software",
			wantPri:  "low",
			wantConf: 0.85,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.ticket)
			if got.TicketID != tc.ticket.TicketID {
				t.Errorf("ticket_id: got %q, want %q", got.TicketID, tc.ticket.TicketID)
			}
			if got.Category != tc.wantCat {
				t.Errorf("category: got %q, want %q", got.Category, tc.wantCat)
			}
			if got.Priority != tc.wantPri {
				t.Errorf("priority: got %q, want %q", got.Priority, tc.wantPri)
			}
			if got.Confidence != tc.wantConf {
				t.Errorf("confidence: got %v, want %v", got.Confidence, tc.wantConf)
			}
		})
	}
}
