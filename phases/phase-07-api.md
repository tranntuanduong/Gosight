# Phase 7: API Service

## Mục Tiêu

Xây dựng REST API cho dashboard.

## Prerequisites

- Phase 4, 5, 6 hoàn thành (Data có trong ClickHouse)

## Tasks

### 7.1 Project Structure

```
api/
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── handler/
│   │   ├── auth.go
│   │   ├── projects.go
│   │   ├── sessions.go
│   │   ├── events.go
│   │   ├── insights.go
│   │   ├── heatmaps.go
│   │   ├── replay.go
│   │   └── alerts.go
│   ├── middleware/
│   │   ├── auth.go
│   │   ├── cors.go
│   │   └── ratelimit.go
│   ├── service/
│   │   ├── analytics.go
│   │   ├── sessions.go
│   │   └── replay.go
│   └── repository/
│       ├── postgres.go
│       └── clickhouse.go
└── go.mod
```

---

### 7.2 Main Entry Point

**`cmd/api/main.go`**

```go
package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/api/internal/config"
    "github.com/gosight/gosight/api/internal/handler"
    appMiddleware "github.com/gosight/gosight/api/internal/middleware"
    "github.com/gosight/gosight/api/internal/repository"
    "github.com/gosight/gosight/api/internal/service"
)

func main() {
    cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load config")
    }

    // Initialize repositories
    pgRepo, _ := repository.NewPostgres(cfg.Postgres)
    chRepo, _ := repository.NewClickHouse(cfg.ClickHouse)
    redisRepo, _ := repository.NewRedis(cfg.Redis)

    // Initialize services
    authService := service.NewAuthService(pgRepo, cfg.JWT)
    analyticsService := service.NewAnalyticsService(chRepo, redisRepo)
    sessionService := service.NewSessionService(chRepo)
    replayService := service.NewReplayService(chRepo, cfg.MinIO)

    // Initialize handlers
    authHandler := handler.NewAuthHandler(authService)
    projectHandler := handler.NewProjectHandler(pgRepo)
    sessionHandler := handler.NewSessionHandler(sessionService)
    analyticsHandler := handler.NewAnalyticsHandler(analyticsService)
    replayHandler := handler.NewReplayHandler(replayService)
    alertHandler := handler.NewAlertHandler(pgRepo)

    // Setup router
    r := chi.NewRouter()

    // Middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(appMiddleware.CORS)

    // Health check
    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("OK"))
    })

    // Auth routes (public)
    r.Route("/api/v1/auth", func(r chi.Router) {
        r.Post("/register", authHandler.Register)
        r.Post("/login", authHandler.Login)
        r.Post("/refresh", authHandler.RefreshToken)
    })

    // Protected routes
    r.Route("/api/v1", func(r chi.Router) {
        r.Use(appMiddleware.Auth(authService))

        // Projects
        r.Route("/projects", func(r chi.Router) {
            r.Get("/", projectHandler.List)
            r.Post("/", projectHandler.Create)
            r.Route("/{projectId}", func(r chi.Router) {
                r.Use(appMiddleware.ProjectAccess(pgRepo))
                r.Get("/", projectHandler.Get)
                r.Put("/", projectHandler.Update)
                r.Delete("/", projectHandler.Delete)

                // Analytics
                r.Get("/overview", analyticsHandler.Overview)
                r.Get("/sessions", sessionHandler.List)
                r.Get("/sessions/{sessionId}", sessionHandler.Get)
                r.Get("/sessions/{sessionId}/events", sessionHandler.GetEvents)
                r.Get("/sessions/{sessionId}/replay", replayHandler.GetReplay)
                r.Get("/sessions/{sessionId}/replay/chunks/{chunkIndex}", replayHandler.GetChunk)

                // Heatmaps
                r.Get("/heatmaps/clicks", analyticsHandler.ClickHeatmap)
                r.Get("/heatmaps/scroll", analyticsHandler.ScrollHeatmap)

                // Insights
                r.Get("/insights", analyticsHandler.Insights)

                // Errors
                r.Get("/errors", analyticsHandler.Errors)
                r.Get("/errors/{errorHash}", analyticsHandler.ErrorDetail)

                // Alerts
                r.Get("/alerts", alertHandler.List)
                r.Post("/alerts", alertHandler.Create)
                r.Put("/alerts/{alertId}", alertHandler.Update)
                r.Delete("/alerts/{alertId}", alertHandler.Delete)
                r.Post("/alerts/{alertId}/test", alertHandler.Test)
                r.Get("/alerts/history", alertHandler.History)

                // API Keys
                r.Get("/api-keys", projectHandler.ListAPIKeys)
                r.Post("/api-keys", projectHandler.CreateAPIKey)
                r.Delete("/api-keys/{keyId}", projectHandler.DeleteAPIKey)

                // Members
                r.Get("/members", projectHandler.ListMembers)
                r.Post("/members", projectHandler.AddMember)
                r.Delete("/members/{userId}", projectHandler.RemoveMember)
            })
        })

        // User
        r.Get("/me", authHandler.GetCurrentUser)
        r.Put("/me", authHandler.UpdateProfile)
    })

    // WebSocket for real-time updates
    r.Get("/ws", handler.WebSocketHandler)

    // Start server
    server := &http.Server{
        Addr:    ":" + cfg.Server.Port,
        Handler: r,
    }

    go func() {
        log.Info().Str("port", cfg.Server.Port).Msg("Starting API server")
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal().Err(err).Msg("Server error")
        }
    }()

    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    server.Shutdown(ctx)
}
```

---

### 7.3 Auth Handler

**`internal/handler/auth.go`**

```go
package handler

import (
    "encoding/json"
    "net/http"

    "github.com/gosight/gosight/api/internal/service"
)

type AuthHandler struct {
    authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
    return &AuthHandler{authService: authService}
}

type RegisterRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
    Name     string `json:"name"`
}

type LoginRequest struct {
    Email    string `json:"email"`
    Password string `json:"password"`
}

type AuthResponse struct {
    Success bool   `json:"success"`
    Token   string `json:"token,omitempty"`
    User    *User  `json:"user,omitempty"`
    Error   string `json:"error,omitempty"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
    var req RegisterRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid request")
        return
    }

    user, token, err := h.authService.Register(r.Context(), req.Email, req.Password, req.Name)
    if err != nil {
        respondError(w, http.StatusBadRequest, err.Error())
        return
    }

    respondJSON(w, http.StatusCreated, AuthResponse{
        Success: true,
        Token:   token,
        User:    user,
    })
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
    var req LoginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid request")
        return
    }

    user, token, err := h.authService.Login(r.Context(), req.Email, req.Password)
    if err != nil {
        respondError(w, http.StatusUnauthorized, "Invalid credentials")
        return
    }

    respondJSON(w, http.StatusOK, AuthResponse{
        Success: true,
        Token:   token,
        User:    user,
    })
}

func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
    // Extract refresh token from header or body
    // Generate new access token
}

func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
    user := r.Context().Value("user").(*service.User)
    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    user,
    })
}
```

---

### 7.4 Analytics Handler

**`internal/handler/analytics.go`**

```go
package handler

import (
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"

    "github.com/gosight/gosight/api/internal/service"
)

type AnalyticsHandler struct {
    analyticsService *service.AnalyticsService
}

func NewAnalyticsHandler(s *service.AnalyticsService) *AnalyticsHandler {
    return &AnalyticsHandler{analyticsService: s}
}

func (h *AnalyticsHandler) Overview(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectId")

    // Parse query params
    startDate := parseDate(r.URL.Query().Get("start_date"), time.Now().AddDate(0, 0, -7))
    endDate := parseDate(r.URL.Query().Get("end_date"), time.Now())

    overview, err := h.analyticsService.GetOverview(r.Context(), projectID, startDate, endDate)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    overview,
    })
}

type OverviewResponse struct {
    // Current period
    Sessions      int64   `json:"sessions"`
    PageViews     int64   `json:"pageviews"`
    UniqueUsers   int64   `json:"unique_users"`
    AvgDuration   float64 `json:"avg_duration_seconds"`
    BounceRate    float64 `json:"bounce_rate"`
    ErrorCount    int64   `json:"error_count"`

    // Comparison with previous period
    SessionsChange    float64 `json:"sessions_change"`
    PageViewsChange   float64 `json:"pageviews_change"`

    // Time series
    SessionsByDay []TimeSeriesPoint `json:"sessions_by_day"`

    // Top pages
    TopPages []PageStats `json:"top_pages"`

    // Top referrers
    TopReferrers []ReferrerStats `json:"top_referrers"`

    // Device breakdown
    Browsers []DeviceStats `json:"browsers"`
    OS       []DeviceStats `json:"os"`
    Devices  []DeviceStats `json:"devices"`

    // Geo breakdown
    Countries []GeoStats `json:"countries"`
}

func (h *AnalyticsHandler) ClickHeatmap(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectId")
    path := r.URL.Query().Get("path")
    viewportWidth := parseInt(r.URL.Query().Get("viewport_width"), 1920)

    startDate := parseDate(r.URL.Query().Get("start_date"), time.Now().AddDate(0, 0, -7))
    endDate := parseDate(r.URL.Query().Get("end_date"), time.Now())

    heatmap, err := h.analyticsService.GetClickHeatmap(r.Context(), projectID, path, viewportWidth, startDate, endDate)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    heatmap,
    })
}

type HeatmapPoint struct {
    X      int `json:"x"`
    Y      int `json:"y"`
    Value  int `json:"value"`
}

func (h *AnalyticsHandler) ScrollHeatmap(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectId")
    path := r.URL.Query().Get("path")

    startDate := parseDate(r.URL.Query().Get("start_date"), time.Now().AddDate(0, 0, -7))
    endDate := parseDate(r.URL.Query().Get("end_date"), time.Now())

    scrollData, err := h.analyticsService.GetScrollHeatmap(r.Context(), projectID, path, startDate, endDate)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    scrollData,
    })
}

type ScrollDepthData struct {
    Depth      int     `json:"depth"`       // 0, 10, 20, ..., 100
    Percentage float64 `json:"percentage"`  // % of users reaching this depth
}

func (h *AnalyticsHandler) Insights(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectId")
    insightType := r.URL.Query().Get("type") // rage_click, dead_click, error_click, etc.

    startDate := parseDate(r.URL.Query().Get("start_date"), time.Now().AddDate(0, 0, -7))
    endDate := parseDate(r.URL.Query().Get("end_date"), time.Now())
    page := parseInt(r.URL.Query().Get("page"), 1)
    limit := parseInt(r.URL.Query().Get("limit"), 20)

    insights, total, err := h.analyticsService.GetInsights(r.Context(), projectID, insightType, startDate, endDate, page, limit)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    insights,
        "total":   total,
        "page":    page,
        "limit":   limit,
    })
}

func (h *AnalyticsHandler) Errors(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectId")

    startDate := parseDate(r.URL.Query().Get("start_date"), time.Now().AddDate(0, 0, -7))
    endDate := parseDate(r.URL.Query().Get("end_date"), time.Now())

    errors, err := h.analyticsService.GetGroupedErrors(r.Context(), projectID, startDate, endDate)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    errors,
    })
}

type GroupedError struct {
    ErrorHash    string    `json:"error_hash"`
    Message      string    `json:"message"`
    ErrorType    string    `json:"error_type"`
    Count        int64     `json:"count"`
    SessionCount int64     `json:"session_count"`
    FirstSeen    time.Time `json:"first_seen"`
    LastSeen     time.Time `json:"last_seen"`
}
```

---

### 7.5 Sessions Handler

**`internal/handler/sessions.go`**

```go
package handler

import (
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"

    "github.com/gosight/gosight/api/internal/service"
)

type SessionHandler struct {
    sessionService *service.SessionService
}

func NewSessionHandler(s *service.SessionService) *SessionHandler {
    return &SessionHandler{sessionService: s}
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "projectId")

    // Parse filters
    filters := service.SessionFilters{
        StartDate:    parseDate(r.URL.Query().Get("start_date"), time.Now().AddDate(0, 0, -7)),
        EndDate:      parseDate(r.URL.Query().Get("end_date"), time.Now()),
        HasError:     parseBoolPtr(r.URL.Query().Get("has_error")),
        HasRageClick: parseBoolPtr(r.URL.Query().Get("has_rage_click")),
        Browser:      r.URL.Query().Get("browser"),
        Country:      r.URL.Query().Get("country"),
        DeviceType:   r.URL.Query().Get("device_type"),
        Path:         r.URL.Query().Get("path"),
        MinDuration:  parseInt(r.URL.Query().Get("min_duration"), 0),
        Page:         parseInt(r.URL.Query().Get("page"), 1),
        Limit:        parseInt(r.URL.Query().Get("limit"), 20),
        SortBy:       r.URL.Query().Get("sort_by"),
        SortOrder:    r.URL.Query().Get("sort_order"),
    }

    sessions, total, err := h.sessionService.List(r.Context(), projectID, filters)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    sessions,
        "total":   total,
        "page":    filters.Page,
        "limit":   filters.Limit,
    })
}

type SessionSummary struct {
    SessionID      string    `json:"session_id"`
    UserID         string    `json:"user_id,omitempty"`
    StartedAt      time.Time `json:"started_at"`
    Duration       int       `json:"duration_seconds"`
    PageCount      int       `json:"page_count"`
    EventCount     int       `json:"event_count"`
    HasError       bool      `json:"has_error"`
    HasRageClick   bool      `json:"has_rage_click"`
    EntryPath      string    `json:"entry_path"`
    ExitPath       string    `json:"exit_path"`
    Browser        string    `json:"browser"`
    OS             string    `json:"os"`
    DeviceType     string    `json:"device_type"`
    Country        string    `json:"country"`
}

func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionId")

    session, err := h.sessionService.Get(r.Context(), sessionID)
    if err != nil {
        respondError(w, http.StatusNotFound, "Session not found")
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    session,
    })
}

type SessionDetail struct {
    SessionSummary
    ClickCount     int            `json:"click_count"`
    ErrorCount     int            `json:"error_count"`
    MaxScrollDepth int            `json:"max_scroll_depth"`
    UTMSource      string         `json:"utm_source,omitempty"`
    UTMMedium      string         `json:"utm_medium,omitempty"`
    UTMCampaign    string         `json:"utm_campaign,omitempty"`
    Insights       []InsightShort `json:"insights"`
    HasReplay      bool           `json:"has_replay"`
}

func (h *SessionHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionId")

    events, err := h.sessionService.GetEvents(r.Context(), sessionID)
    if err != nil {
        respondError(w, http.StatusInternalServerError, err.Error())
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    events,
    })
}
```

---

### 7.6 Replay Handler

**`internal/handler/replay.go`**

```go
package handler

import (
    "net/http"

    "github.com/go-chi/chi/v5"

    "github.com/gosight/gosight/api/internal/service"
)

type ReplayHandler struct {
    replayService *service.ReplayService
}

func NewReplayHandler(s *service.ReplayService) *ReplayHandler {
    return &ReplayHandler{replayService: s}
}

func (h *ReplayHandler) GetReplay(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionId")

    // Get replay metadata
    meta, err := h.replayService.GetReplayMeta(r.Context(), sessionID)
    if err != nil {
        respondError(w, http.StatusNotFound, "Replay not found")
        return
    }

    respondJSON(w, http.StatusOK, map[string]interface{}{
        "success": true,
        "data":    meta,
    })
}

type ReplayMeta struct {
    SessionID   string       `json:"session_id"`
    Duration    int          `json:"duration_ms"`
    ChunkCount  int          `json:"chunk_count"`
    Chunks      []ChunkMeta  `json:"chunks"`
    Events      []EventMark  `json:"events"`       // Timeline markers
    Permission  string       `json:"permission"`   // full, masked, none
}

type ChunkMeta struct {
    Index          int   `json:"index"`
    TimestampStart int64 `json:"timestamp_start"`
    TimestampEnd   int64 `json:"timestamp_end"`
    Size           int   `json:"size"`
}

type EventMark struct {
    Timestamp int64  `json:"timestamp"`
    Type      string `json:"type"`      // click, error, rage_click, etc.
    Label     string `json:"label"`
}

func (h *ReplayHandler) GetChunk(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionId")
    chunkIndex := parseInt(chi.URLParam(r, "chunkIndex"), 0)

    // Get chunk data (compressed)
    data, err := h.replayService.GetChunk(r.Context(), sessionID, chunkIndex)
    if err != nil {
        respondError(w, http.StatusNotFound, "Chunk not found")
        return
    }

    // Return gzip compressed data
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Encoding", "gzip")
    w.Write(data)
}
```

---

### 7.7 Analytics Service

**`internal/service/analytics.go`**

```go
package service

import (
    "context"
    "time"

    "github.com/gosight/gosight/api/internal/repository"
)

type AnalyticsService struct {
    ch    *repository.ClickHouse
    redis *repository.Redis
}

func NewAnalyticsService(ch *repository.ClickHouse, redis *repository.Redis) *AnalyticsService {
    return &AnalyticsService{ch: ch, redis: redis}
}

func (s *AnalyticsService) GetOverview(ctx context.Context, projectID string, start, end time.Time) (*OverviewData, error) {
    // Try cache first
    cacheKey := fmt.Sprintf("overview:%s:%s:%s", projectID, start.Format("2006-01-02"), end.Format("2006-01-02"))
    if cached, err := s.redis.Get(ctx, cacheKey); err == nil {
        var data OverviewData
        json.Unmarshal([]byte(cached), &data)
        return &data, nil
    }

    // Query ClickHouse
    data, err := s.ch.GetOverview(ctx, projectID, start, end)
    if err != nil {
        return nil, err
    }

    // Cache for 5 minutes
    s.redis.Set(ctx, cacheKey, data, 5*time.Minute)

    return data, nil
}

func (s *AnalyticsService) GetClickHeatmap(ctx context.Context, projectID, path string, viewportWidth int, start, end time.Time) ([]HeatmapPoint, error) {
    return s.ch.GetClickHeatmap(ctx, projectID, path, viewportWidth, start, end)
}

func (s *AnalyticsService) GetScrollHeatmap(ctx context.Context, projectID, path string, start, end time.Time) ([]ScrollDepthData, error) {
    return s.ch.GetScrollHeatmap(ctx, projectID, path, start, end)
}
```

---

### 7.8 Auth Middleware

**`internal/middleware/auth.go`**

```go
package middleware

import (
    "context"
    "net/http"
    "strings"

    "github.com/gosight/gosight/api/internal/service"
)

func Auth(authService *service.AuthService) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract token from header
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Parse Bearer token
            parts := strings.Split(authHeader, " ")
            if len(parts) != 2 || parts[0] != "Bearer" {
                http.Error(w, "Invalid token format", http.StatusUnauthorized)
                return
            }

            token := parts[1]

            // Validate token
            user, err := authService.ValidateToken(r.Context(), token)
            if err != nil {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            // Add user to context
            ctx := context.WithValue(r.Context(), "user", user)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func ProjectAccess(repo *repository.Postgres) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user := r.Context().Value("user").(*service.User)
            projectID := chi.URLParam(r, "projectId")

            // Check if user has access to project
            hasAccess, role, err := repo.CheckProjectAccess(r.Context(), user.ID, projectID)
            if err != nil || !hasAccess {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }

            // Add role to context
            ctx := context.WithValue(r.Context(), "projectRole", role)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

---

## API Endpoints Summary

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | /api/v1/auth/register | Register new user |
| POST | /api/v1/auth/login | Login |
| POST | /api/v1/auth/refresh | Refresh token |
| GET | /api/v1/me | Get current user |
| GET | /api/v1/projects | List projects |
| POST | /api/v1/projects | Create project |
| GET | /api/v1/projects/:id | Get project |
| GET | /api/v1/projects/:id/overview | Get analytics overview |
| GET | /api/v1/projects/:id/sessions | List sessions |
| GET | /api/v1/projects/:id/sessions/:sid | Get session detail |
| GET | /api/v1/projects/:id/sessions/:sid/replay | Get replay metadata |
| GET | /api/v1/projects/:id/heatmaps/clicks | Get click heatmap |
| GET | /api/v1/projects/:id/heatmaps/scroll | Get scroll heatmap |
| GET | /api/v1/projects/:id/insights | Get insights |
| GET | /api/v1/projects/:id/errors | Get grouped errors |
| GET | /api/v1/projects/:id/alerts | List alert rules |
| POST | /api/v1/projects/:id/alerts | Create alert rule |

---

## Checklist

- [ ] Auth handlers (register, login, refresh)
- [ ] Project CRUD
- [ ] Analytics overview endpoint
- [ ] Sessions list với filters
- [ ] Session detail và events
- [ ] Replay endpoints
- [ ] Heatmaps endpoints
- [ ] Insights endpoint
- [ ] Errors grouping endpoint
- [ ] Alerts CRUD
- [ ] API keys management
- [ ] Auth middleware (JWT)
- [ ] CORS middleware
- [ ] Rate limiting
- [ ] WebSocket cho real-time

## Kết Quả

Sau phase này:
- Full REST API hoạt động
- Authentication với JWT
- Tất cả analytics endpoints
- WebSocket cho real-time updates
