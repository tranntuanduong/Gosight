package insights

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gosight/gosight/processor/internal/config"
)

// DeadClickDetector detects clicks on interactive elements that produce no response
type DeadClickDetector struct {
	observationWindowMs int64
	pendingClicks       sync.Map // key -> ClickContext
	emitCallback        func(*Insight)
}

// ClickContext stores context about a pending click
type ClickContext struct {
	Event      *Event
	ExpectedTo string // "navigate", "mutate", "handle"
	Timestamp  int64
}

var expectedInteractiveTags = []string{
	"a", "button", "input", "select", "textarea",
}

var expectedInteractiveClasses = []string{
	"btn", "button", "link", "clickable", "interactive",
}

// NewDeadClickDetector creates a new dead click detector
func NewDeadClickDetector(cfg config.DeadClickConfig, emitCallback func(*Insight)) *DeadClickDetector {
	return &DeadClickDetector{
		observationWindowMs: cfg.ObservationWindowMs,
		emitCallback:        emitCallback,
	}
}

// ProcessClick processes a click event
func (d *DeadClickDetector) ProcessClick(event *Event) {
	// Check if target looks interactive
	if !d.looksInteractive(event) {
		return
	}

	// Determine expected behavior
	expected := d.determineExpectedBehavior(event)

	// Store pending click
	key := fmt.Sprintf("%s:%s", event.SessionID, event.EventID)
	d.pendingClicks.Store(key, ClickContext{
		Event:      event,
		ExpectedTo: expected,
		Timestamp:  event.Timestamp,
	})

	// Schedule check
	go func(checkKey string, clickEvent *Event) {
		time.Sleep(time.Duration(d.observationWindowMs) * time.Millisecond)
		d.checkForResponse(checkKey, clickEvent)
	}(key, event)
}

// ProcessEvent processes events that might resolve pending dead clicks
func (d *DeadClickDetector) ProcessEvent(event *Event) {
	// Check if this event resolves any pending clicks
	d.pendingClicks.Range(func(key, value interface{}) bool {
		ctx := value.(ClickContext)

		// Only check clicks from the same session
		if ctx.Event.SessionID != event.SessionID {
			return true
		}

		if d.isResponseTo(ctx, event) {
			d.pendingClicks.Delete(key)
		}

		return true
	})
}

func (d *DeadClickDetector) isResponseTo(ctx ClickContext, event *Event) bool {
	// Time check
	if event.Timestamp < ctx.Event.Timestamp {
		return false
	}

	// Within observation window
	if event.Timestamp > ctx.Event.Timestamp+d.observationWindowMs {
		return false
	}

	switch ctx.ExpectedTo {
	case "navigate":
		return event.Type == "page_view" || event.Type == "EVENT_TYPE_PAGE_VIEW"
	case "mutate":
		return event.Type == "dom_mutation" || event.Type == "EVENT_TYPE_DOM_MUTATION"
	case "handle":
		return event.Type != "mouse_move" && event.Type != "scroll" &&
			event.Type != "EVENT_TYPE_MOUSE_MOVE" && event.Type != "EVENT_TYPE_SCROLL"
	}

	return false
}

func (d *DeadClickDetector) checkForResponse(key string, event *Event) {
	value, exists := d.pendingClicks.LoadAndDelete(key)
	if !exists {
		return // Already resolved
	}

	ctx := value.(ClickContext)

	// No response - this is a dead click
	x := ctx.Event.ClickX
	y := ctx.Event.ClickY

	insight := &Insight{
		Type:           "dead_click",
		ProjectID:      ctx.Event.ProjectID,
		SessionID:      ctx.Event.SessionID,
		Timestamp:      time.Now(),
		URL:            ctx.Event.URL,
		Path:           ctx.Event.Path,
		X:              &x,
		Y:              &y,
		TargetSelector: ctx.Event.TargetSelector,
		Details: map[string]interface{}{
			"expected_behavior":      ctx.ExpectedTo,
			"observation_window_ms":  d.observationWindowMs,
			"target_tag":             ctx.Event.TargetTag,
		},
		RelatedEventIDs: []string{ctx.Event.EventID},
	}

	if d.emitCallback != nil {
		d.emitCallback(insight)
	}
}

func (d *DeadClickDetector) looksInteractive(event *Event) bool {
	// Check tag
	for _, tag := range expectedInteractiveTags {
		if event.TargetTag == tag {
			return true
		}
	}

	// Check classes
	for _, class := range event.TargetClasses {
		for _, expected := range expectedInteractiveClasses {
			if strings.Contains(strings.ToLower(class), expected) {
				return true
			}
		}
	}

	// Check role attribute
	if event.TargetRole == "button" || event.TargetRole == "link" {
		return true
	}

	return false
}

func (d *DeadClickDetector) determineExpectedBehavior(event *Event) string {
	if event.TargetTag == "a" && event.TargetHref != "" {
		return "navigate"
	}

	if event.TargetTag == "button" || event.TargetTag == "input" {
		return "handle"
	}

	return "mutate"
}
