package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/nats-io/nats.go"
)

func main() {
	ctx := context.Background()

	shutdown, err := initTracer(ctx)
	if err != nil {
		slog.Warn("tracer init failed, continuing without tracing", "err", err)
	} else {
		defer shutdown(ctx)
	}

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
	slog.Info("knowledge base loaded", "articles", len(knowledgeBase))

	tracer := otel.Tracer("knowledge")

	_, err = nc.Subscribe("tickets.find_solution", func(msg *nats.Msg) {
		ctx, span := tracer.Start(extractCtx(msg), "process.tickets.find_solution")
		defer span.End()

		var req SolutionRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to decode request", "err", err)
			return
		}

		span.SetAttributes(
			attribute.String("ticket_id", req.TicketID),
			attribute.String("category", req.Category),
		)

		slog.Info("searching knowledge base", "ticket_id", req.TicketID, "category", req.Category)

		articles, found := search(knowledgeBase, req.Category, req.Title, req.Description, 3)

		span.SetAttributes(
			attribute.Bool("found", found),
			attribute.Int("articles_returned", len(articles)),
		)

		resp := SolutionResponse{
			TicketID: req.TicketID,
			Articles: articles,
			Found:    found,
		}

		payload, err := json.Marshal(resp)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to encode response", "ticket_id", req.TicketID, "err", err)
			return
		}

		out := nats.NewMsg("tickets.solution_found")
		out.Data = payload
		injectHeaders(ctx, out)

		if err := nc.PublishMsg(out); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			slog.Error("failed to publish response", "ticket_id", req.TicketID, "err", err)
			return
		}

		slog.Info("solution search complete",
			"ticket_id", req.TicketID,
			"found", found,
			"articles_returned", len(articles),
		)
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.find_solution", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for requests", "subject", "tickets.find_solution")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}
