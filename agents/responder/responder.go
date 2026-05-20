package main

import (
	"fmt"
	"strings"
)

type ArticleRef struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Solution string  `json:"solution"`
	Score    float64 `json:"score"`
}

type ResponseRequest struct {
	TicketID string       `json:"ticket_id"`
	Category string       `json:"category"`
	Priority string       `json:"priority"`
	Articles []ArticleRef `json:"articles"`
	Title    string       `json:"title"`
}

type ResponseResult struct {
	TicketID            string   `json:"ticket_id"`
	ResponseText        string   `json:"response_text"`
	EstimatedResolution string   `json:"estimated_resolution"`
	KBReferences        []string `json:"kb_references"`
}

func buildGreeting(priority string) string {
	switch priority {
	case "critical":
		return "URGENT: We have received your critical issue report and are treating it as our highest priority."
	case "high":
		return "We have received your high-priority request and have escalated it to our support team."
	case "medium":
		return "Thank you for contacting IT Support. We have received your ticket and will address it soon."
	default:
		return "Thank you for contacting IT Support. We have received your request."
	}
}

func etaForPriority(priority string) string {
	switch priority {
	case "critical":
		return "1 hour"
	case "high":
		return "4 hours"
	case "medium":
		return "24 hours"
	default:
		return "72 hours"
	}
}

func generateResponse(req ResponseRequest) ResponseResult {
	eta := etaForPriority(req.Priority)

	kbRefs := make([]string, 0)
	var kbBody strings.Builder
	for _, a := range req.Articles {
		if a.Score > 0.5 {
			kbRefs = append(kbRefs, a.ID)
			fmt.Fprintf(&kbBody, "[%s] %s:\n%s\n\n", a.ID, a.Title, a.Solution)
		}
	}

	var sb strings.Builder
	sb.WriteString(buildGreeting(req.Priority))
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "Ticket Reference: #%s\n", req.TicketID)
	if req.Title != "" {
		fmt.Fprintf(&sb, "Issue: %s\n", req.Title)
	}
	fmt.Fprintf(&sb, "Category: %s | Priority: %s\n", req.Category, req.Priority)

	if kbBody.Len() > 0 {
		sb.WriteString("\nThe following solution(s) from our knowledge base may help:\n\n")
		sb.WriteString(kbBody.String())
	}

	fmt.Fprintf(&sb, "Our team will investigate and respond within %s.\n", eta)
	fmt.Fprintf(&sb, "\nFor follow-up, please use your ticket reference: #%s\n", req.TicketID)
	sb.WriteString("\nIT Support Team")

	return ResponseResult{
		TicketID:            req.TicketID,
		ResponseText:        sb.String(),
		EstimatedResolution: eta,
		KBReferences:        kbRefs,
	}
}
