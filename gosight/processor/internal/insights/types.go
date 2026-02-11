package insights

import (
	"time"
)

// Event represents a parsed event from Kafka
type Event struct {
	EventID        string
	Type           string
	ProjectID      string
	SessionID      string
	UserID         string
	Timestamp      int64
	URL            string
	Path           string
	ClickX         int
	ClickY         int
	TargetSelector string
	TargetTag      string
	TargetClasses  []string
	TargetRole     string
	TargetHref     string
	ErrorMessage   string
	ErrorType      string
	LCP            *float64
	FID            *float64
	CLS            *float64
	TTFB           *float64
	FCP            *float64
	INP            *float64
	MouseX         int
	MouseY         int
}

// Insight represents a detected UX insight
type Insight struct {
	Type            string
	ProjectID       string
	SessionID       string
	Timestamp       time.Time
	URL             string
	Path            string
	X               *int
	Y               *int
	TargetSelector  string
	Details         map[string]interface{}
	RelatedEventIDs []string
}
