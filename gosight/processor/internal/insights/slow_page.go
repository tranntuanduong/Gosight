package insights

import (
	"time"

	"github.com/gosight/gosight/processor/internal/config"
)

// SlowPageDetector detects pages with poor performance metrics
type SlowPageDetector struct {
	lcpThresholdMs  int64
	ttfbThresholdMs int64
}

// NewSlowPageDetector creates a new slow page detector
func NewSlowPageDetector(cfg config.SlowPageConfig) *SlowPageDetector {
	return &SlowPageDetector{
		lcpThresholdMs:  cfg.LCPThresholdMs,
		ttfbThresholdMs: cfg.TTFBThresholdMs,
	}
}

// ProcessPerformance processes web vitals events and detects slow pages
func (d *SlowPageDetector) ProcessPerformance(event *Event) *Insight {
	var reasons []string
	var slowestMetric float64

	// Check LCP (Largest Contentful Paint)
	if event.LCP != nil && *event.LCP > float64(d.lcpThresholdMs) {
		reasons = append(reasons, "lcp")
		if *event.LCP > slowestMetric {
			slowestMetric = *event.LCP
		}
	}

	// Check TTFB (Time to First Byte)
	if event.TTFB != nil && *event.TTFB > float64(d.ttfbThresholdMs) {
		reasons = append(reasons, "ttfb")
		if *event.TTFB > slowestMetric {
			slowestMetric = *event.TTFB
		}
	}

	// Check FCP (First Contentful Paint) - use LCP threshold as approximation
	if event.FCP != nil && *event.FCP > float64(d.lcpThresholdMs)*0.8 {
		reasons = append(reasons, "fcp")
		if *event.FCP > slowestMetric {
			slowestMetric = *event.FCP
		}
	}

	if len(reasons) == 0 {
		return nil
	}

	details := map[string]interface{}{
		"load_time_ms": slowestMetric,
		"reasons":      reasons,
	}

	// Add all available metrics
	if event.LCP != nil {
		details["lcp"] = *event.LCP
	}
	if event.TTFB != nil {
		details["ttfb"] = *event.TTFB
	}
	if event.FCP != nil {
		details["fcp"] = *event.FCP
	}
	if event.FID != nil {
		details["fid"] = *event.FID
	}
	if event.CLS != nil {
		details["cls"] = *event.CLS
	}
	if event.INP != nil {
		details["inp"] = *event.INP
	}

	return &Insight{
		Type:      "slow_page",
		ProjectID: event.ProjectID,
		SessionID: event.SessionID,
		Timestamp: time.Now(),
		URL:       event.URL,
		Path:      event.Path,
		Details:   details,
		RelatedEventIDs: []string{event.EventID},
	}
}
