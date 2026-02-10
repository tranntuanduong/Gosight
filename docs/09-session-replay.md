# GoSight - Session Replay

## 1. Overview

Session Replay enables pixel-perfect playback of user sessions by recording DOM mutations rather than screen recording. This approach provides:

- **Small payload size** - Only changes are recorded, not full screenshots
- **Interactive playback** - Can inspect elements, view network requests
- **Privacy controls** - Granular masking and blocking
- **Cross-browser support** - Works on all modern browsers

### Technology

GoSight uses **rrweb** (record and replay the web) as the core recording engine.

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         SDK (Browser)                            │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │   rrweb     │───▶│   Privacy   │───▶│   Buffer    │         │
│  │  Recorder   │    │   Filter    │    │   & Batch   │         │
│  └─────────────┘    └─────────────┘    └──────┬──────┘         │
└──────────────────────────────────────────────┬──────────────────┘
                                               │
                                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Ingestor                                 │
│  ┌─────────────┐    ┌─────────────┐                             │
│  │  Decompress │───▶│   Validate  │───▶ Kafka (replay topic)   │
│  └─────────────┘    └─────────────┘                             │
└─────────────────────────────────────────────────────────────────┘
                                               │
                                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Replay Processor                            │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │  Aggregate  │───▶│  Compress   │───▶│   Store     │         │
│  │   Chunks    │    │   (gzip)    │    │ ClickHouse  │         │
│  └─────────────┘    └─────────────┘    └─────────────┘         │
└─────────────────────────────────────────────────────────────────┘
                                               │
                                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Dashboard                                │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │   Fetch     │───▶│  Decompress │───▶│   rrweb     │         │
│  │   Chunks    │    │             │    │   Player    │         │
│  └─────────────┘    └─────────────┘    └─────────────┘         │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Recording

### 3.1 rrweb Event Types

| Type | Code | Description |
|------|------|-------------|
| DomContentLoaded | 0 | DOM ready |
| Load | 1 | Page load complete |
| FullSnapshot | 2 | Complete DOM serialization |
| IncrementalSnapshot | 3 | DOM mutations |
| Meta | 4 | Viewport/URL metadata |
| Custom | 5 | Custom events |

### 3.2 Incremental Snapshot Sources

| Source | Code | Description |
|--------|------|-------------|
| Mutation | 0 | DOM changes |
| MouseMove | 1 | Mouse movement |
| MouseInteraction | 2 | Click, hover, etc. |
| Scroll | 3 | Scroll position |
| ViewportResize | 4 | Window resize |
| Input | 5 | Form input changes |
| TouchMove | 6 | Touch events |
| MediaInteraction | 7 | Video/audio |
| StyleSheetRule | 8 | CSS changes |
| CanvasMutation | 9 | Canvas updates |
| Font | 10 | Font loading |
| Log | 11 | Console logs |
| Drag | 12 | Drag events |
| StyleDeclaration | 13 | Inline styles |

### 3.3 SDK Recording Setup

```typescript
import { record, EventType } from 'rrweb';

class ReplayRecorder {
  private stopFn: (() => void) | null = null;
  private events: any[] = [];
  private config: ReplayConfig;

  start(config: ReplayConfig) {
    this.config = config;

    this.stopFn = record({
      emit: (event, isCheckout) => {
        this.handleEvent(event, isCheckout);
      },

      // Checkout = take full snapshot
      checkoutEveryNms: 60000,  // Every 60 seconds
      checkoutEveryNth: 200,     // Or every 200 events

      // Privacy
      maskAllInputs: config.privacy.maskAllInputs,
      maskTextSelector: config.privacy.maskSelectors?.join(', '),
      blockSelector: config.privacy.blockSelectors?.join(', '),

      // Performance
      sampling: {
        mousemove: true,
        mouseInteraction: true,
        scroll: 150,      // Throttle scroll events
        media: 800,       // Media sampling interval
        input: 'last',    // Only capture last input value
      },

      // Inline resources
      inlineStylesheet: true,
      inlineImages: false,  // Don't inline (size + privacy)

      // Collect fonts
      collectFonts: true,

      // Plugins
      plugins: [
        // Console recording
        getRecordConsolePlugin({
          level: ['error', 'warn'],
          lengthThreshold: 1000,
        }),
        // Network recording (optional)
        config.events.network ? getRecordNetworkPlugin() : null,
      ].filter(Boolean),
    });
  }

  private handleEvent(event: any, isCheckout: boolean) {
    // Apply additional privacy filtering
    const filteredEvent = this.applyPrivacyFilters(event);
    if (!filteredEvent) return;

    this.events.push(filteredEvent);

    // Flush on checkpoint or buffer full
    if (isCheckout || this.events.length >= 50) {
      this.flush();
    }
  }

  private applyPrivacyFilters(event: any): any | null {
    // Additional filtering beyond rrweb's built-in
    if (event.type === EventType.IncrementalSnapshot) {
      // Filter sensitive network data
      if (event.data.source === 'network') {
        event.data = this.sanitizeNetworkData(event.data);
      }
    }

    return event;
  }

  private flush() {
    if (this.events.length === 0) return;

    const chunk = {
      events: this.events,
      timestamp: Date.now(),
    };

    // Compress and send
    this.transport.sendReplayChunk(chunk);

    this.events = [];
  }

  stop() {
    if (this.stopFn) {
      this.stopFn();
      this.flush();  // Final flush
    }
  }
}
```

---

## 4. Storage

### 4.1 Chunk Strategy

Replay data is stored in chunks for efficient retrieval:

```
Session Duration: 5 minutes
Chunk Size: ~60 seconds each
Total Chunks: 5

┌─────────┬─────────┬─────────┬─────────┬─────────┐
│ Chunk 0 │ Chunk 1 │ Chunk 2 │ Chunk 3 │ Chunk 4 │
│ 0:00-   │ 1:00-   │ 2:00-   │ 3:00-   │ 4:00-   │
│ 1:00    │ 2:00    │ 3:00    │ 4:00    │ 5:00    │
│         │         │         │         │         │
│ [Full   │ [Incr   │ [Incr   │ [Full   │ [Incr   │
│ Snap]   │ only]   │ only]   │ Snap]   │ only]   │
└─────────┴─────────┴─────────┴─────────┴─────────┘
```

### 4.2 ClickHouse Schema

```sql
CREATE TABLE replay_chunks (
    session_id        UUID,
    project_id        String,
    chunk_index       UInt16,

    -- Timing
    timestamp_start   DateTime64(3),
    timestamp_end     DateTime64(3),

    -- Data
    data              String,       -- Compressed rrweb events JSON
    data_size         UInt32,       -- Uncompressed size in bytes
    compressed_size   UInt32,       -- Compressed size
    event_count       UInt16,       -- Number of rrweb events

    -- Metadata
    has_full_snapshot UInt8,        -- Contains FullSnapshot
    has_errors        UInt8,        -- Contains console errors
    has_network       UInt8,        -- Contains network data

    -- Partitioning
    chunk_date        Date DEFAULT toDate(timestamp_start)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(chunk_date)
ORDER BY (project_id, session_id, chunk_index)
TTL chunk_date + INTERVAL 90 DAY;
```

### 4.3 Compression

```go
type ReplayCompressor struct {
    level int  // gzip compression level (1-9)
}

func (c *ReplayCompressor) Compress(events []byte) ([]byte, error) {
    var buf bytes.Buffer
    writer, err := gzip.NewWriterLevel(&buf, c.level)
    if err != nil {
        return nil, err
    }

    _, err = writer.Write(events)
    if err != nil {
        return nil, err
    }

    err = writer.Close()
    if err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}

func (c *ReplayCompressor) Decompress(data []byte) ([]byte, error) {
    reader, err := gzip.NewReader(bytes.NewReader(data))
    if err != nil {
        return nil, err
    }
    defer reader.Close()

    return io.ReadAll(reader)
}
```

### 4.4 Cold Storage Migration

For replays older than 30 days, move to MinIO:

```go
type ColdStorageMigrator struct {
    clickhouse *sql.DB
    minio      *minio.Client
    bucket     string
}

func (m *ColdStorageMigrator) MigrateOldReplays(ctx context.Context) error {
    // Find sessions older than 30 days
    rows, err := m.clickhouse.Query(ctx, `
        SELECT DISTINCT session_id, project_id
        FROM replay_chunks
        WHERE chunk_date < today() - 30
        AND NOT is_archived
    `)

    for rows.Next() {
        var sessionID, projectID string
        rows.Scan(&sessionID, &projectID)

        // Get all chunks for session
        chunks := m.getSessionChunks(ctx, sessionID)

        // Combine and compress
        combined := m.combineChunks(chunks)

        // Upload to MinIO
        objectName := fmt.Sprintf("%s/%s/%s.gz",
            projectID,
            time.Now().Format("2006/01"),
            sessionID,
        )

        _, err := m.minio.PutObject(ctx, m.bucket, objectName,
            bytes.NewReader(combined),
            int64(len(combined)),
            minio.PutObjectOptions{
                ContentType:     "application/gzip",
                ContentEncoding: "gzip",
            },
        )

        if err != nil {
            continue
        }

        // Mark as archived (or delete from ClickHouse)
        m.clickhouse.Exec(ctx, `
            ALTER TABLE replay_chunks
            UPDATE is_archived = 1
            WHERE session_id = ?
        `, sessionID)
    }

    return nil
}
```

---

## 5. Playback

### 5.1 Replay Player Component

```typescript
import { Replayer } from 'rrweb';
import type { eventWithTime } from 'rrweb/typings/types';

interface ReplayPlayerProps {
  sessionId: string;
  projectId: string;
  onEventPlay?: (event: eventWithTime) => void;
}

class ReplayPlayer {
  private replayer: Replayer | null = null;
  private container: HTMLElement;
  private events: eventWithTime[] = [];
  private currentChunkIndex = 0;

  constructor(container: HTMLElement) {
    this.container = container;
  }

  async load(sessionId: string): Promise<void> {
    // Fetch replay metadata
    const meta = await api.get(`/sessions/${sessionId}/replay`);

    // Load first chunk
    await this.loadChunk(sessionId, 0);

    // Initialize replayer
    this.replayer = new Replayer(this.events, {
      root: this.container,
      skipInactive: true,
      showWarning: false,
      showDebug: false,
      blockClass: 'gosight-block',
      maskTextClass: 'gosight-mask',

      // Speed controls
      speed: 1,

      // Mobile viewport handling
      mouseTail: {
        strokeStyle: '#3b82f6',
        lineWidth: 2,
      },

      // CORS handling for resources
      insertStyleRules: [
        // Add any custom styles
      ],
    });

    // Preload next chunks
    this.preloadChunks(sessionId, meta.chunks);
  }

  private async loadChunk(sessionId: string, index: number): Promise<void> {
    const response = await api.get(
      `/sessions/${sessionId}/replay/chunks/${index}`,
      { responseType: 'arraybuffer' }
    );

    // Decompress
    const decompressed = await this.decompress(response.data);
    const chunkEvents = JSON.parse(decompressed);

    // Merge events
    this.events = [...this.events, ...chunkEvents];

    // Update replayer if already initialized
    if (this.replayer) {
      this.replayer.addEvent(chunkEvents);
    }
  }

  private async preloadChunks(sessionId: string, chunks: ChunkMeta[]): Promise<void> {
    // Preload chunks in background
    for (let i = 1; i < chunks.length; i++) {
      // Load chunk when playback reaches 80% of previous chunk
      const threshold = chunks[i - 1].timestamp_end * 0.8;

      this.replayer?.on('event-cast', (event) => {
        if (event.timestamp >= threshold && !this.loadedChunks.has(i)) {
          this.loadChunk(sessionId, i);
          this.loadedChunks.add(i);
        }
      });
    }
  }

  play(): void {
    this.replayer?.play();
  }

  pause(): void {
    this.replayer?.pause();
  }

  seek(timeOffset: number): void {
    this.replayer?.pause();
    this.replayer?.play(timeOffset);
  }

  setSpeed(speed: number): void {
    this.replayer?.setConfig({ speed });
  }

  getCurrentTime(): number {
    return this.replayer?.getCurrentTime() ?? 0;
  }

  getDuration(): number {
    return this.replayer?.getMetaData().totalTime ?? 0;
  }

  destroy(): void {
    this.replayer?.destroy();
    this.replayer = null;
    this.events = [];
  }
}
```

### 5.2 Player UI Features

```typescript
interface PlayerControls {
  // Playback
  play(): void;
  pause(): void;
  seek(time: number): void;
  setSpeed(speed: 0.5 | 1 | 2 | 4 | 8): void;

  // Navigation
  skipInactivity(): void;
  jumpToEvent(eventId: string): void;
  jumpToError(): void;
  jumpToRageClick(): void;

  // View
  toggleFullscreen(): void;
  toggleDevTools(): void;
  toggleTimeline(): void;

  // Inspection
  inspectElement(x: number, y: number): ElementInfo;
  highlightElement(selector: string): void;
}
```

### 5.3 Timeline with Events

```typescript
interface TimelineEvent {
  id: string;
  timestamp: number;
  type: 'click' | 'error' | 'rage_click' | 'page_view' | 'custom';
  label: string;
  severity?: 'info' | 'warning' | 'error';
}

function ReplayTimeline({ events, duration, currentTime, onSeek }) {
  return (
    <div className="timeline">
      <div className="timeline-track">
        {/* Progress bar */}
        <div
          className="timeline-progress"
          style={{ width: `${(currentTime / duration) * 100}%` }}
        />

        {/* Event markers */}
        {events.map((event) => (
          <div
            key={event.id}
            className={`timeline-marker timeline-marker-${event.type}`}
            style={{ left: `${(event.timestamp / duration) * 100}%` }}
            onClick={() => onSeek(event.timestamp)}
            title={event.label}
          />
        ))}
      </div>

      {/* Time display */}
      <div className="timeline-time">
        {formatTime(currentTime)} / {formatTime(duration)}
      </div>
    </div>
  );
}
```

---

## 6. Skip Inactivity

Automatically skip periods of user inactivity:

```typescript
interface InactivityConfig {
  threshold: number;      // ms of inactivity to skip
  minSkipDuration: number; // minimum skip duration
  maxPlaybackSpeed: number; // speed during inactive periods
}

class InactivitySkipper {
  private config: InactivityConfig = {
    threshold: 5000,      // 5 seconds
    minSkipDuration: 2000, // 2 seconds
    maxPlaybackSpeed: 8,
  };

  findInactiveRanges(events: eventWithTime[]): Range[] {
    const ranges: Range[] = [];
    let lastActiveTime = events[0]?.timestamp ?? 0;

    for (const event of events) {
      // Check if this is an "active" event
      if (this.isActiveEvent(event)) {
        const gap = event.timestamp - lastActiveTime;

        if (gap > this.config.threshold) {
          ranges.push({
            start: lastActiveTime,
            end: event.timestamp,
            duration: gap,
          });
        }

        lastActiveTime = event.timestamp;
      }
    }

    return ranges;
  }

  private isActiveEvent(event: eventWithTime): boolean {
    // User interaction events
    if (event.type === EventType.IncrementalSnapshot) {
      const source = event.data.source;
      return [
        IncrementalSource.MouseInteraction,
        IncrementalSource.Scroll,
        IncrementalSource.Input,
        IncrementalSource.TouchMove,
      ].includes(source);
    }

    // Page navigation
    if (event.type === EventType.Meta) {
      return true;
    }

    return false;
  }
}
```

---

## 7. DevTools Integration

### 7.1 Console Panel

```typescript
interface ConsoleEntry {
  level: 'log' | 'info' | 'warn' | 'error';
  timestamp: number;
  message: string;
  stack?: string;
}

function ConsolePanel({ entries, currentTime }) {
  // Filter entries up to current playback time
  const visibleEntries = entries.filter(e => e.timestamp <= currentTime);

  return (
    <div className="console-panel">
      {visibleEntries.map((entry, i) => (
        <div key={i} className={`console-entry console-${entry.level}`}>
          <span className="console-time">
            {formatTime(entry.timestamp)}
          </span>
          <span className="console-message">{entry.message}</span>
          {entry.stack && (
            <pre className="console-stack">{entry.stack}</pre>
          )}
        </div>
      ))}
    </div>
  );
}
```

### 7.2 Network Panel

```typescript
interface NetworkRequest {
  id: string;
  timestamp: number;
  method: string;
  url: string;
  status: number;
  duration: number;
  size: number;
  type: 'xhr' | 'fetch' | 'resource';
}

function NetworkPanel({ requests, currentTime }) {
  const visibleRequests = requests.filter(r => r.timestamp <= currentTime);

  return (
    <div className="network-panel">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Type</th>
            <th>Size</th>
            <th>Time</th>
          </tr>
        </thead>
        <tbody>
          {visibleRequests.map((req) => (
            <tr key={req.id} className={req.status >= 400 ? 'error' : ''}>
              <td>{getUrlName(req.url)}</td>
              <td>{req.status}</td>
              <td>{req.type}</td>
              <td>{formatBytes(req.size)}</td>
              <td>{req.duration}ms</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

---

## 8. Performance Optimization

### 8.1 Lazy Loading

```typescript
class LazyReplayLoader {
  private loadedChunks: Set<number> = new Set();
  private loadingChunks: Set<number> = new Set();

  async ensureChunkLoaded(index: number): Promise<void> {
    if (this.loadedChunks.has(index)) return;
    if (this.loadingChunks.has(index)) {
      // Wait for existing load
      await this.waitForChunk(index);
      return;
    }

    this.loadingChunks.add(index);

    try {
      await this.loadChunk(index);
      this.loadedChunks.add(index);
    } finally {
      this.loadingChunks.delete(index);
    }
  }

  // Preload chunks based on playback position
  preloadAround(currentChunkIndex: number, preloadCount = 2): void {
    for (let i = 1; i <= preloadCount; i++) {
      const nextIndex = currentChunkIndex + i;
      if (!this.loadedChunks.has(nextIndex)) {
        this.ensureChunkLoaded(nextIndex);
      }
    }
  }
}
```

### 8.2 Virtual DOM Optimization

```typescript
// Limit DOM size for very long sessions
const replayerConfig = {
  // Pause older mutations to reduce memory
  liveMode: false,

  // Use virtual scrolling for long sessions
  useVirtualDom: true,

  // Limit mutation queue size
  maxMutationQueue: 1000,
};
```

---

## 9. Mobile Replay

### 9.1 Viewport Handling

```typescript
function MobileReplayWrapper({ children, originalViewport }) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const container = containerRef.current;
    const containerWidth = container.clientWidth;

    // Calculate scale to fit mobile viewport in container
    const scale = containerWidth / originalViewport.width;

    // Apply transform to replay iframe
    const iframe = container.querySelector('iframe');
    if (iframe) {
      iframe.style.transform = `scale(${scale})`;
      iframe.style.transformOrigin = 'top left';
      iframe.style.width = `${originalViewport.width}px`;
      iframe.style.height = `${originalViewport.height}px`;
    }
  }, [originalViewport]);

  return (
    <div ref={containerRef} className="mobile-replay-container">
      {children}
    </div>
  );
}
```

### 9.2 Touch Event Visualization

```typescript
const touchConfig = {
  // Show touch points
  showTouchPoints: true,
  touchPointColor: '#3b82f6',
  touchPointSize: 30,

  // Show touch trails
  showTouchTrail: true,
  touchTrailColor: 'rgba(59, 130, 246, 0.3)',
};
```

---

## 10. References

- [rrweb Documentation](https://github.com/rrweb-io/rrweb)
- [SDK Specification](./04-sdk-specification.md)
- [Privacy Module](./08-privacy-module.md)
- [API Specification](./05-api-specification.md)
