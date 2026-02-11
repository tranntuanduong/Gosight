package insights

import (
	"sync"
	"time"

	"github.com/gosight/gosight/processor/internal/config"
)

// UTurnDetector detects when users navigate away and quickly return to a page
type UTurnDetector struct {
	maxTimeAwayMs int64
	sessionPages  sync.Map // sessionID -> *PageHistory
}

// PageHistory tracks page navigation history per session
type PageHistory struct {
	Pages []PageVisit
	mu    sync.Mutex
}

// PageVisit represents a page visit
type PageVisit struct {
	URL       string
	Path      string
	Timestamp int64
	EventID   string
}

// NewUTurnDetector creates a new U-turn detector
func NewUTurnDetector(cfg config.UTurnConfig) *UTurnDetector {
	return &UTurnDetector{
		maxTimeAwayMs: cfg.MaxTimeAwayMs,
	}
}

// ProcessPageView processes a page view event and detects U-turns
func (d *UTurnDetector) ProcessPageView(event *Event) *Insight {
	// Get or create session history
	historyI, _ := d.sessionPages.LoadOrStore(event.SessionID, &PageHistory{
		Pages: make([]PageVisit, 0, 20),
	})
	history := historyI.(*PageHistory)

	history.mu.Lock()
	defer history.mu.Unlock()

	currentVisit := PageVisit{
		URL:       event.URL,
		Path:      event.Path,
		Timestamp: event.Timestamp,
		EventID:   event.EventID,
	}

	// Need at least 2 previous pages to detect a U-turn
	// Pattern: A -> B -> A (within time window)
	if len(history.Pages) >= 2 {
		lastPage := history.Pages[len(history.Pages)-1]
		secondLastPage := history.Pages[len(history.Pages)-2]

		// Check if current page matches the second-to-last page (U-turn)
		if currentVisit.Path == secondLastPage.Path {
			// Check time away
			timeAway := currentVisit.Timestamp - lastPage.Timestamp

			if timeAway > 0 && timeAway <= d.maxTimeAwayMs {
				// This is a U-turn!
				history.Pages = append(history.Pages, currentVisit)

				// Keep history bounded
				if len(history.Pages) > 20 {
					history.Pages = history.Pages[len(history.Pages)-20:]
				}

				return &Insight{
					Type:      "u_turn",
					ProjectID: event.ProjectID,
					SessionID: event.SessionID,
					Timestamp: time.Now(),
					URL:       event.URL,
					Path:      event.Path,
					Details: map[string]interface{}{
						"original_page":   secondLastPage.Path,
						"navigated_to":    lastPage.Path,
						"time_away_ms":    timeAway,
						"returned_to":     currentVisit.Path,
					},
					RelatedEventIDs: []string{
						secondLastPage.EventID,
						lastPage.EventID,
						currentVisit.EventID,
					},
				}
			}
		}
	}

	// Add to history
	history.Pages = append(history.Pages, currentVisit)

	// Keep history bounded
	if len(history.Pages) > 20 {
		history.Pages = history.Pages[len(history.Pages)-20:]
	}

	return nil
}
