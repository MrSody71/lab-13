package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	nc, err := nats.Connect(natsURL)
	if err != nil {
		slog.Error("failed to connect to NATS", "url", natsURL, "err", err)
		os.Exit(1)
	}
	defer nc.Drain()

	slog.Info("connected to NATS", "url", natsURL)

	_, err = nc.Subscribe("tickets.escalate", func(msg *nats.Msg) {
		var req EscalationRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			slog.Error("failed to decode request", "err", err)
			return
		}

		slog.Info("escalating ticket",
			"ticket_id", req.TicketID,
			"priority", req.Priority,
			"category", req.Category,
			"attempts", req.Attempts,
			"reason", req.Reason,
		)

		result, audit := escalate(req, time.Now(), newUUID())

		resultPayload, err := json.Marshal(result)
		if err != nil {
			slog.Error("failed to encode result", "ticket_id", req.TicketID, "err", err)
			return
		}
		if err := nc.Publish("tickets.escalated", resultPayload); err != nil {
			slog.Error("failed to publish escalation", "ticket_id", req.TicketID, "err", err)
			return
		}

		if auditPayload, err := json.Marshal(audit); err == nil {
			if err := nc.Publish("tickets.audit", auditPayload); err != nil {
				slog.Warn("audit publish failed", "ticket_id", req.TicketID, "err", err)
			}
		}

		slog.Info("ticket escalated",
			"ticket_id", result.TicketID,
			"escalated_to", result.EscalatedTo,
			"notify", result.NotificationTime,
			"escalation_id", result.EscalationID,
		)
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.escalate", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for requests", "subject", "tickets.escalate")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}
