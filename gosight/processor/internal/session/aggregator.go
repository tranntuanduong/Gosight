package session

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/gosight/gosight/processor/internal/config"
	"github.com/gosight/gosight/processor/internal/storage"
)

// Aggregator aggregates session data in Redis
type Aggregator struct {
	ch    *storage.ClickHouse
	redis *redis.Client
}

// NewAggregator creates a new session aggregator
func NewAggregator(ch *storage.ClickHouse, redisCfg config.RedisConfig) *Aggregator {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})

	return &Aggregator{
		ch:    ch,
		redis: rdb,
	}
}

// UpdateSession updates session aggregation in Redis
func (a *Aggregator) UpdateSession(ctx context.Context, event storage.EventRow) error {
	if a.redis == nil {
		return nil
	}

	key := "session:" + event.SessionID

	// Use Redis pipeline for efficiency
	pipe := a.redis.Pipeline()

	// Update session end time
	pipe.HSet(ctx, key, "ended_at", event.Timestamp.UnixMilli())

	// Increment event count
	pipe.HIncrBy(ctx, key, "events_count", 1)

	// Track based on event type (support both simple and proto enum names)
	switch event.EventType {
	case "page_view", "EVENT_TYPE_PAGE_VIEW":
		pipe.HIncrBy(ctx, key, "page_views", 1)
		pipe.HSetNX(ctx, key, "entry_page", event.PagePath)
		pipe.HSet(ctx, key, "exit_page", event.PagePath)

	case "click", "EVENT_TYPE_CLICK":
		pipe.HIncrBy(ctx, key, "click_count", 1)

	case "js_error", "EVENT_TYPE_JS_ERROR":
		pipe.HIncrBy(ctx, key, "errors_count", 1)
	}

	// Set session metadata (only if not exists)
	pipe.HSetNX(ctx, key, "project_id", event.ProjectID)
	pipe.HSetNX(ctx, key, "user_id", event.UserID)
	pipe.HSetNX(ctx, key, "started_at", event.Timestamp.UnixMilli())
	pipe.HSetNX(ctx, key, "browser", event.Browser)
	pipe.HSetNX(ctx, key, "os", event.OS)
	pipe.HSetNX(ctx, key, "device_type", event.DeviceType)
	pipe.HSetNX(ctx, key, "country", event.Country)
	pipe.HSetNX(ctx, key, "city", event.City)

	// Set TTL (1 hour)
	pipe.Expire(ctx, key, time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		log.Error().Err(err).Str("session_id", event.SessionID).Msg("Failed to update session in Redis")
	}
	return err
}

// FlushSession writes session data to ClickHouse
func (a *Aggregator) FlushSession(ctx context.Context, sessionID string) error {
	if a.redis == nil || a.ch == nil {
		return nil
	}

	key := "session:" + sessionID

	// Get all session data from Redis
	data, err := a.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	// Convert to SessionRow and insert to ClickHouse
	session := a.parseSessionData(sessionID, data)

	err = a.ch.UpsertSession(ctx, session)
	if err != nil {
		return err
	}

	// Delete from Redis after successful insert
	a.redis.Del(ctx, key)

	return nil
}

func (a *Aggregator) parseSessionData(sessionID string, data map[string]string) storage.SessionRow {
	session := storage.SessionRow{
		SessionID: sessionID,
	}

	if v, ok := data["project_id"]; ok {
		session.ProjectID = v
	}
	if v, ok := data["user_id"]; ok {
		session.UserID = v
	}
	if v, ok := data["started_at"]; ok {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			session.StartedAt = time.UnixMilli(ms)
		}
	}
	if v, ok := data["ended_at"]; ok {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			session.EndedAt = time.UnixMilli(ms)
		}
	}
	if !session.StartedAt.IsZero() && !session.EndedAt.IsZero() {
		session.DurationMs = uint64(session.EndedAt.Sub(session.StartedAt).Milliseconds())
	}
	if v, ok := data["browser"]; ok {
		session.Browser = v
	}
	if v, ok := data["os"]; ok {
		session.OS = v
	}
	if v, ok := data["device_type"]; ok {
		session.DeviceType = v
	}
	if v, ok := data["country"]; ok {
		session.Country = v
	}
	if v, ok := data["city"]; ok {
		session.City = v
	}
	if v, ok := data["page_views"]; ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			session.PageViews = uint32(n)
		}
	}
	if v, ok := data["events_count"]; ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			session.EventsCount = uint32(n)
		}
	}
	if v, ok := data["errors_count"]; ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			session.ErrorsCount = uint32(n)
		}
	}
	if v, ok := data["entry_page"]; ok {
		session.EntryPage = v
	}
	if v, ok := data["exit_page"]; ok {
		session.ExitPage = v
	}

	// Determine if bounced (only 1 page view)
	if session.PageViews <= 1 {
		session.IsBounced = 1
	}

	return session
}

// FlushAllSessions flushes all pending sessions to ClickHouse
func (a *Aggregator) FlushAllSessions(ctx context.Context) error {
	if a.redis == nil {
		return nil
	}

	// Find all session keys
	keys, err := a.redis.Keys(ctx, "session:*").Result()
	if err != nil {
		return err
	}

	for _, key := range keys {
		sessionID := key[8:] // Remove "session:" prefix
		if err := a.FlushSession(ctx, sessionID); err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to flush session")
		}
	}

	return nil
}

// Close closes the aggregator
func (a *Aggregator) Close() error {
	if a.redis != nil {
		return a.redis.Close()
	}
	return nil
}
