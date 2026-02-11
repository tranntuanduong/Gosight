package insights

import (
	"container/ring"
	"sync"
	"time"

	"github.com/gosight/gosight/processor/internal/config"
)

// ErrorClickDetector detects clicks that are followed by JavaScript errors
type ErrorClickDetector struct {
	errorWindowMs int64
	recentClicks  *ring.Ring
	mu            sync.Mutex
}

// ClickWithSession stores a click event with its session ID
type ClickWithSession struct {
	SessionID string
	Event     *Event
}

// NewErrorClickDetector creates a new error click detector
func NewErrorClickDetector(cfg config.ErrorClickConfig) *ErrorClickDetector {
	return &ErrorClickDetector{
		errorWindowMs: cfg.ErrorWindowMs,
		recentClicks:  ring.New(100), // Keep last 100 clicks
	}
}

// ProcessClick records a click for potential error correlation
func (d *ErrorClickDetector) ProcessClick(event *Event) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.recentClicks.Value = ClickWithSession{
		SessionID: event.SessionID,
		Event:     event,
	}
	d.recentClicks = d.recentClicks.Next()
}

// ProcessError checks if an error was preceded by a click
func (d *ErrorClickDetector) ProcessError(errorEvent *Event) *Insight {
	d.mu.Lock()
	defer d.mu.Unlock()

	var matchingClick *ClickWithSession

	// Find most recent click in same session within error window
	d.recentClicks.Do(func(v interface{}) {
		if v == nil {
			return
		}

		click := v.(ClickWithSession)

		// Same session
		if click.SessionID != errorEvent.SessionID {
			return
		}

		// Within error window (click before error)
		timeDiff := errorEvent.Timestamp - click.Event.Timestamp
		if timeDiff > 0 && timeDiff <= d.errorWindowMs {
			if matchingClick == nil || click.Event.Timestamp > matchingClick.Event.Timestamp {
				matchingClick = &click
			}
		}
	})

	if matchingClick == nil {
		return nil
	}

	x := matchingClick.Event.ClickX
	y := matchingClick.Event.ClickY

	return &Insight{
		Type:           "error_click",
		ProjectID:      errorEvent.ProjectID,
		SessionID:      errorEvent.SessionID,
		Timestamp:      time.Now(),
		URL:            matchingClick.Event.URL,
		Path:           matchingClick.Event.Path,
		X:              &x,
		Y:              &y,
		TargetSelector: matchingClick.Event.TargetSelector,
		Details: map[string]interface{}{
			"error_message": errorEvent.ErrorMessage,
			"error_type":    errorEvent.ErrorType,
			"time_to_error": errorEvent.Timestamp - matchingClick.Event.Timestamp,
		},
		RelatedEventIDs: []string{
			matchingClick.Event.EventID,
			errorEvent.EventID,
		},
	}
}
