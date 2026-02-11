package insights

import (
	"math"
	"sync"
	"time"

	"github.com/gosight/gosight/processor/internal/config"
)

// ThrashedCursorDetector detects erratic mouse movements indicating confusion
type ThrashedCursorDetector struct {
	minDurationMs       int64
	minDirectionChanges int
	minVelocity         int
	sessionData         sync.Map // sessionID -> *CursorTrackingData
}

// CursorTrackingData tracks mouse movement data per session
type CursorTrackingData struct {
	Points           []MousePoint
	DirectionChanges int
	StartTime        int64
	LastDirection    float64
	mu               sync.Mutex
}

// MousePoint represents a mouse position at a given time
type MousePoint struct {
	X         int
	Y         int
	Timestamp int64
}

// NewThrashedCursorDetector creates a new thrashed cursor detector
func NewThrashedCursorDetector(cfg config.ThrashedCursorConfig) *ThrashedCursorDetector {
	return &ThrashedCursorDetector{
		minDurationMs:       cfg.MinDurationMs,
		minDirectionChanges: cfg.MinDirectionChanges,
		minVelocity:         cfg.MinVelocity,
	}
}

// ProcessMouseMove processes a mouse move event
func (d *ThrashedCursorDetector) ProcessMouseMove(event *Event) *Insight {
	// Get or create session tracking data
	dataI, _ := d.sessionData.LoadOrStore(event.SessionID, &CursorTrackingData{
		Points:    make([]MousePoint, 0, 100),
		StartTime: event.Timestamp,
	})
	data := dataI.(*CursorTrackingData)

	data.mu.Lock()
	defer data.mu.Unlock()

	// Add new point
	point := MousePoint{
		X:         event.MouseX,
		Y:         event.MouseY,
		Timestamp: event.Timestamp,
	}

	// Calculate direction change
	if len(data.Points) > 0 {
		lastPoint := data.Points[len(data.Points)-1]
		dx := float64(point.X - lastPoint.X)
		dy := float64(point.Y - lastPoint.Y)

		if dx != 0 || dy != 0 {
			direction := math.Atan2(dy, dx)

			// Check for direction change (more than 90 degrees)
			if data.LastDirection != 0 {
				angleDiff := math.Abs(direction - data.LastDirection)
				if angleDiff > math.Pi {
					angleDiff = 2*math.Pi - angleDiff
				}
				if angleDiff > math.Pi/2 { // 90 degrees
					data.DirectionChanges++
				}
			}
			data.LastDirection = direction
		}
	}

	data.Points = append(data.Points, point)

	// Clean old points (keep last 2 seconds)
	cutoff := event.Timestamp - d.minDurationMs
	newPoints := make([]MousePoint, 0, len(data.Points))
	for _, p := range data.Points {
		if p.Timestamp >= cutoff {
			newPoints = append(newPoints, p)
		}
	}
	data.Points = newPoints

	// Check if we have enough data for detection
	if len(data.Points) < 2 {
		return nil
	}

	duration := event.Timestamp - data.StartTime
	if duration < d.minDurationMs {
		return nil
	}

	// Check direction changes threshold
	if data.DirectionChanges < d.minDirectionChanges {
		return nil
	}

	// Calculate average velocity
	totalDistance := 0.0
	for i := 1; i < len(data.Points); i++ {
		dx := float64(data.Points[i].X - data.Points[i-1].X)
		dy := float64(data.Points[i].Y - data.Points[i-1].Y)
		totalDistance += math.Sqrt(dx*dx + dy*dy)
	}

	timeDiff := float64(data.Points[len(data.Points)-1].Timestamp-data.Points[0].Timestamp) / 1000.0 // seconds
	if timeDiff == 0 {
		return nil
	}

	velocity := totalDistance / timeDiff
	if velocity < float64(d.minVelocity) {
		return nil
	}

	// Calculate center point
	var sumX, sumY int
	for _, p := range data.Points {
		sumX += p.X
		sumY += p.Y
	}
	centerX := sumX / len(data.Points)
	centerY := sumY / len(data.Points)

	// Reset tracking data
	data.Points = data.Points[:0]
	data.DirectionChanges = 0
	data.StartTime = event.Timestamp

	return &Insight{
		Type:      "thrashed_cursor",
		ProjectID: event.ProjectID,
		SessionID: event.SessionID,
		Timestamp: time.Now(),
		URL:       event.URL,
		Path:      event.Path,
		X:         &centerX,
		Y:         &centerY,
		Details: map[string]interface{}{
			"direction_changes": data.DirectionChanges,
			"velocity_px_sec":   velocity,
			"duration_ms":       duration,
		},
		RelatedEventIDs: []string{event.EventID},
	}
}
