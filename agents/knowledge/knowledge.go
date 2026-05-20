package main

import (
	"math"
	"sort"
	"strings"
)

type Article struct {
	ID       string
	Title    string
	Category string
	Keywords []string
	Solution string
}

type ArticleResult struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Solution string  `json:"solution"`
	Score    float64 `json:"score"`
}

type SolutionRequest struct {
	TicketID    string `json:"ticket_id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type SolutionResponse struct {
	TicketID string          `json:"ticket_id"`
	Articles []ArticleResult `json:"articles"`
	Found    bool            `json:"found"`
}

const categoryBonus = 0.15

var knowledgeBase = []Article{
	{
		ID: "KB001", Title: "VPN not connecting", Category: "network",
		Keywords: []string{"vpn", "connect", "tunnel", "credentials"},
		Solution: "1. Verify VPN credentials and server address.\n2. Disable/re-enable the VPN client.\n3. Flush DNS: run 'ipconfig /flushdns'.\n4. Re-import the VPN certificate if required.",
	},
	{
		ID: "KB002", Title: "WiFi dropping or no signal", Category: "network",
		Keywords: []string{"wifi", "wireless", "signal", "drops"},
		Solution: "1. Restart the wireless adapter via Device Manager.\n2. Forget and reconnect to the WiFi network.\n3. Move closer to the access point.\n4. Update the wireless driver.",
	},
	{
		ID: "KB003", Title: "Slow internet connection", Category: "network",
		Keywords: []string{"internet", "slow", "bandwidth", "speed"},
		Solution: "1. Run a speed test at fast.com.\n2. Restart the router and modem.\n3. Disconnect other devices from the network.\n4. Contact ISP if throughput stays below contract.",
	},
	{
		ID: "KB004", Title: "Blue screen of death (BSOD)", Category: "hardware",
		Keywords: []string{"blue screen", "bsod", "driver", "memory"},
		Solution: "1. Note the stop code shown on the blue screen.\n2. Boot into Safe Mode and check Event Viewer.\n3. Run Windows Memory Diagnostic.\n4. Update or roll back recently changed drivers.",
	},
	{
		ID: "KB005", Title: "Computer will not power on", Category: "hardware",
		Keywords: []string{"power", "boot", "start", "beep"},
		Solution: "1. Check that the power cable is seated at both ends.\n2. Test with a different outlet.\n3. Listen for POST beep codes and look them up.\n4. Disconnect all peripherals and retry.",
	},
	{
		ID: "KB006", Title: "Application crashes on launch", Category: "software",
		Keywords: []string{"crash", "application", "launch", "error"},
		Solution: "1. Reinstall the application (full uninstall first).\n2. Check Event Viewer > Application log for crash details.\n3. Run the app as Administrator.\n4. Verify that system requirements are met.",
	},
	{
		ID: "KB007", Title: "Software update or install failing", Category: "software",
		Keywords: []string{"update", "install", "windows", "patch"},
		Solution: "1. Free at least 10 GB of disk space.\n2. Run the Windows Update troubleshooter.\n3. Manually download the update from Microsoft Catalog.\n4. Reset Windows Update components via command line.",
	},
	{
		ID: "KB008", Title: "Printer not printing", Category: "software",
		Keywords: []string{"printer", "print", "driver", "queue"},
		Solution: "1. Restart the Print Spooler service (services.msc).\n2. Clear the print queue.\n3. Remove and re-add the printer.\n4. Download the latest driver from the manufacturer.",
	},
	{
		ID: "KB009", Title: "Password reset or expired", Category: "account",
		Keywords: []string{"password", "reset", "expired", "credentials"},
		Solution: "1. Use the self-service reset portal at password.company.com.\n2. Contact the helpdesk with your employee ID if the portal is unavailable.\n3. Update saved passwords in all browsers and apps.\n4. Enable MFA to reduce future lockouts.",
	},
	{
		ID: "KB010", Title: "Account locked out", Category: "account",
		Keywords: []string{"locked", "account", "login", "attempts"},
		Solution: "1. Wait 15 minutes for the auto-unlock policy to trigger.\n2. Contact IT with your employee ID and manager's name.\n3. Review recent login attempts for unauthorized access.\n4. Change your password after unlock as a precaution.",
	},
	{
		ID: "KB011", Title: "Email not syncing or loading", Category: "other",
		Keywords: []string{"email", "sync", "outlook", "calendar"},
		Solution: "1. Confirm your internet connection is active.\n2. Restart Outlook and wait 5 minutes for a full sync.\n3. Remove and re-add the email account in Outlook settings.\n4. Verify your mailbox is not over storage quota.",
	},
}

// keywordScore returns the fraction of a's keywords found in text (case-insensitive).
func keywordScore(a Article, text string) float64 {
	if len(a.Keywords) == 0 {
		return 0
	}
	lower := strings.ToLower(text)
	hits := 0
	for _, kw := range a.Keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hits++
		}
	}
	return float64(hits) / float64(len(a.Keywords))
}

// search returns the top-N articles ranked by keyword overlap + optional category bonus.
func search(kb []Article, category, title, description string, topN int) ([]ArticleResult, bool) {
	if topN <= 0 {
		return make([]ArticleResult, 0), false
	}

	text := title + " " + description

	type candidate struct {
		result ArticleResult
		score  float64
	}

	candidates := make([]candidate, 0, len(kb))
	for _, a := range kb {
		s := keywordScore(a, text)
		if category != "" && a.Category == category {
			s += categoryBonus
			if s > 1.0 {
				s = 1.0
			}
		}
		candidates = append(candidates, candidate{
			result: ArticleResult{ID: a.ID, Title: a.Title, Solution: a.Solution, Score: s},
			score:  s,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		diff := candidates[i].score - candidates[j].score
		if math.Abs(diff) > 1e-9 {
			return diff > 0
		}
		return candidates[i].result.ID < candidates[j].result.ID
	})

	out := make([]ArticleResult, 0, topN)
	for _, c := range candidates {
		if c.score < 1e-9 || len(out) >= topN {
			break
		}
		out = append(out, c.result)
	}

	return out, len(out) > 0
}
