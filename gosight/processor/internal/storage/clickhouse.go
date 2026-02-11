package storage

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/gosight/gosight/processor/internal/config"
)

type ClickHouse struct {
	conn driver.Conn
}

// EventRow represents a row in the events table
type EventRow struct {
	EventID        string
	ProjectID      string
	SessionID      string
	UserID         string
	EventType      string
	Timestamp      time.Time
	PageURL        string
	PagePath       string
	PageTitle      string
	Referrer       string
	Browser        string
	BrowserVersion string
	OS             string
	OSVersion      string
	DeviceType     string
	ScreenWidth    uint16
	ScreenHeight   uint16
	ViewportWidth  uint16
	ViewportHeight uint16
	Country        string
	City           string
	Payload        string
}

// SessionRow represents a row in the sessions table
type SessionRow struct {
	SessionID    string
	ProjectID    string
	UserID       string
	StartedAt    time.Time
	EndedAt      time.Time
	DurationMs   uint64
	Browser      string
	OS           string
	DeviceType   string
	Country      string
	City         string
	PageViews    uint32
	EventsCount  uint32
	ErrorsCount  uint32
	EntryPage    string
	ExitPage     string
	HasReplay    uint8
	IsBounced    uint8
}

// WebVitalsRow represents a row in the web_vitals table
type WebVitalsRow struct {
	ProjectID  string
	SessionID  string
	PageURL    string
	PagePath   string
	Timestamp  time.Time
	LCP        *float64
	FID        *float64
	CLS        *float64
	TTFB       *float64
	FCP        *float64
	INP        *float64
	DeviceType string
	Country    string
}

// ErrorRow represents a row in the errors table
type ErrorRow struct {
	ProjectID string
	SessionID string
	Timestamp time.Time
	ErrorType string
	Message   string
	Stack     string
	Source    string
	Line      uint32
	Col       uint32
	PageURL   string
	PagePath  string
	Browser   string
	OS        string
}

// PageViewRow represents a row in the page_views table
type PageViewRow struct {
	ProjectID      string
	SessionID      string
	UserID         string
	PageURL        string
	PagePath       string
	PageTitle      string
	Referrer       string
	Timestamp      time.Time
	TimeOnPageMs   uint64
	MaxScrollDepth uint8
	DeviceType     string
	Country        string
}

func NewClickHouse(cfg config.ClickHouseConfig) (*ClickHouse, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		MaxOpenConns: cfg.MaxOpenConns,
		MaxIdleConns: cfg.MaxIdleConns,
	})
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := conn.Ping(context.Background()); err != nil {
		return nil, err
	}

	return &ClickHouse{conn: conn}, nil
}

func (c *ClickHouse) InsertEvents(ctx context.Context, events []EventRow) error {
	if len(events) == 0 {
		return nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO events (
			event_id, project_id, session_id, user_id, event_type, timestamp,
			page_url, page_path, page_title, referrer,
			browser, browser_version, os, os_version, device_type,
			screen_width, screen_height, viewport_width, viewport_height,
			country, city, payload
		)
	`)
	if err != nil {
		return err
	}

	for _, e := range events {
		err := batch.Append(
			e.EventID, e.ProjectID, e.SessionID, e.UserID, e.EventType, e.Timestamp,
			e.PageURL, e.PagePath, e.PageTitle, e.Referrer,
			e.Browser, e.BrowserVersion, e.OS, e.OSVersion, e.DeviceType,
			e.ScreenWidth, e.ScreenHeight, e.ViewportWidth, e.ViewportHeight,
			e.Country, e.City, e.Payload,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

func (c *ClickHouse) InsertWebVitals(ctx context.Context, vitals []WebVitalsRow) error {
	if len(vitals) == 0 {
		return nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO web_vitals (
			project_id, session_id, page_url, page_path, timestamp,
			lcp, fid, cls, ttfb, fcp, inp,
			device_type, country
		)
	`)
	if err != nil {
		return err
	}

	for _, v := range vitals {
		err := batch.Append(
			v.ProjectID, v.SessionID, v.PageURL, v.PagePath, v.Timestamp,
			v.LCP, v.FID, v.CLS, v.TTFB, v.FCP, v.INP,
			v.DeviceType, v.Country,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

func (c *ClickHouse) InsertErrors(ctx context.Context, errors []ErrorRow) error {
	if len(errors) == 0 {
		return nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO errors (
			project_id, session_id, timestamp,
			error_type, message, stack, source, line, col,
			page_url, page_path, browser, os
		)
	`)
	if err != nil {
		return err
	}

	for _, e := range errors {
		err := batch.Append(
			e.ProjectID, e.SessionID, e.Timestamp,
			e.ErrorType, e.Message, e.Stack, e.Source, e.Line, e.Col,
			e.PageURL, e.PagePath, e.Browser, e.OS,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

func (c *ClickHouse) InsertPageViews(ctx context.Context, pageViews []PageViewRow) error {
	if len(pageViews) == 0 {
		return nil
	}

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO page_views (
			project_id, session_id, user_id,
			page_url, page_path, page_title, referrer,
			timestamp, time_on_page_ms, max_scroll_depth,
			device_type, country
		)
	`)
	if err != nil {
		return err
	}

	for _, pv := range pageViews {
		err := batch.Append(
			pv.ProjectID, pv.SessionID, pv.UserID,
			pv.PageURL, pv.PagePath, pv.PageTitle, pv.Referrer,
			pv.Timestamp, pv.TimeOnPageMs, pv.MaxScrollDepth,
			pv.DeviceType, pv.Country,
		)
		if err != nil {
			return err
		}
	}

	return batch.Send()
}

func (c *ClickHouse) UpsertSession(ctx context.Context, session SessionRow) error {
	return c.conn.Exec(ctx, `
		INSERT INTO sessions (
			session_id, project_id, user_id,
			started_at, ended_at, duration_ms,
			browser, os, device_type,
			country, city,
			page_views, events_count, errors_count,
			entry_page, exit_page,
			has_replay, is_bounced
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		session.SessionID, session.ProjectID, session.UserID,
		session.StartedAt, session.EndedAt, session.DurationMs,
		session.Browser, session.OS, session.DeviceType,
		session.Country, session.City,
		session.PageViews, session.EventsCount, session.ErrorsCount,
		session.EntryPage, session.ExitPage,
		session.HasReplay, session.IsBounced,
	)
}

func (c *ClickHouse) Close() error {
	return c.conn.Close()
}
