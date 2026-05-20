package main

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyTotalQueries      = "knowledge:stats:total_queries"
	keyFoundCount        = "knowledge:stats:found_count"
	keyArticleHitsPrefix = "knowledge:article_hits:"
	keyCachePrefix       = "knowledge:cache:"
	cacheTTL             = 5 * time.Minute
)

// StoreStats holds aggregated knowledge-base usage statistics.
type StoreStats struct {
	TotalQueries int64            `json:"total_queries"`
	FoundCount   int64            `json:"found_count"`
	TopArticles  []ArticleHitCount `json:"top_articles"`
}

// ArticleHitCount pairs an article ID with its cumulative hit count.
type ArticleHitCount struct {
	ArticleID string `json:"article_id"`
	Hits      int64  `json:"hits"`
}

// Store abstracts Redis so tests can inject an in-memory fake.
type Store interface {
	IncrTotalQueries(ctx context.Context)
	IncrFoundCount(ctx context.Context)
	IncrArticleHit(ctx context.Context, articleID string)
	GetCached(ctx context.Context, ticketID string) ([]byte, bool)
	SetCached(ctx context.Context, ticketID string, data []byte)
	GetStats(ctx context.Context) (StoreStats, error)
	RestoreAndLogStats(ctx context.Context)
}

// RedisStore is the production Store backed by Redis.
type RedisStore struct {
	rdb *redis.Client
}

// newRedisStore validates rawURL and returns a RedisStore.
// The TCP connection is established lazily on the first command.
func newRedisStore(rawURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, err
	}
	return &RedisStore{rdb: redis.NewClient(opts)}, nil
}

func (s *RedisStore) Close() error { return s.rdb.Close() }

func (s *RedisStore) IncrTotalQueries(ctx context.Context) {
	if err := s.rdb.Incr(ctx, keyTotalQueries).Err(); err != nil {
		slog.Warn("redis: incr total_queries failed", "err", err)
	}
}

func (s *RedisStore) IncrFoundCount(ctx context.Context) {
	if err := s.rdb.Incr(ctx, keyFoundCount).Err(); err != nil {
		slog.Warn("redis: incr found_count failed", "err", err)
	}
}

func (s *RedisStore) IncrArticleHit(ctx context.Context, articleID string) {
	if err := s.rdb.Incr(ctx, keyArticleHitsPrefix+articleID).Err(); err != nil {
		slog.Warn("redis: incr article hit failed", "article_id", articleID, "err", err)
	}
}

func (s *RedisStore) GetCached(ctx context.Context, ticketID string) ([]byte, bool) {
	data, err := s.rdb.Get(ctx, keyCachePrefix+ticketID).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false
	}
	if err != nil {
		slog.Warn("redis: get cache failed", "ticket_id", ticketID, "err", err)
		return nil, false
	}
	return data, true
}

func (s *RedisStore) SetCached(ctx context.Context, ticketID string, data []byte) {
	if err := s.rdb.Set(ctx, keyCachePrefix+ticketID, data, cacheTTL).Err(); err != nil {
		slog.Warn("redis: set cache failed", "ticket_id", ticketID, "err", err)
	}
}

// GetStats fetches all counters in a single pipeline and returns the top-5 articles by hit count.
func (s *RedisStore) GetStats(ctx context.Context) (StoreStats, error) {
	pipe := s.rdb.Pipeline()
	totalQCmd := pipe.Get(ctx, keyTotalQueries)
	foundCCmd := pipe.Get(ctx, keyFoundCount)

	hitCmds := make(map[string]*redis.StringCmd, len(knowledgeBase))
	for _, a := range knowledgeBase {
		hitCmds[a.ID] = pipe.Get(ctx, keyArticleHitsPrefix+a.ID)
	}

	// Ignore the combined error — each command's Val()/Err() is checked individually.
	_, _ = pipe.Exec(ctx)

	totalQueries, _ := strconv.ParseInt(totalQCmd.Val(), 10, 64)
	foundCount, _ := strconv.ParseInt(foundCCmd.Val(), 10, 64)

	type hitEntry struct {
		id   string
		hits int64
	}
	entries := make([]hitEntry, 0, len(knowledgeBase))
	for id, cmd := range hitCmds {
		if n, err := strconv.ParseInt(cmd.Val(), 10, 64); err == nil && n > 0 {
			entries = append(entries, hitEntry{id: id, hits: n})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].hits != entries[j].hits {
			return entries[i].hits > entries[j].hits
		}
		return entries[i].id < entries[j].id
	})

	topN := 5
	if len(entries) < topN {
		topN = len(entries)
	}
	topArticles := make([]ArticleHitCount, topN)
	for i := range topN {
		topArticles[i] = ArticleHitCount{ArticleID: entries[i].id, Hits: entries[i].hits}
	}

	return StoreStats{
		TotalQueries: totalQueries,
		FoundCount:   foundCount,
		TopArticles:  topArticles,
	}, nil
}

func (s *RedisStore) RestoreAndLogStats(ctx context.Context) {
	stats, err := s.GetStats(ctx)
	if err != nil {
		slog.Warn("failed to restore stats from Redis", "err", err)
		return
	}
	slog.Info("Restored state",
		"total_queries", stats.TotalQueries,
		"found_count", stats.FoundCount,
	)
}
