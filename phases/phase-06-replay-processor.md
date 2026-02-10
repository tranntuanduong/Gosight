# Phase 6: Replay Processor

## Mục Tiêu

Xây dựng service xử lý và lưu trữ session replay chunks.

## Prerequisites

- Phase 4 hoàn thành
- SDK đang gửi replay chunks

## Tasks

### 6.1 Replay Processor Main

**`cmd/replay-processor/main.go`**

```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"

    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/config"
    "github.com/gosight/gosight/processor/internal/consumer"
    "github.com/gosight/gosight/processor/internal/replay"
    "github.com/gosight/gosight/processor/internal/storage"
)

func main() {
    cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load config")
    }

    // Initialize storage
    ch, _ := storage.NewClickHouse(cfg.ClickHouse)
    defer ch.Close()

    minio, _ := storage.NewMinIO(cfg.MinIO)

    // Create replay processor
    replayProcessor := replay.NewProcessor(ch, minio, cfg.Replay)

    // Create Kafka consumer
    kafkaConsumer, _ := consumer.NewKafkaConsumer(consumer.Config{
        Brokers:       cfg.Kafka.Brokers,
        Topic:         cfg.Kafka.Topics["replay"],
        ConsumerGroup: "gosight-replay-processor",
    }, replayProcessor)

    ctx, cancel := context.WithCancel(context.Background())
    go kafkaConsumer.Start(ctx)

    log.Info().Msg("Replay processor started")

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    cancel()
    kafkaConsumer.Close()
    replayProcessor.Flush()
}
```

---

### 6.2 Replay Processor

**`internal/replay/processor.go`**

```go
package replay

import (
    "bytes"
    "compress/gzip"
    "context"
    "encoding/base64"
    "encoding/json"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/storage"
)

type Processor struct {
    ch         *storage.ClickHouse
    minio      *storage.MinIO
    cfg        Config

    // Buffer chunks per session
    sessions   map[uuid.UUID]*SessionBuffer
    mu         sync.Mutex
    ticker     *time.Ticker
}

type SessionBuffer struct {
    ProjectID       string
    Chunks          []ChunkData
    LastUpdate      time.Time
}

type ChunkData struct {
    ChunkIndex      int
    TimestampStart  int64
    TimestampEnd    int64
    Data            []byte  // Compressed rrweb events
    EventCount      int
    HasFullSnapshot bool
}

func NewProcessor(ch *storage.ClickHouse, minio *storage.MinIO, cfg Config) *Processor {
    p := &Processor{
        ch:       ch,
        minio:    minio,
        cfg:      cfg,
        sessions: make(map[uuid.UUID]*SessionBuffer),
    }

    // Start flush ticker
    p.ticker = time.NewTicker(cfg.FlushInterval)
    go p.flushLoop()

    return p
}

func (p *Processor) Process(ctx context.Context, raw map[string]interface{}) error {
    chunk, err := p.parseChunk(raw)
    if err != nil {
        return err
    }

    p.mu.Lock()
    defer p.mu.Unlock()

    // Get or create session buffer
    sessionID := chunk.SessionID
    buf, exists := p.sessions[sessionID]
    if !exists {
        buf = &SessionBuffer{
            ProjectID: chunk.ProjectID,
            Chunks:    make([]ChunkData, 0),
        }
        p.sessions[sessionID] = buf
    }

    // Add chunk
    buf.Chunks = append(buf.Chunks, ChunkData{
        ChunkIndex:      chunk.ChunkIndex,
        TimestampStart:  chunk.TimestampStart,
        TimestampEnd:    chunk.TimestampEnd,
        Data:            chunk.Data,
        EventCount:      chunk.EventCount,
        HasFullSnapshot: chunk.HasFullSnapshot,
    })
    buf.LastUpdate = time.Now()

    // Check if should flush (too many chunks or has full snapshot)
    if len(buf.Chunks) >= p.cfg.MaxChunksPerSession || chunk.HasFullSnapshot {
        p.flushSession(ctx, sessionID, buf)
        delete(p.sessions, sessionID)
    }

    return nil
}

func (p *Processor) flushLoop() {
    for range p.ticker.C {
        p.flushIdleSessions()
    }
}

func (p *Processor) flushIdleSessions() {
    p.mu.Lock()
    defer p.mu.Unlock()

    ctx := context.Background()
    cutoff := time.Now().Add(-p.cfg.IdleTimeout)

    for sessionID, buf := range p.sessions {
        if buf.LastUpdate.Before(cutoff) {
            p.flushSession(ctx, sessionID, buf)
            delete(p.sessions, sessionID)
        }
    }
}

func (p *Processor) flushSession(ctx context.Context, sessionID uuid.UUID, buf *SessionBuffer) {
    if len(buf.Chunks) == 0 {
        return
    }

    // Sort chunks by index
    sortChunks(buf.Chunks)

    // Insert each chunk to ClickHouse
    for _, chunk := range buf.Chunks {
        err := p.ch.InsertReplayChunk(ctx, storage.ReplayChunkRow{
            SessionID:       sessionID,
            ProjectID:       buf.ProjectID,
            ChunkIndex:      uint16(chunk.ChunkIndex),
            TimestampStart:  time.UnixMilli(chunk.TimestampStart),
            TimestampEnd:    time.UnixMilli(chunk.TimestampEnd),
            Data:            string(chunk.Data),
            DataSize:        uint32(len(chunk.Data)),
            EventCount:      uint16(chunk.EventCount),
            HasFullSnapshot: chunk.HasFullSnapshot,
        })

        if err != nil {
            log.Error().Err(err).
                Str("session_id", sessionID.String()).
                Int("chunk_index", chunk.ChunkIndex).
                Msg("Failed to insert replay chunk")
        }
    }

    log.Info().
        Str("session_id", sessionID.String()).
        Int("chunks", len(buf.Chunks)).
        Msg("Flushed replay session")
}

func (p *Processor) Flush() {
    p.mu.Lock()
    defer p.mu.Unlock()

    ctx := context.Background()
    for sessionID, buf := range p.sessions {
        p.flushSession(ctx, sessionID, buf)
    }
    p.sessions = make(map[uuid.UUID]*SessionBuffer)
}

type RawChunk struct {
    SessionID       uuid.UUID
    ProjectID       string
    ChunkIndex      int
    TimestampStart  int64
    TimestampEnd    int64
    Data            []byte
    EventCount      int
    HasFullSnapshot bool
}

func (p *Processor) parseChunk(raw map[string]interface{}) (*RawChunk, error) {
    chunk := &RawChunk{}

    if sid, ok := raw["session_id"].(string); ok {
        chunk.SessionID, _ = uuid.Parse(sid)
    }

    chunk.ProjectID, _ = raw["project_id"].(string)
    chunk.ChunkIndex = int(getFloat64(raw, "chunk_index"))
    chunk.TimestampStart = int64(getFloat64(raw, "timestamp_start"))
    chunk.TimestampEnd = int64(getFloat64(raw, "timestamp_end"))
    chunk.HasFullSnapshot, _ = raw["has_full_snapshot"].(bool)

    // Decode and re-compress data
    if dataStr, ok := raw["data"].(string); ok {
        // Data from SDK is base64 encoded gzip
        decoded, err := base64.StdEncoding.DecodeString(dataStr)
        if err != nil {
            return nil, err
        }

        // Re-compress with higher level if needed
        chunk.Data = p.recompress(decoded)
    }

    // Count events
    if events, ok := raw["events"].([]interface{}); ok {
        chunk.EventCount = len(events)
    }

    return chunk, nil
}

func (p *Processor) recompress(data []byte) []byte {
    // Decompress
    reader, err := gzip.NewReader(bytes.NewReader(data))
    if err != nil {
        return data // Return original if not gzipped
    }

    var decompressed bytes.Buffer
    decompressed.ReadFrom(reader)
    reader.Close()

    // Recompress with higher level
    var compressed bytes.Buffer
    writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
    writer.Write(decompressed.Bytes())
    writer.Close()

    return compressed.Bytes()
}
```

---

### 6.3 Cold Storage Migration

**`internal/replay/migrator.go`**

```go
package replay

import (
    "bytes"
    "compress/gzip"
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/minio/minio-go/v7"
    "github.com/rs/zerolog/log"

    "github.com/gosight/gosight/processor/internal/storage"
)

type ColdStorageMigrator struct {
    ch     *storage.ClickHouse
    minio  *minio.Client
    bucket string
    cfg    MigratorConfig
}

type MigratorConfig struct {
    HotRetentionDays  int    // Keep in ClickHouse
    ColdRetentionDays int    // Keep in MinIO
    BatchSize         int
    Interval          time.Duration
}

func NewColdStorageMigrator(ch *storage.ClickHouse, minio *minio.Client, cfg MigratorConfig) *ColdStorageMigrator {
    return &ColdStorageMigrator{
        ch:     ch,
        minio:  minio,
        bucket: "gosight-replays",
        cfg:    cfg,
    }
}

func (m *ColdStorageMigrator) Start(ctx context.Context) {
    ticker := time.NewTicker(m.cfg.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.migrateOldReplays(ctx)
        }
    }
}

func (m *ColdStorageMigrator) migrateOldReplays(ctx context.Context) {
    log.Info().Msg("Starting cold storage migration")

    // Find sessions older than retention period
    cutoffDate := time.Now().AddDate(0, 0, -m.cfg.HotRetentionDays)

    sessions, err := m.ch.GetOldReplaySessions(ctx, cutoffDate, m.cfg.BatchSize)
    if err != nil {
        log.Error().Err(err).Msg("Failed to get old replay sessions")
        return
    }

    for _, session := range sessions {
        if err := m.migrateSession(ctx, session); err != nil {
            log.Error().Err(err).
                Str("session_id", session.SessionID.String()).
                Msg("Failed to migrate session")
            continue
        }
    }

    log.Info().Int("count", len(sessions)).Msg("Migration completed")
}

func (m *ColdStorageMigrator) migrateSession(ctx context.Context, session storage.SessionInfo) error {
    // Get all chunks for session
    chunks, err := m.ch.GetReplayChunks(ctx, session.SessionID)
    if err != nil {
        return err
    }

    if len(chunks) == 0 {
        return nil
    }

    // Combine all chunks
    var allEvents []byte
    for _, chunk := range chunks {
        // Decompress chunk data
        reader, err := gzip.NewReader(bytes.NewReader([]byte(chunk.Data)))
        if err != nil {
            continue
        }
        var buf bytes.Buffer
        buf.ReadFrom(reader)
        reader.Close()

        allEvents = append(allEvents, buf.Bytes()...)
    }

    // Compress combined data
    var compressed bytes.Buffer
    writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
    writer.Write(allEvents)
    writer.Close()

    // Upload to MinIO
    objectName := fmt.Sprintf("%s/%s/%s.gz",
        session.ProjectID,
        session.StartedAt.Format("2006/01"),
        session.SessionID.String(),
    )

    _, err = m.minio.PutObject(ctx, m.bucket, objectName,
        bytes.NewReader(compressed.Bytes()),
        int64(compressed.Len()),
        minio.PutObjectOptions{
            ContentType:     "application/gzip",
            ContentEncoding: "gzip",
        },
    )
    if err != nil {
        return err
    }

    // Delete from ClickHouse
    err = m.ch.DeleteReplayChunks(ctx, session.SessionID)
    if err != nil {
        log.Warn().Err(err).
            Str("session_id", session.SessionID.String()).
            Msg("Failed to delete chunks from ClickHouse")
    }

    return nil
}
```

---

### 6.4 ClickHouse Storage

**`internal/storage/clickhouse_replay.go`**

```go
package storage

import (
    "context"
    "time"

    "github.com/google/uuid"
)

type ReplayChunkRow struct {
    SessionID       uuid.UUID
    ProjectID       string
    ChunkIndex      uint16
    TimestampStart  time.Time
    TimestampEnd    time.Time
    Data            string
    DataSize        uint32
    CompressedSize  uint32
    EventCount      uint16
    HasFullSnapshot bool
}

func (c *ClickHouse) InsertReplayChunk(ctx context.Context, chunk ReplayChunkRow) error {
    return c.conn.Exec(ctx, `
        INSERT INTO replay_chunks (
            session_id, project_id, chunk_index,
            timestamp_start, timestamp_end,
            data, data_size, event_count, has_full_snapshot
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `,
        chunk.SessionID, chunk.ProjectID, chunk.ChunkIndex,
        chunk.TimestampStart, chunk.TimestampEnd,
        chunk.Data, chunk.DataSize, chunk.EventCount,
        boolToUint8(chunk.HasFullSnapshot),
    )
}

func (c *ClickHouse) GetReplayChunks(ctx context.Context, sessionID uuid.UUID) ([]ReplayChunkRow, error) {
    rows, err := c.conn.Query(ctx, `
        SELECT
            session_id, project_id, chunk_index,
            timestamp_start, timestamp_end,
            data, data_size, event_count, has_full_snapshot
        FROM replay_chunks
        WHERE session_id = ?
        ORDER BY chunk_index
    `, sessionID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var chunks []ReplayChunkRow
    for rows.Next() {
        var chunk ReplayChunkRow
        var hasFullSnapshot uint8
        err := rows.Scan(
            &chunk.SessionID, &chunk.ProjectID, &chunk.ChunkIndex,
            &chunk.TimestampStart, &chunk.TimestampEnd,
            &chunk.Data, &chunk.DataSize, &chunk.EventCount, &hasFullSnapshot,
        )
        if err != nil {
            return nil, err
        }
        chunk.HasFullSnapshot = hasFullSnapshot == 1
        chunks = append(chunks, chunk)
    }

    return chunks, nil
}

type SessionInfo struct {
    SessionID uuid.UUID
    ProjectID string
    StartedAt time.Time
}

func (c *ClickHouse) GetOldReplaySessions(ctx context.Context, before time.Time, limit int) ([]SessionInfo, error) {
    rows, err := c.conn.Query(ctx, `
        SELECT DISTINCT session_id, project_id, min(timestamp_start) as started_at
        FROM replay_chunks
        WHERE chunk_date < ?
        GROUP BY session_id, project_id
        LIMIT ?
    `, before, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var sessions []SessionInfo
    for rows.Next() {
        var s SessionInfo
        rows.Scan(&s.SessionID, &s.ProjectID, &s.StartedAt)
        sessions = append(sessions, s)
    }

    return sessions, nil
}

func (c *ClickHouse) DeleteReplayChunks(ctx context.Context, sessionID uuid.UUID) error {
    return c.conn.Exec(ctx, `
        ALTER TABLE replay_chunks DELETE WHERE session_id = ?
    `, sessionID)
}
```

---

### 6.5 Configuration

**`config/replay.yaml`**

```yaml
replay:
  enabled: true

  # Processing
  max_chunks_per_session: 100
  idle_timeout: 30s
  flush_interval: 10s

  # Compression
  compression: gzip
  compression_level: 9

  # Cold storage
  migration:
    enabled: true
    hot_retention_days: 30
    cold_retention_days: 90
    batch_size: 100
    interval: 1h

minio:
  endpoint: minio:9000
  access_key: ${MINIO_ROOT_USER}
  secret_key: ${MINIO_ROOT_PASSWORD}
  bucket: gosight-replays
  use_ssl: false
```

---

## Checklist

- [ ] Replay processor main
- [ ] Chunk buffering per session
- [ ] Compression/recompression
- [ ] ClickHouse storage
- [ ] Cold storage migration (MinIO)
- [ ] Cleanup old data
- [ ] Unit tests

## Kết Quả

Sau phase này:
- Replay chunks được xử lý và lưu trữ
- Hot storage trong ClickHouse (30 ngày)
- Cold storage trong MinIO (90 ngày)
- Tự động cleanup data cũ
