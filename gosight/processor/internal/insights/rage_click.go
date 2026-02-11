package insights

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/gosight/gosight/processor/internal/config"
)

// RageClickDetector detects rapid clicks in a small area indicating user frustration
type RageClickDetector struct {
	redis        *redis.Client
	minClicks    int
	timeWindowMs int64
	radiusPx     int
}

// ClickRecord stores info about a single click
type ClickRecord struct {
	X       int
	Y       int
	EventID string
}

// NewRageClickDetector creates a new rage click detector
func NewRageClickDetector(rdb *redis.Client, cfg config.RageClickConfig) *RageClickDetector {
	return &RageClickDetector{
		redis:        rdb,
		minClicks:    cfg.MinClicks,
		timeWindowMs: cfg.TimeWindowMs,
		radiusPx:     cfg.RadiusPx,
	}
}

// ProcessClick processes a click event and detects rage clicks
func (d *RageClickDetector) ProcessClick(event *Event) *Insight {
	if d.redis == nil {
		return nil
	}

	ctx := context.Background()

	// Get click coordinates
	x := event.ClickX
	y := event.ClickY
	if x == 0 && y == 0 {
		return nil
	}

	// Grid cell for spatial grouping
	gridX := x / d.radiusPx
	gridY := y / d.radiusPx

	key := fmt.Sprintf("clicks:%s:%d:%d", event.SessionID, gridX, gridY)

	// Add click to Redis sorted set (score = timestamp)
	d.redis.ZAdd(ctx, key, redis.Z{
		Score:  float64(event.Timestamp),
		Member: fmt.Sprintf("%d:%d:%s", x, y, event.EventID),
	})

	// Set expiry
	d.redis.Expire(ctx, key, time.Duration(d.timeWindowMs*2)*time.Millisecond)

	// Remove old clicks outside time window
	cutoff := event.Timestamp - d.timeWindowMs
	d.redis.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", cutoff))

	// Get remaining clicks
	clicks, err := d.redis.ZRange(ctx, key, 0, -1).Result()
	if err != nil || len(clicks) < d.minClicks {
		return nil
	}

	// Parse clicks
	var records []ClickRecord
	for _, c := range clicks {
		var cx, cy int
		var eid string
		fmt.Sscanf(c, "%d:%d:%s", &cx, &cy, &eid)
		records = append(records, ClickRecord{X: cx, Y: cy, EventID: eid})
	}

	// Calculate center
	centerX, centerY := d.calculateCenter(records)

	// Verify all within radius
	if !d.allWithinRadius(records, centerX, centerY) {
		return nil
	}

	// Clear processed clicks
	d.redis.Del(ctx, key)

	// Create insight
	return &Insight{
		Type:           "rage_click",
		ProjectID:      event.ProjectID,
		SessionID:      event.SessionID,
		Timestamp:      time.Now(),
		URL:            event.URL,
		Path:           event.Path,
		X:              &centerX,
		Y:              &centerY,
		TargetSelector: event.TargetSelector,
		Details: map[string]interface{}{
			"click_count":    len(records),
			"time_window_ms": d.timeWindowMs,
			"radius_px":      d.radiusPx,
		},
		RelatedEventIDs: d.extractEventIDs(records),
	}
}

func (d *RageClickDetector) calculateCenter(clicks []ClickRecord) (int, int) {
	var sumX, sumY int
	for _, c := range clicks {
		sumX += c.X
		sumY += c.Y
	}
	return sumX / len(clicks), sumY / len(clicks)
}

func (d *RageClickDetector) allWithinRadius(clicks []ClickRecord, centerX, centerY int) bool {
	for _, c := range clicks {
		dx := c.X - centerX
		dy := c.Y - centerY
		distance := math.Sqrt(float64(dx*dx + dy*dy))
		if distance > float64(d.radiusPx) {
			return false
		}
	}
	return true
}

func (d *RageClickDetector) extractEventIDs(clicks []ClickRecord) []string {
	var ids []string
	for _, c := range clicks {
		ids = append(ids, c.EventID)
	}
	return ids
}
