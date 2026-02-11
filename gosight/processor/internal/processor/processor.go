package processor

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/gosight/gosight/processor/internal/config"
	"github.com/gosight/gosight/processor/internal/session"
	"github.com/gosight/gosight/processor/internal/storage"
	"github.com/gosight/gosight/processor/internal/transformer"
)

// EventProcessor processes events from Kafka and writes them to ClickHouse
type EventProcessor struct {
	ch         *storage.ClickHouse
	sessionAgg *session.Aggregator
	batchCfg   config.BatchConfig

	// Event buffers
	eventBuffer     []storage.EventRow
	pageViewBuffer  []storage.PageViewRow
	webVitalsBuffer []storage.WebVitalsRow
	errorBuffer     []storage.ErrorRow

	mu        sync.Mutex
	lastFlush time.Time
	ticker    *time.Ticker
	done      chan struct{}
}

// NewEventProcessor creates a new event processor
func NewEventProcessor(ch *storage.ClickHouse, sessionAgg *session.Aggregator, batchCfg config.BatchConfig) *EventProcessor {
	p := &EventProcessor{
		ch:              ch,
		sessionAgg:      sessionAgg,
		batchCfg:        batchCfg,
		eventBuffer:     make([]storage.EventRow, 0, batchCfg.Size),
		pageViewBuffer:  make([]storage.PageViewRow, 0, 100),
		webVitalsBuffer: make([]storage.WebVitalsRow, 0, 100),
		errorBuffer:     make([]storage.ErrorRow, 0, 100),
		lastFlush:       time.Now(),
		done:            make(chan struct{}),
	}

	// Start flush ticker
	p.ticker = time.NewTicker(batchCfg.FlushInterval)
	go p.flushLoop()

	return p
}

// Process processes a single event
func (p *EventProcessor) Process(ctx context.Context, event map[string]interface{}) error {
	// Transform to ClickHouse rows
	result, err := transformer.TransformEvent(event)
	if err != nil {
		return err
	}

	// Add to buffers
	p.mu.Lock()
	if result.Event != nil {
		p.eventBuffer = append(p.eventBuffer, *result.Event)
	}
	if result.PageView != nil {
		p.pageViewBuffer = append(p.pageViewBuffer, *result.PageView)
	}
	if result.WebVitals != nil {
		p.webVitalsBuffer = append(p.webVitalsBuffer, *result.WebVitals)
	}
	if result.Error != nil {
		p.errorBuffer = append(p.errorBuffer, *result.Error)
	}
	shouldFlush := len(p.eventBuffer) >= p.batchCfg.Size
	p.mu.Unlock()

	// Update session aggregation
	if result.Event != nil && p.sessionAgg != nil {
		go p.sessionAgg.UpdateSession(ctx, *result.Event)
	}

	// Flush if buffer full
	if shouldFlush {
		p.Flush()
	}

	return nil
}

func (p *EventProcessor) flushLoop() {
	for {
		select {
		case <-p.done:
			return
		case <-p.ticker.C:
			p.Flush()
		}
	}
}

// Flush writes all buffered data to ClickHouse
func (p *EventProcessor) Flush() {
	p.mu.Lock()

	// Check if there's anything to flush
	if len(p.eventBuffer) == 0 && len(p.pageViewBuffer) == 0 && len(p.webVitalsBuffer) == 0 && len(p.errorBuffer) == 0 {
		p.mu.Unlock()
		return
	}

	// Get current buffers and create new ones
	events := p.eventBuffer
	pageViews := p.pageViewBuffer
	webVitals := p.webVitalsBuffer
	errors := p.errorBuffer

	p.eventBuffer = make([]storage.EventRow, 0, p.batchCfg.Size)
	p.pageViewBuffer = make([]storage.PageViewRow, 0, 100)
	p.webVitalsBuffer = make([]storage.WebVitalsRow, 0, 100)
	p.errorBuffer = make([]storage.ErrorRow, 0, 100)
	p.lastFlush = time.Now()
	p.mu.Unlock()

	ctx := context.Background()
	start := time.Now()

	// Insert events
	if len(events) > 0 {
		if err := p.ch.InsertEvents(ctx, events); err != nil {
			log.Error().Err(err).Int("count", len(events)).Msg("Failed to insert events")
		} else {
			log.Info().
				Int("count", len(events)).
				Dur("duration", time.Since(start)).
				Msg("Flushed events to ClickHouse")
		}
	}

	// Insert page views
	if len(pageViews) > 0 {
		if err := p.ch.InsertPageViews(ctx, pageViews); err != nil {
			log.Error().Err(err).Int("count", len(pageViews)).Msg("Failed to insert page views")
		} else {
			log.Debug().Int("count", len(pageViews)).Msg("Flushed page views to ClickHouse")
		}
	}

	// Insert web vitals
	if len(webVitals) > 0 {
		if err := p.ch.InsertWebVitals(ctx, webVitals); err != nil {
			log.Error().Err(err).Int("count", len(webVitals)).Msg("Failed to insert web vitals")
		} else {
			log.Debug().Int("count", len(webVitals)).Msg("Flushed web vitals to ClickHouse")
		}
	}

	// Insert errors
	if len(errors) > 0 {
		if err := p.ch.InsertErrors(ctx, errors); err != nil {
			log.Error().Err(err).Int("count", len(errors)).Msg("Failed to insert errors")
		} else {
			log.Debug().Int("count", len(errors)).Msg("Flushed errors to ClickHouse")
		}
	}
}

// Stop stops the processor
func (p *EventProcessor) Stop() {
	p.ticker.Stop()
	close(p.done)
	p.Flush() // Final flush
}
