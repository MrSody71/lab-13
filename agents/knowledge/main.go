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

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	store, err := newRedisStore(redisURL)
	if err != nil {
		slog.Error("failed to configure Redis", "url", redisURL, "err", err)
		os.Exit(1)
	}
	defer store.Close()
	store.RestoreAndLogStats(ctx)

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

		resp, cacheHit := processQuery(ctx, store, req)

		span.SetAttributes(
			attribute.Bool("found", resp.Found),
			attribute.Int("articles_returned", len(resp.Articles)),
			attribute.Bool("cache_hit", cacheHit),
		)

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
			"found", resp.Found,
			"articles_returned", len(resp.Articles),
			"cache_hit", cacheHit,
		)
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "tickets.find_solution", "err", err)
		os.Exit(1)
	}

	_, err = nc.Subscribe("knowledge.stats", func(msg *nats.Msg) {
		if msg.Reply == "" {
			return
		}
		stats, err := store.GetStats(context.Background())
		if err != nil {
			slog.Error("failed to get stats", "err", err)
			return
		}
		payload, err := json.Marshal(stats)
		if err != nil {
			return
		}
		if err := nc.Publish(msg.Reply, payload); err != nil {
			slog.Error("failed to reply to stats request", "err", err)
		}
	})
	if err != nil {
		slog.Error("failed to subscribe", "subject", "knowledge.stats", "err", err)
		os.Exit(1)
	}

	slog.Info("subscribed, waiting for requests",
		"subjects", "tickets.find_solution, knowledge.stats",
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
}
