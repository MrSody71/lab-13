package main

import "strings"

type Ticket struct {
	TicketID    string `json:"ticket_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type Classification struct {
	TicketID   string  `json:"ticket_id"`
	Category   string  `json:"category"`
	Priority   string  `json:"priority"`
	Confidence float64 `json:"confidence"`
}

var categoryRules = []struct {
	category string
	keywords []string
}{
	{"network", []string{"network", "internet", "vpn", "wifi"}},
	{"hardware", []string{"crash", "blue screen", "reboot", "hardware"}},
	{"software", []string{"software", "install", "update", "bug", "error"}},
	{"account", []string{"password", "login", "access", "account"}},
}

var priorityRules = []struct {
	priority string
	keywords []string
}{
	{"critical", []string{"critical", "urgent", "down", "not working"}},
	{"medium", []string{"slow", "issue", "problem"}},
}

func classifyCategory(text string) (category string, confidence float64) {
	for _, rule := range categoryRules {
		for _, kw := range rule.keywords {
			if strings.Contains(text, kw) {
				return rule.category, 0.85
			}
		}
	}
	return "other", 0.60
}

func classifyPriority(text string) string {
	for _, rule := range priorityRules {
		for _, kw := range rule.keywords {
			if strings.Contains(text, kw) {
				return rule.priority
			}
		}
	}
	return "low"
}

func classify(t Ticket) Classification {
	text := strings.ToLower(t.Title + " " + t.Description)
	category, confidence := classifyCategory(text)
	priority := classifyPriority(text)
	return Classification{
		TicketID:   t.TicketID,
		Category:   category,
		Priority:   priority,
		Confidence: confidence,
	}
}
