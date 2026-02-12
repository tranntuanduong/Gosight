package insights

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"

	"github.com/gosight/gosight/processor/internal/config"
	"github.com/gosight/gosight/processor/internal/storage"
)

// Processor coordinates all insight detectors
type Processor struct {
	rageClick      *RageClickDetector
	deadClick      *DeadClickDetector
	errorClick     *ErrorClickDetector
	thrashedCursor *ThrashedCursorDetector
	uTurn          *UTurnDetector
	slowPage       *SlowPageDetector

	ch    *storage.ClickHouse
	redis *redis.Client

	// Kafka writer for alerts
	alertWriter *kafka.Writer

	// Buffer for batch inserts
	insightBuffer []storage.InsightRow
	mu            sync.Mutex
	lastFlush     time.Time
}

// NewProcessor creates a new insight processor
func NewProcessor(ch *storage.ClickHouse, rdb *redis.Client, cfg config.InsightsConfig) *Processor {
	return NewProcessorWithKafka(ch, rdb, cfg, config.KafkaConfig{})
}

// NewProcessorWithKafka creates a new insight processor with Kafka alert publishing
func NewProcessorWithKafka(ch *storage.ClickHouse, rdb *redis.Client, cfg config.InsightsConfig, kafkaCfg config.KafkaConfig) *Processor {
	p := &Processor{
		ch:            ch,
		redis:         rdb,
		insightBuffer: make([]storage.InsightRow, 0, 100),
		lastFlush:     time.Now(),
	}

	// Initialize Kafka writer for alerts if configured
	if alertsTopic, ok := kafkaCfg.Topics["alerts"]; ok && len(kafkaCfg.Brokers) > 0 {
		p.alertWriter = &kafka.Writer{
			Addr:                   kafka.TCP(kafkaCfg.Brokers...),
			Topic:                  alertsTopic,
			Balancer:               &kafka.LeastBytes{},
			BatchSize:              1,
			BatchTimeout:           time.Millisecond * 10,
			Async:                  true, // Async for alerts to not block processing
			AllowAutoTopicCreation: true,
		}
		log.Info().Str("topic", alertsTopic).Msg("Kafka alert writer initialized")
	}

	// Initialize detectors based on config
	if cfg.RageClick.Enabled {
		p.rageClick = NewRageClickDetector(rdb, cfg.RageClick)
	}
	if cfg.DeadClick.Enabled {
		p.deadClick = NewDeadClickDetector(cfg.DeadClick, p.emitInsight)
	}
	if cfg.ErrorClick.Enabled {
		p.errorClick = NewErrorClickDetector(cfg.ErrorClick)
	}
	if cfg.ThrashedCursor.Enabled {
		p.thrashedCursor = NewThrashedCursorDetector(cfg.ThrashedCursor)
	}
	if cfg.UTurn.Enabled {
		p.uTurn = NewUTurnDetector(cfg.UTurn)
	}
	if cfg.SlowPage.Enabled {
		p.slowPage = NewSlowPageDetector(cfg.SlowPage)
	}

	// Start flush ticker
	go p.flushLoop()

	return p
}

// Process processes a single event from Kafka
func (p *Processor) Process(ctx context.Context, raw map[string]interface{}) error {
	event := p.parseEvent(raw)

	var insights []*Insight

	// Handle based on event type (support both proto enum names and simple names)
	switch event.Type {
	case "click", "EVENT_TYPE_CLICK":
		// Rage click detection
		if p.rageClick != nil {
			if insight := p.rageClick.ProcessClick(event); insight != nil {
				insights = append(insights, insight)
			}
		}

		// Dead click detection
		if p.deadClick != nil {
			p.deadClick.ProcessClick(event)
		}

		// Error click tracking
		if p.errorClick != nil {
			p.errorClick.ProcessClick(event)
		}

	case "js_error", "EVENT_TYPE_JS_ERROR", "EVENT_TYPE_CUSTOM":
		// Check if custom event is actually an error
		if event.Type == "EVENT_TYPE_CUSTOM" && event.ErrorType == "" {
			break
		}

		// Error click detection
		if p.errorClick != nil {
			if insight := p.errorClick.ProcessError(event); insight != nil {
				insights = append(insights, insight)
			}
		}

	case "mouse_move", "EVENT_TYPE_MOUSE_MOVE":
		// Thrashed cursor detection
		if p.thrashedCursor != nil {
			if insight := p.thrashedCursor.ProcessMouseMove(event); insight != nil {
				insights = append(insights, insight)
			}
		}

	case "page_view", "EVENT_TYPE_PAGE_VIEW":
		// U-turn detection
		if p.uTurn != nil {
			if insight := p.uTurn.ProcessPageView(event); insight != nil {
				insights = append(insights, insight)
			}
		}

		// Resolve pending dead clicks
		if p.deadClick != nil {
			p.deadClick.ProcessEvent(event)
		}

	case "dom_mutation", "EVENT_TYPE_DOM_MUTATION":
		// Resolve pending dead clicks
		if p.deadClick != nil {
			p.deadClick.ProcessEvent(event)
		}

	case "web_vitals", "EVENT_TYPE_WEB_VITALS":
		// Slow page detection
		if p.slowPage != nil {
			if insight := p.slowPage.ProcessPerformance(event); insight != nil {
				insights = append(insights, insight)
			}
		}
	}

	// Store insights
	for _, insight := range insights {
		p.storeInsight(ctx, insight)
	}

	return nil
}

func (p *Processor) emitInsight(insight *Insight) {
	ctx := context.Background()
	p.storeInsight(ctx, insight)
}

func (p *Processor) storeInsight(ctx context.Context, insight *Insight) {
	row := storage.InsightRow{
		InsightID:       uuid.New(),
		ProjectID:       insight.ProjectID,
		SessionID:       insight.SessionID,
		InsightType:     insight.Type,
		Timestamp:       insight.Timestamp,
		URL:             insight.URL,
		Path:            insight.Path,
		X:               insight.X,
		Y:               insight.Y,
		TargetSelector:  insight.TargetSelector,
		Details:         insight.Details,
		RelatedEventIDs: insight.RelatedEventIDs,
	}

	p.mu.Lock()
	p.insightBuffer = append(p.insightBuffer, row)
	shouldFlush := len(p.insightBuffer) >= 100
	p.mu.Unlock()

	if shouldFlush {
		p.Flush()
	}

	// Publish alert to Kafka for downstream alert processing (Phase 9)
	p.publishAlert(ctx, insight, row.InsightID)

	log.Info().
		Str("type", insight.Type).
		Str("session_id", insight.SessionID).
		Str("url", insight.URL).
		Msg("Insight detected")
}

// publishAlert publishes an insight alert to Kafka for downstream alert processing
func (p *Processor) publishAlert(ctx context.Context, insight *Insight, insightID uuid.UUID) {
	if p.alertWriter == nil {
		return
	}

	alert := map[string]interface{}{
		"insight_id":   insightID.String(),
		"type":         insight.Type,
		"project_id":   insight.ProjectID,
		"session_id":   insight.SessionID,
		"timestamp":    insight.Timestamp,
		"url":          insight.URL,
		"path":         insight.Path,
		"details":      insight.Details,
		"published_at": time.Now().UnixMilli(),
	}

	if insight.X != nil {
		alert["x"] = *insight.X
	}
	if insight.Y != nil {
		alert["y"] = *insight.Y
	}
	if insight.TargetSelector != "" {
		alert["target_selector"] = insight.TargetSelector
	}

	data, err := json.Marshal(alert)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal alert")
		return
	}

	err = p.alertWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(insight.ProjectID),
		Value: data,
	})
	if err != nil {
		log.Error().Err(err).Str("type", insight.Type).Msg("Failed to publish alert to Kafka")
	} else {
		log.Debug().Str("type", insight.Type).Str("project_id", insight.ProjectID).Msg("Alert published to Kafka")
	}
}

func (p *Processor) flushLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.Flush()
	}
}

// Flush writes buffered insights to ClickHouse
func (p *Processor) Flush() {
	p.mu.Lock()
	if len(p.insightBuffer) == 0 {
		p.mu.Unlock()
		return
	}

	insights := p.insightBuffer
	p.insightBuffer = make([]storage.InsightRow, 0, 100)
	p.lastFlush = time.Now()
	p.mu.Unlock()

	ctx := context.Background()
	if err := p.ch.InsertInsights(ctx, insights); err != nil {
		log.Error().Err(err).Int("count", len(insights)).Msg("Failed to insert insights")
	} else {
		log.Info().Int("count", len(insights)).Msg("Flushed insights to ClickHouse")
	}
}

func (p *Processor) parseEvent(raw map[string]interface{}) *Event {
	event := &Event{}

	if v, ok := raw["event_id"].(string); ok {
		event.EventID = v
	}
	if v, ok := raw["type"].(string); ok {
		event.Type = v
	}
	if v, ok := raw["project_id"].(string); ok {
		event.ProjectID = v
	}
	if v, ok := raw["session_id"].(string); ok {
		event.SessionID = v
	}
	if v, ok := raw["user_id"].(string); ok {
		event.UserID = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		event.Timestamp = int64(v)
	}

	// Parse page info
	if page, ok := raw["page"].(map[string]interface{}); ok {
		if v, ok := page["url"].(string); ok {
			event.URL = v
		}
		if v, ok := page["path"].(string); ok {
			event.Path = v
		}
	}

	// Parse payload
	if payload, ok := raw["payload"].(map[string]interface{}); ok {
		// Click coordinates
		if v, ok := payload["x"].(float64); ok {
			event.ClickX = int(v)
		}
		if v, ok := payload["y"].(float64); ok {
			event.ClickY = int(v)
		}

		// Target info
		if v, ok := payload["target_selector"].(string); ok {
			event.TargetSelector = v
		}
		if v, ok := payload["target_tag"].(string); ok {
			event.TargetTag = v
		}
		if v, ok := payload["target_role"].(string); ok {
			event.TargetRole = v
		}
		if v, ok := payload["target_href"].(string); ok {
			event.TargetHref = v
		}
		if classes, ok := payload["target_classes"].([]interface{}); ok {
			for _, c := range classes {
				if s, ok := c.(string); ok {
					event.TargetClasses = append(event.TargetClasses, s)
				}
			}
		}

		// Error info
		if v, ok := payload["message"].(string); ok {
			event.ErrorMessage = v
		}
		if v, ok := payload["error_type"].(string); ok {
			event.ErrorType = v
		}
		if v, ok := payload["errorType"].(string); ok {
			event.ErrorType = v
		}

		// Web vitals (individual metric format)
		if metric, ok := payload["metric"].(string); ok {
			if value, ok := payload["value"].(float64); ok {
				switch metric {
				case "LCP":
					event.LCP = &value
				case "FID":
					event.FID = &value
				case "CLS":
					event.CLS = &value
				case "TTFB":
					event.TTFB = &value
				case "FCP":
					event.FCP = &value
				case "INP":
					event.INP = &value
				}
			}
		} else {
			// Combined format
			if v, ok := payload["lcp"].(float64); ok {
				event.LCP = &v
			}
			if v, ok := payload["ttfb"].(float64); ok {
				event.TTFB = &v
			}
			if v, ok := payload["fcp"].(float64); ok {
				event.FCP = &v
			}
			if v, ok := payload["fid"].(float64); ok {
				event.FID = &v
			}
			if v, ok := payload["cls"].(float64); ok {
				event.CLS = &v
			}
			if v, ok := payload["inp"].(float64); ok {
				event.INP = &v
			}
		}

		// Mouse move coordinates
		if v, ok := payload["mouse_x"].(float64); ok {
			event.MouseX = int(v)
		}
		if v, ok := payload["mouse_y"].(float64); ok {
			event.MouseY = int(v)
		}
	}

	return event
}

// Stop stops the processor
func (p *Processor) Stop() {
	p.Flush()
	if p.alertWriter != nil {
		if err := p.alertWriter.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close alert writer")
		}
	}
}
