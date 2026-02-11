package enricher

import (
	"net"
	"time"

	"github.com/mssola/useragent"
	"github.com/oschwald/geoip2-golang"
)

type Enricher struct {
	geoIP *geoip2.Reader
}

func NewEnricher(geoIPPath string) *Enricher {
	// Try to load GeoIP database
	var geoIP *geoip2.Reader
	if geoIPPath != "" {
		geoIP, _ = geoip2.Open(geoIPPath)
	}

	return &Enricher{
		geoIP: geoIP,
	}
}

type EnrichedEvent struct {
	// Original event fields
	EventID   string                 `json:"event_id"`
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	ProjectID string                 `json:"project_id"`
	SessionID string                 `json:"session_id"`
	UserID    string                 `json:"user_id,omitempty"`
	Page      map[string]interface{} `json:"page,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`

	// Enriched fields
	ServerTimestamp int64  `json:"server_timestamp"`
	Browser         string `json:"browser"`
	BrowserVersion  string `json:"browser_version"`
	OS              string `json:"os"`
	OSVersion       string `json:"os_version"`
	DeviceType      string `json:"device_type"`
	Country         string `json:"country"`
	City            string `json:"city"`
	ClientIP        string `json:"client_ip,omitempty"`
}

func (e *Enricher) Enrich(event map[string]interface{}, userAgentString, clientIP string) *EnrichedEvent {
	enriched := &EnrichedEvent{
		ServerTimestamp: time.Now().UnixMilli(),
	}

	// Copy original event fields
	if v, ok := event["event_id"].(string); ok {
		enriched.EventID = v
	}
	if v, ok := event["type"].(string); ok {
		enriched.Type = v
	}
	if v, ok := event["timestamp"].(float64); ok {
		enriched.Timestamp = int64(v)
	}
	if v, ok := event["project_id"].(string); ok {
		enriched.ProjectID = v
	}
	if v, ok := event["session_id"].(string); ok {
		enriched.SessionID = v
	}
	if v, ok := event["user_id"].(string); ok {
		enriched.UserID = v
	}
	if v, ok := event["page"].(map[string]interface{}); ok {
		enriched.Page = v
	} else {
		// Build page object from top-level url/path if not provided
		page := make(map[string]interface{})
		if url, ok := event["url"].(string); ok {
			page["url"] = url
		}
		if path, ok := event["path"].(string); ok {
			page["path"] = path
		}
		if title, ok := event["title"].(string); ok {
			page["title"] = title
		}
		if referrer, ok := event["referrer"].(string); ok {
			page["referrer"] = referrer
		}
		if len(page) > 0 {
			enriched.Page = page
		}
	}
	if v, ok := event["payload"].(map[string]interface{}); ok {
		enriched.Payload = v
	}

	// Parse user agent
	if userAgentString != "" {
		ua := useragent.New(userAgentString)
		enriched.Browser, enriched.BrowserVersion = ua.Browser()
		enriched.OS = ua.OS()
		enriched.DeviceType = getDeviceType(ua)
	}

	// GeoIP lookup
	if e.geoIP != nil && clientIP != "" {
		ip := net.ParseIP(clientIP)
		if ip != nil {
			record, err := e.geoIP.City(ip)
			if err == nil {
				enriched.Country = record.Country.IsoCode
				if name, ok := record.City.Names["en"]; ok {
					enriched.City = name
				}
			}
		}
	}

	enriched.ClientIP = clientIP

	return enriched
}

func getDeviceType(ua *useragent.UserAgent) string {
	if ua.Mobile() {
		return "mobile"
	}
	if ua.Bot() {
		return "bot"
	}
	return "desktop"
}

func (e *Enricher) Close() {
	if e.geoIP != nil {
		e.geoIP.Close()
	}
}
