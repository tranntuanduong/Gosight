package validation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/gosight/gosight/ingestor/internal/config"
)

type Validator struct {
	db    *pgxpool.Pool
	redis *redis.Client
	cfg   *config.Config
}

func NewValidator(cfg *config.Config) (*Validator, error) {
	// Connect to PostgreSQL
	db, err := pgxpool.New(context.Background(), cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	return &Validator{
		db:    db,
		redis: rdb,
		cfg:   cfg,
	}, nil
}

func (v *Validator) ValidateAPIKey(ctx context.Context, apiKey string) (string, error) {
	if len(apiKey) < 12 {
		return "", errors.New("invalid API key format")
	}

	// Check cache first
	cacheKey := "apikey:" + apiKey[:12]
	projectID, err := v.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		return projectID, nil
	}

	// Hash the key
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	// Query database
	var id string
	err = v.db.QueryRow(ctx, `
		SELECT project_id::text FROM api_keys
		WHERE key_hash = $1 AND is_active = true
		AND (expires_at IS NULL OR expires_at > NOW())
	`, keyHash).Scan(&id)

	if err != nil {
		return "", errors.New("invalid API key")
	}

	// Cache for 5 minutes
	v.redis.Set(ctx, cacheKey, id, 5*time.Minute)

	// Update last used
	go v.db.Exec(context.Background(), `
		UPDATE api_keys
		SET last_used_at = NOW(), request_count = request_count + 1
		WHERE key_hash = $1
	`, keyHash)

	return id, nil
}

func (v *Validator) CheckRateLimit(projectID string) bool {
	ctx := context.Background()
	key := "ratelimit:" + projectID

	// Increment counter
	count, err := v.redis.Incr(ctx, key).Result()
	if err != nil {
		return true // Allow on error
	}

	// Set expiry on first request
	if count == 1 {
		v.redis.Expire(ctx, key, time.Second)
	}

	return count <= int64(v.cfg.RateLimit.RequestsPerSecond)
}

func (v *Validator) ValidateEvent(event interface{}) error {
	// Basic validation
	// - Required fields
	// - Field types
	// - Value ranges
	return nil
}

func (v *Validator) Close() {
	if v.db != nil {
		v.db.Close()
	}
	if v.redis != nil {
		v.redis.Close()
	}
}
