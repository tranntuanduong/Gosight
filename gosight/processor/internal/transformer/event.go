package transformer

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/gosight/gosight/processor/internal/storage"
)

// EnrichedEvent represents the event structure from the ingestor
type EnrichedEvent struct {
	EventID         string                 `json:"event_id"`
	Type            string                 `json:"type"`
	Timestamp       int64                  `json:"timestamp"`
	ProjectID       string                 `json:"project_id"`
	SessionID       string                 `json:"session_id"`
	UserID          string                 `json:"user_id"`
	Page            map[string]interface{} `json:"page"`
	Payload         map[string]interface{} `json:"payload"`
	ServerTimestamp int64                  `json:"server_timestamp"`
	Browser         string                 `json:"browser"`
	BrowserVersion  string                 `json:"browser_version"`
	OS              string                 `json:"os"`
	OSVersion       string                 `json:"os_version"`
	DeviceType      string                 `json:"device_type"`
	Country         string                 `json:"country"`
	City            string                 `json:"city"`
	ClientIP        string                 `json:"client_ip"`
}

// TransformResult contains the transformed data for different tables
type TransformResult struct {
	Event     *storage.EventRow
	PageView  *storage.PageViewRow
	WebVitals *storage.WebVitalsRow
	Error     *storage.ErrorRow
}

// TransformEvent transforms a raw event from Kafka to ClickHouse row structures
func TransformEvent(raw map[string]interface{}) (*TransformResult, error) {
	result := &TransformResult{}

	// Parse the enriched event
	event := parseEnrichedEvent(raw)

	// Create base event row
	eventRow := &storage.EventRow{
		EventID:        event.EventID,
		ProjectID:      event.ProjectID,
		SessionID:      event.SessionID,
		UserID:         event.UserID,
		EventType:      event.Type,
		Timestamp:      time.UnixMilli(event.Timestamp),
		Browser:        event.Browser,
		BrowserVersion: event.BrowserVersion,
		OS:             event.OS,
		OSVersion:      event.OSVersion,
		DeviceType:     event.DeviceType,
		Country:        event.Country,
		City:           event.City,
	}

	// Parse page info
	if event.Page != nil {
		eventRow.PageURL = getString(event.Page, "url")
		eventRow.PagePath = getString(event.Page, "path")
		eventRow.PageTitle = getString(event.Page, "title")
		eventRow.Referrer = getString(event.Page, "referrer")

		// Get viewport dimensions
		eventRow.ViewportWidth = getUint16(event.Page, "viewport_width")
		eventRow.ViewportHeight = getUint16(event.Page, "viewport_height")
		eventRow.ScreenWidth = getUint16(event.Page, "screen_width")
		eventRow.ScreenHeight = getUint16(event.Page, "screen_height")
	}

	// Store payload as JSON
	if event.Payload != nil {
		payloadBytes, _ := json.Marshal(event.Payload)
		eventRow.Payload = string(payloadBytes)
	}

	result.Event = eventRow

	// Handle specific event types (support both proto enum names and simple names)
	switch event.Type {
	case "page_view", "EVENT_TYPE_PAGE_VIEW":
		result.PageView = &storage.PageViewRow{
			ProjectID:      event.ProjectID,
			SessionID:      event.SessionID,
			UserID:         event.UserID,
			PageURL:        eventRow.PageURL,
			PagePath:       eventRow.PagePath,
			PageTitle:      eventRow.PageTitle,
			Referrer:       eventRow.Referrer,
			Timestamp:      eventRow.Timestamp,
			TimeOnPageMs:   0, // Will be calculated later or from payload
			MaxScrollDepth: 0, // Will be updated from scroll events
			DeviceType:     event.DeviceType,
			Country:        event.Country,
		}

	case "web_vitals", "EVENT_TYPE_WEB_VITALS":
		if event.Payload != nil {
			webVitals := &storage.WebVitalsRow{
				ProjectID:  event.ProjectID,
				SessionID:  event.SessionID,
				PageURL:    eventRow.PageURL,
				PagePath:   eventRow.PagePath,
				Timestamp:  eventRow.Timestamp,
				DeviceType: event.DeviceType,
				Country:    event.Country,
			}

			// Handle individual metric format: {"metric":"LCP","value":732}
			if metric, ok := event.Payload["metric"].(string); ok {
				value := getFloat64Ptr(event.Payload, "value")
				switch metric {
				case "LCP":
					webVitals.LCP = value
				case "FID":
					webVitals.FID = value
				case "CLS":
					webVitals.CLS = value
				case "TTFB":
					webVitals.TTFB = value
				case "FCP":
					webVitals.FCP = value
				case "INP":
					webVitals.INP = value
				}
			} else {
				// Handle combined format: {"lcp":1200,"fid":50,...}
				webVitals.LCP = getFloat64Ptr(event.Payload, "lcp")
				webVitals.FID = getFloat64Ptr(event.Payload, "fid")
				webVitals.CLS = getFloat64Ptr(event.Payload, "cls")
				webVitals.TTFB = getFloat64Ptr(event.Payload, "ttfb")
				webVitals.FCP = getFloat64Ptr(event.Payload, "fcp")
				webVitals.INP = getFloat64Ptr(event.Payload, "inp")
			}

			result.WebVitals = webVitals
		}

	case "js_error", "EVENT_TYPE_JS_ERROR":
		if event.Payload != nil {
			result.Error = &storage.ErrorRow{
				ProjectID: event.ProjectID,
				SessionID: event.SessionID,
				Timestamp: eventRow.Timestamp,
				ErrorType: getString(event.Payload, "error_type"),
				Message:   getString(event.Payload, "message"),
				Stack:     getString(event.Payload, "stack"),
				Source:    getString(event.Payload, "source"),
				Line:      getUint32(event.Payload, "line"),
				Col:       getUint32(event.Payload, "column"),
				PageURL:   eventRow.PageURL,
				PagePath:  eventRow.PagePath,
				Browser:   event.Browser,
				OS:        event.OS,
			}
		}

	case "EVENT_TYPE_CUSTOM":
		if event.Payload != nil {
			// Check the "name" field to determine the actual event type
			// SDK sends: {"name":"web_vitals","properties":{"lcp":...}}
			name := getString(event.Payload, "name")
			properties, hasProperties := event.Payload["properties"].(map[string]interface{})

			switch name {
			case "web_vitals":
				// Custom tracked web_vitals
				if hasProperties {
					result.WebVitals = &storage.WebVitalsRow{
						ProjectID:  event.ProjectID,
						SessionID:  event.SessionID,
						PageURL:    eventRow.PageURL,
						PagePath:   eventRow.PagePath,
						Timestamp:  eventRow.Timestamp,
						LCP:        getFloat64Ptr(properties, "lcp"),
						FID:        getFloat64Ptr(properties, "fid"),
						CLS:        getFloat64Ptr(properties, "cls"),
						TTFB:       getFloat64Ptr(properties, "ttfb"),
						FCP:        getFloat64Ptr(properties, "fcp"),
						INP:        getFloat64Ptr(properties, "inp"),
						DeviceType: event.DeviceType,
						Country:    event.Country,
					}
				}

			case "js_error":
				// Custom tracked error
				if hasProperties {
					result.Error = &storage.ErrorRow{
						ProjectID: event.ProjectID,
						SessionID: event.SessionID,
						Timestamp: eventRow.Timestamp,
						ErrorType: getString(properties, "error_type"),
						Message:   getString(properties, "message"),
						Stack:     getString(properties, "stack"),
						Source:    getString(properties, "source"),
						Line:      getUint32(properties, "line"),
						Col:       getUint32(properties, "column"),
						PageURL:   eventRow.PageURL,
						PagePath:  eventRow.PagePath,
						Browser:   event.Browser,
						OS:        event.OS,
					}
				}

			default:
				// Check if this is actually a JS error (SDK auto-captures errors)
				if _, hasErrorType := event.Payload["errorType"]; hasErrorType {
					result.Error = &storage.ErrorRow{
						ProjectID: event.ProjectID,
						SessionID: event.SessionID,
						Timestamp: eventRow.Timestamp,
						ErrorType: getString(event.Payload, "errorType"),
						Message:   getString(event.Payload, "message"),
						Stack:     getString(event.Payload, "stack"),
						Source:    getString(event.Payload, "source"),
						Line:      getUint32(event.Payload, "line"),
						Col:       getUint32(event.Payload, "column"),
						PageURL:   eventRow.PageURL,
						PagePath:  eventRow.PagePath,
						Browser:   event.Browser,
						OS:        event.OS,
					}
				}
			}
		}
	}

	return result, nil
}

func parseEnrichedEvent(raw map[string]interface{}) *EnrichedEvent {
	event := &EnrichedEvent{}

	// Validate event_id is a proper UUID, generate new one if invalid
	if v, ok := raw["event_id"].(string); ok {
		if _, err := uuid.Parse(v); err == nil {
			event.EventID = v
		} else {
			event.EventID = uuid.New().String()
		}
	} else {
		event.EventID = uuid.New().String()
	}

	if v, ok := raw["type"].(string); ok {
		event.Type = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		event.Timestamp = int64(v)
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
	if v, ok := raw["page"].(map[string]interface{}); ok {
		event.Page = v
	}
	if v, ok := raw["payload"].(map[string]interface{}); ok {
		event.Payload = v
	}
	if v, ok := raw["server_timestamp"].(float64); ok {
		event.ServerTimestamp = int64(v)
	}
	if v, ok := raw["browser"].(string); ok {
		event.Browser = v
	}
	if v, ok := raw["browser_version"].(string); ok {
		event.BrowserVersion = v
	}
	if v, ok := raw["os"].(string); ok {
		event.OS = v
	}
	if v, ok := raw["os_version"].(string); ok {
		event.OSVersion = v
	}
	if v, ok := raw["device_type"].(string); ok {
		event.DeviceType = v
	}
	if v, ok := raw["country"].(string); ok {
		event.Country = v
	}
	if v, ok := raw["city"].(string); ok {
		event.City = v
	}
	if v, ok := raw["client_ip"].(string); ok {
		event.ClientIP = v
	}

	return event
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getUint16(m map[string]interface{}, key string) uint16 {
	if v, ok := m[key].(float64); ok {
		return uint16(v)
	}
	return 0
}

func getUint32(m map[string]interface{}, key string) uint32 {
	if v, ok := m[key].(float64); ok {
		return uint32(v)
	}
	return 0
}

func getFloat64Ptr(m map[string]interface{}, key string) *float64 {
	if v, ok := m[key].(float64); ok {
		return &v
	}
	return nil
}
