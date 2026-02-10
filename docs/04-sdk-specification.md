# GoSight - SDK Specification

## 1. Overview

The GoSight SDK is a lightweight JavaScript library that automatically captures user interactions and session replays from websites and web applications.

### Key Features

| Feature | Description |
|---------|-------------|
| **Auto-tracking** | Automatic capture of clicks, scrolls, inputs, errors |
| **Session Replay** | DOM recording using rrweb |
| **Privacy Controls** | Configurable masking and blocking |
| **Offline Support** | Queue events when offline |
| **Framework Agnostic** | Works with any JS framework |
| **Minimal Footprint** | < 30KB gzipped |

---

## 2. Installation

### 2.1 Script Tag (Recommended)

```html
<script>
  (function(w,d,s,k,c){
    w.GoSight=w.GoSight||{q:[],c:c};
    w.GoSight.q.push(['init',k]);
    var f=d.getElementsByTagName(s)[0],
        j=d.createElement(s);
    j.async=1;
    j.src='https://cdn.gosight.io/v1/sdk.js';
    f.parentNode.insertBefore(j,f);
  })(window,document,'script','YOUR_PROJECT_KEY',{
    // Optional configuration
  });
</script>
```

### 2.2 NPM Package

```bash
npm install @gosight/sdk
```

```typescript
import { GoSight } from '@gosight/sdk';

GoSight.init({
  projectKey: 'YOUR_PROJECT_KEY',
  // Optional configuration
});
```

### 2.3 ES Module (CDN)

```html
<script type="module">
  import { GoSight } from 'https://cdn.gosight.io/v1/sdk.esm.js';

  GoSight.init({
    projectKey: 'YOUR_PROJECT_KEY'
  });
</script>
```

---

## 3. Configuration

### 3.1 Full Configuration Interface

```typescript
interface GoSightConfig {
  // Required
  projectKey: string;

  // Server
  endpoint?: string;          // Default: 'https://ingest.gosight.io'

  // Session
  sessionTimeout?: number;    // Default: 30 (minutes)

  // Event toggles
  events?: {
    session?: boolean;        // Default: true
    page?: boolean;           // Default: true
    mouse?: boolean;          // Default: true
    scroll?: boolean;         // Default: true
    input?: boolean;          // Default: true
    form?: boolean;           // Default: true
    error?: boolean;          // Default: true
    performance?: boolean;    // Default: true
    replay?: boolean;         // Default: true
    network?: boolean;        // Default: false
    media?: boolean;          // Default: false
    clipboard?: boolean;      // Default: false
    selection?: boolean;      // Default: false
    resize?: boolean;         // Default: true
  };

  // Sampling
  sampling?: {
    sessionSampleRate?: number;   // 0-100, Default: 100
    replaySampleRate?: number;    // 0-100, Default: 100
    mouseMoveInterval?: number;   // ms, Default: 50
    scrollThrottle?: number;      // ms, Default: 100
  };

  // Batching
  batching?: {
    maxEvents?: number;       // Default: 10
    maxWaitMs?: number;       // Default: 5000
    flushOnClose?: boolean;   // Default: true
  };

  // Privacy
  privacy?: {
    maskAllInputs?: boolean;      // Default: true
    maskInputTypes?: string[];    // Default: ['password', 'email', 'tel']
    maskSelectors?: string[];     // CSS selectors to mask
    blockSelectors?: string[];    // CSS selectors to block from replay
    blockUrls?: string[];         // URL patterns to not record
    ignoreSelectors?: string[];   // Don't track events from these elements
    anonymizeIp?: boolean;        // Default: false
    maskRegex?: RegExp[];         // Patterns to mask in text
  };

  // Hooks
  hooks?: {
    beforeSend?: (events: Event[]) => Event[] | null;
    onError?: (error: Error) => void;
    onSessionStart?: (sessionId: string) => void;
  };

  // Debug
  debug?: boolean;            // Default: false
}
```

### 3.2 Configuration Examples

#### Minimal Setup

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz'
});
```

#### Privacy-Focused Setup

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz',
  privacy: {
    maskAllInputs: true,
    blockSelectors: ['.payment-form', '[data-sensitive]'],
    blockUrls: ['/checkout/*', '/admin/*'],
    anonymizeIp: true,
    maskRegex: [
      /\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b/g,  // Credit cards
      /\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b/g  // Emails
    ]
  }
});
```

#### High-Traffic Site (Sampling)

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz',
  sampling: {
    sessionSampleRate: 10,    // Only track 10% of sessions
    replaySampleRate: 5,      // Only replay 5% of sessions
    mouseMoveInterval: 100    // Less frequent mouse tracking
  }
});
```

#### Development Setup

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz',
  endpoint: 'http://localhost:8080',
  debug: true,
  hooks: {
    beforeSend: (events) => {
      console.log('Sending events:', events);
      return events;
    }
  }
});
```

#### Analytics-Only Mode (No Replay - Giảm 99% storage)

Chỉ thu thập events cho heatmaps và analytics, không record DOM.

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz',
  events: {
    session: true,
    page: true,
    mouse: true,        // Clicks cho heatmap
    scroll: true,       // Scroll depth
    input: true,
    form: true,
    error: true,
    performance: true,
    replay: false,      // ← TẮT REPLAY
    network: false,
    media: false,
  },
  sampling: {
    mouseMoveInterval: 0,    // Không track mouse movement
    scrollThrottle: 500,
  }
});
```

**SDK bundle:** ~8KB (không load rrweb)
**Storage:** ~1-2 GB/day thay vì 100 GB/day

**Vẫn có đầy đủ:**
- ✅ Click heatmaps
- ✅ Scroll heatmaps
- ✅ Time on page
- ✅ User journeys
- ✅ Rage click detection
- ✅ Error tracking

#### Replay Optimized Mode (Giảm 90% storage)

Giữ replay nhưng chỉ record khi có vấn đề.

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz',

  // Sampling
  sampling: {
    sessionSampleRate: 100,   // Track 100% sessions
    replaySampleRate: 10,     // Chỉ replay 10% sessions
  },

  // Trigger-based recording
  replay: {
    mode: 'trigger',          // Chỉ record khi có vấn đề
    triggers: {
      onError: true,          // JS error
      onRageClick: true,      // Rage click detected
      onDeadClick: false,     // Dead click (optional)
      onCustomEvent: ['checkout_failed', 'payment_error'],
    },
    bufferSeconds: 30,        // Giữ 30s trước trigger
    recordAfterSeconds: 60,   // Record 60s sau trigger
  },

  // Reduce fidelity
  replayOptions: {
    recordMouseMove: false,   // Không track mouse movement (-35%)
    scrollThrottle: 300,      // Throttle scroll
    checkoutEveryNms: 120000, // Snapshot mỗi 2 phút
    recordCanvas: false,
    recordVideo: false,
  },

  // Duration limit
  maxReplayDuration: 5 * 60 * 1000,  // Max 5 phút
});
```

**Storage estimate:**
| Config | Storage/day |
|--------|-------------|
| Full replay | 100 GB |
| + 10% sampling | 10 GB |
| + Trigger mode | 3-5 GB |
| + Reduce fidelity | 2-3 GB |

#### Selective Page Recording

Chỉ record replay trên các trang quan trọng.

```typescript
GoSight.init({
  projectKey: 'gs_abc123xyz',
  replay: {
    // Chỉ record các trang này
    includeUrls: [
      '/checkout/*',
      '/payment/*',
      '/signup',
      '/onboarding/*'
    ],

    // Hoặc exclude mode
    excludeUrls: [
      '/blog/*',
      '/docs/*',
      '/about',
      '/terms'
    ]
  }
});
```

---

## 4. Public API

### 4.1 Core Methods

```typescript
interface GoSightAPI {
  /**
   * Initialize the SDK
   */
  init(config: GoSightConfig): void;

  /**
   * Identify a user
   */
  identify(userId: string, traits?: Record<string, any>): void;

  /**
   * Track a custom event
   */
  track(eventName: string, properties?: Record<string, any>): void;

  /**
   * Set custom attributes for all subsequent events
   */
  setCustomAttributes(attributes: Record<string, any>): void;

  /**
   * Get the current session ID
   */
  getSessionId(): string | null;

  /**
   * Get the current user ID
   */
  getUserId(): string | null;

  /**
   * Manually start a new session
   */
  startNewSession(): void;

  /**
   * Pause tracking
   */
  pause(): void;

  /**
   * Resume tracking
   */
  resume(): void;

  /**
   * Check if tracking is paused
   */
  isPaused(): boolean;

  /**
   * Manually flush queued events
   */
  flush(): Promise<void>;

  /**
   * Opt out of tracking (persisted)
   */
  optOut(): void;

  /**
   * Opt back in to tracking
   */
  optIn(): void;

  /**
   * Check opt-out status
   */
  hasOptedOut(): boolean;

  /**
   * Reset user identity
   */
  reset(): void;

  /**
   * Get SDK version
   */
  getVersion(): string;
}
```

### 4.2 Method Details

#### `identify(userId, traits?)`

Associates a user identity with the current session.

```typescript
// Basic identification
GoSight.identify('user_123');

// With traits
GoSight.identify('user_123', {
  email: 'user@example.com',
  name: 'John Doe',
  plan: 'premium',
  company: 'Acme Inc',
  createdAt: '2024-01-15'
});
```

**Behavior:**
- Persists user ID in localStorage
- Associates all future events with this user
- Sends an `identify` event with traits
- Merges with existing traits (doesn't replace)

---

#### `track(eventName, properties?)`

Sends a custom event.

```typescript
// Simple event
GoSight.track('button_clicked');

// With properties
GoSight.track('purchase_completed', {
  orderId: 'ORD-123',
  total: 99.99,
  currency: 'USD',
  items: 3
});

// E-commerce example
GoSight.track('product_added_to_cart', {
  productId: 'SKU-456',
  productName: 'Blue T-Shirt',
  price: 29.99,
  quantity: 2,
  category: 'Apparel'
});
```

**Constraints:**
- Event name: 1-100 characters, alphanumeric + underscore
- Properties: Max 50 keys, values must be primitives

---

#### `setCustomAttributes(attributes)`

Sets attributes that are included with every event.

```typescript
GoSight.setCustomAttributes({
  appVersion: '2.1.0',
  environment: 'production',
  abTestGroup: 'variant_b'
});
```

**Use cases:**
- A/B test variants
- Feature flags
- App version
- User segments

---

### 4.3 Data Attributes

For fine-grained control without code changes:

#### Element Naming

```html
<!-- Custom name for reports -->
<button data-gosight-name="hero-cta">Get Started</button>

<!-- Or use existing test IDs -->
<button data-testid="submit-btn">Submit</button>
<button data-analytics="checkout-btn">Checkout</button>
```

#### Ignore Elements

```html
<!-- Don't track this element -->
<button data-gosight-ignore>Admin Only</button>

<!-- Don't track anything inside -->
<div data-gosight-ignore>
  <button>Won't be tracked</button>
  <input type="text" />
</div>
```

#### Mask Content

```html
<!-- Mask in replay (shows as ****) -->
<div data-gosight-mask>
  SSN: 123-45-6789
</div>

<!-- Block from replay (shows placeholder) -->
<div data-gosight-block>
  <iframe src="payment-form.html"></iframe>
</div>
```

#### Custom Event Data

```html
<!-- Attach data to click events -->
<button
  data-gosight-name="add-to-cart"
  data-gosight-product-id="SKU-123"
  data-gosight-product-price="29.99"
  data-gosight-product-category="electronics"
>
  Add to Cart
</button>
```

Resulting event:
```json
{
  "event_type": "click",
  "target": {
    "name": "add-to-cart",
    "custom": {
      "product_id": "SKU-123",
      "product_price": "29.99",
      "product_category": "electronics"
    }
  }
}
```

---

## 5. Internal Architecture

### 5.1 Module Structure

```
@gosight/sdk/
├── src/
│   ├── index.ts              # Public API
│   ├── core/
│   │   ├── config.ts         # Configuration management
│   │   ├── session.ts        # Session management
│   │   ├── identity.ts       # User identity
│   │   └── storage.ts        # localStorage/IndexedDB
│   ├── capture/
│   │   ├── events.ts         # Event listeners setup
│   │   ├── clicks.ts         # Click handling
│   │   ├── scroll.ts         # Scroll handling
│   │   ├── input.ts          # Input handling
│   │   ├── errors.ts         # Error capturing
│   │   ├── performance.ts    # Web vitals
│   │   └── network.ts        # XHR/Fetch interception
│   ├── replay/
│   │   ├── recorder.ts       # rrweb wrapper
│   │   └── privacy.ts        # Masking/blocking
│   ├── transport/
│   │   ├── buffer.ts         # Event batching
│   │   ├── sender.ts         # Network requests
│   │   └── offline.ts        # Offline queue
│   ├── utils/
│   │   ├── dom.ts            # DOM utilities
│   │   ├── selector.ts       # CSS selector generation
│   │   ├── throttle.ts       # Throttle/debounce
│   │   └── uuid.ts           # UUID generation
│   └── types/
│       └── index.ts          # TypeScript definitions
├── package.json
└── tsconfig.json
```

### 5.2 Initialization Flow

```
┌─────────────────────────────────────────────────────────────┐
│                     GoSight.init()                          │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  1. Validate config                                         │
│  2. Check opt-out status                                    │
│  3. Apply sampling decision                                 │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  4. Initialize/restore session                              │
│     - Check localStorage for existing session               │
│     - Create new if expired (> 30 min idle)                │
│     - Generate session_id (UUID v4)                         │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  5. Setup event listeners (Event Delegation)                │
│     - document.addEventListener('click', ...)               │
│     - document.addEventListener('input', ...)               │
│     - window.addEventListener('error', ...)                 │
│     - etc.                                                  │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  6. Start rrweb recorder (if replay enabled)                │
│     - Take initial DOM snapshot                             │
│     - Observe mutations                                     │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  7. Send session_start event                                │
│  8. Start batch timer                                       │
│  9. Setup beforeunload handler                              │
└─────────────────────────────────────────────────────────────┘
```

### 5.3 Event Capture Flow

```
┌──────────────────┐
│   User Action    │  (click, scroll, input, etc.)
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   Event Listener                             │
│  (document-level via event delegation)                       │
└────────┬─────────────────────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   Should Capture?                            │
│  - Check if element has data-gosight-ignore                 │
│  - Check if URL is in blockUrls                             │
│  - Check if tracking is paused                              │
│  - Check if user has opted out                              │
└────────┬─────────────────────────────────────────────────────┘
         │ Yes
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   Extract Event Data                         │
│  - Get target element info                                  │
│  - Generate unique selector                                 │
│  - Get element name (data-gosight-name, id, text, etc.)    │
│  - Apply privacy rules (masking)                            │
│  - Add metadata (url, timestamp, device)                    │
└────────┬─────────────────────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   beforeSend Hook                            │
│  - Allow modification/filtering                             │
│  - Return null to drop event                                │
└────────┬─────────────────────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   Add to Buffer                              │
│  - Push to event queue                                      │
│  - Check flush conditions                                   │
└────────┬─────────────────────────────────────────────────────┘
         │
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   Flush Conditions                           │
│  - Buffer size >= maxEvents (10)                            │
│  - Time elapsed >= maxWaitMs (5000ms)                       │
│  - Page unload (beforeunload)                               │
│  - Visibility change (hidden)                               │
│  - Manual flush() call                                      │
│  - Priority event (error, session_end)                      │
└────────┬─────────────────────────────────────────────────────┘
         │ Condition met
         ▼
┌──────────────────────────────────────────────────────────────┐
│                   Send to Server                             │
│  - Serialize to Protobuf                                    │
│  - Compress (gzip)                                          │
│  - Send via gRPC-Web (or HTTP fallback)                     │
│  - On failure: add to offline queue (IndexedDB)             │
└──────────────────────────────────────────────────────────────┘
```

### 5.4 Session Management

```typescript
// Session state structure
interface SessionState {
  sessionId: string;
  userId: string | null;
  startedAt: number;
  lastActivityAt: number;
  pageCount: number;
  eventCount: number;
  isRecording: boolean;  // Sampling decision
}

// Session lifecycle
class SessionManager {
  private state: SessionState;
  private readonly TIMEOUT_MS = 30 * 60 * 1000;  // 30 minutes

  initialize(): void {
    const stored = this.loadFromStorage();

    if (stored && !this.isExpired(stored)) {
      // Resume existing session
      this.state = stored;
      this.state.pageCount++;
    } else {
      // Start new session
      this.state = this.createNewSession();
      this.emit('session_start');
    }

    this.saveToStorage();
    this.startActivityMonitor();
  }

  private isExpired(state: SessionState): boolean {
    const now = Date.now();
    const idleTime = now - state.lastActivityAt;
    return idleTime > this.TIMEOUT_MS;
  }

  private createNewSession(): SessionState {
    return {
      sessionId: generateUUID(),
      userId: this.loadUserId(),
      startedAt: Date.now(),
      lastActivityAt: Date.now(),
      pageCount: 1,
      eventCount: 0,
      isRecording: this.shouldRecord()  // Sampling
    };
  }

  recordActivity(): void {
    this.state.lastActivityAt = Date.now();
    this.state.eventCount++;
    this.saveToStorage();
  }
}
```

### 5.5 Element Naming Algorithm

```typescript
function getElementName(element: HTMLElement): string {
  // Priority order for naming
  const strategies = [
    // 1. Explicit GoSight name
    () => element.getAttribute('data-gosight-name'),

    // 2. Test IDs (commonly used)
    () => element.getAttribute('data-testid'),
    () => element.getAttribute('data-test-id'),
    () => element.getAttribute('data-cy'),
    () => element.getAttribute('data-analytics'),

    // 3. Element ID
    () => element.id ? `#${element.id}` : null,

    // 4. Aria label
    () => element.getAttribute('aria-label'),

    // 5. Form field name
    () => (element as HTMLInputElement).name,

    // 6. Button/link text (truncated)
    () => {
      const text = element.textContent?.trim();
      if (text && text.length <= 50) {
        return text;
      }
      return text?.substring(0, 47) + '...';
    },

    // 7. Fallback: tag + classes
    () => {
      const tag = element.tagName.toLowerCase();
      const classes = Array.from(element.classList).slice(0, 2).join('.');
      return classes ? `${tag}.${classes}` : tag;
    }
  ];

  for (const strategy of strategies) {
    const name = strategy();
    if (name) return name;
  }

  return 'unknown';
}
```

### 5.6 Unique Selector Generation

```typescript
function generateSelector(element: HTMLElement): string {
  // Try ID first
  if (element.id) {
    return `#${CSS.escape(element.id)}`;
  }

  // Build path from root
  const path: string[] = [];
  let current: HTMLElement | null = element;

  while (current && current !== document.body) {
    let selector = current.tagName.toLowerCase();

    // Add distinguishing attributes
    if (current.id) {
      selector = `#${CSS.escape(current.id)}`;
      path.unshift(selector);
      break;
    }

    // Add nth-child if needed
    const parent = current.parentElement;
    if (parent) {
      const siblings = Array.from(parent.children).filter(
        (el) => el.tagName === current!.tagName
      );
      if (siblings.length > 1) {
        const index = siblings.indexOf(current) + 1;
        selector += `:nth-of-type(${index})`;
      }
    }

    path.unshift(selector);
    current = parent;
  }

  return path.join(' > ');
}
```

---

## 6. Transport Layer

### 6.1 Event Batching

```typescript
class EventBuffer {
  private queue: Event[] = [];
  private timer: number | null = null;
  private readonly MAX_SIZE = 10;
  private readonly MAX_WAIT_MS = 5000;

  add(event: Event): void {
    this.queue.push(event);

    // Start timer if not running
    if (!this.timer) {
      this.timer = window.setTimeout(() => this.flush(), this.MAX_WAIT_MS);
    }

    // Flush immediately if buffer full
    if (this.queue.length >= this.MAX_SIZE) {
      this.flush();
    }

    // Flush immediately for priority events
    if (this.isPriorityEvent(event)) {
      this.flush();
    }
  }

  private isPriorityEvent(event: Event): boolean {
    return ['js_error', 'session_end', 'page_exit'].includes(event.type);
  }

  async flush(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }

    if (this.queue.length === 0) return;

    const events = [...this.queue];
    this.queue = [];

    try {
      await this.sender.send(events);
    } catch (error) {
      // Store in offline queue
      await this.offlineQueue.add(events);
    }
  }
}
```

### 6.2 Network Transport

```typescript
class EventSender {
  private readonly endpoint: string;
  private readonly projectKey: string;

  async send(events: Event[]): Promise<void> {
    const batch = {
      projectKey: this.projectKey,
      events: events,
      sentAt: Date.now(),
      sdkVersion: SDK_VERSION
    };

    // Serialize to Protobuf
    const payload = EventBatch.encode(batch).finish();

    // Compress
    const compressed = await this.compress(payload);

    // Try gRPC-Web first
    try {
      await this.sendGrpc(compressed);
      return;
    } catch (grpcError) {
      // Fallback to HTTP
      await this.sendHttp(compressed);
    }
  }

  private async sendGrpc(data: Uint8Array): Promise<void> {
    // gRPC-Web implementation
    const response = await fetch(`${this.endpoint}/gosight.IngestService/SendBatch`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/grpc-web+proto',
        'X-Grpc-Web': '1',
        'Content-Encoding': 'gzip'
      },
      body: data,
      keepalive: true  // Important for beforeunload
    });

    if (!response.ok) {
      throw new Error(`gRPC error: ${response.status}`);
    }
  }

  private async sendHttp(data: Uint8Array): Promise<void> {
    // HTTP fallback
    const response = await fetch(`${this.endpoint}/v1/events`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-protobuf',
        'Content-Encoding': 'gzip',
        'X-GoSight-Key': this.projectKey
      },
      body: data,
      keepalive: true
    });

    if (!response.ok) {
      throw new Error(`HTTP error: ${response.status}`);
    }
  }

  private async compress(data: Uint8Array): Promise<Uint8Array> {
    // Use CompressionStream if available, otherwise fall back to pako
    if ('CompressionStream' in window) {
      const stream = new CompressionStream('gzip');
      const writer = stream.writable.getWriter();
      writer.write(data);
      writer.close();

      const reader = stream.readable.getReader();
      const chunks: Uint8Array[] = [];

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
      }

      return this.concat(chunks);
    }

    // Fallback to pako
    return pako.gzip(data);
  }
}
```

### 6.3 Offline Support

```typescript
class OfflineQueue {
  private readonly dbName = 'gosight_offline';
  private readonly storeName = 'events';
  private db: IDBDatabase | null = null;

  async init(): Promise<void> {
    this.db = await new Promise((resolve, reject) => {
      const request = indexedDB.open(this.dbName, 1);

      request.onerror = () => reject(request.error);
      request.onsuccess = () => resolve(request.result);

      request.onupgradeneeded = (event) => {
        const db = (event.target as IDBOpenDBRequest).result;
        db.createObjectStore(this.storeName, {
          keyPath: 'id',
          autoIncrement: true
        });
      };
    });

    // Listen for online event to flush
    window.addEventListener('online', () => this.flush());
  }

  async add(events: Event[]): Promise<void> {
    if (!this.db) return;

    const tx = this.db.transaction(this.storeName, 'readwrite');
    const store = tx.objectStore(this.storeName);

    store.add({
      events,
      timestamp: Date.now()
    });

    await new Promise((resolve, reject) => {
      tx.oncomplete = resolve;
      tx.onerror = () => reject(tx.error);
    });
  }

  async flush(): Promise<void> {
    if (!this.db || !navigator.onLine) return;

    const tx = this.db.transaction(this.storeName, 'readwrite');
    const store = tx.objectStore(this.storeName);
    const items = await this.getAll(store);

    for (const item of items) {
      try {
        await this.sender.send(item.events);
        store.delete(item.id);
      } catch (error) {
        // Keep in queue for next attempt
        break;
      }
    }
  }
}
```

---

## 7. Privacy Implementation

### 7.1 Input Masking

```typescript
class PrivacyManager {
  private config: PrivacyConfig;

  shouldMaskInput(element: HTMLInputElement): boolean {
    // Always mask these types
    const alwaysMask = ['password', 'hidden'];
    if (alwaysMask.includes(element.type)) {
      return true;
    }

    // Check config
    if (this.config.maskAllInputs) {
      return true;
    }

    if (this.config.maskInputTypes?.includes(element.type)) {
      return true;
    }

    // Check selector match
    if (this.matchesAny(element, this.config.maskSelectors)) {
      return true;
    }

    // Check data attribute
    if (element.closest('[data-gosight-mask]')) {
      return true;
    }

    return false;
  }

  maskValue(value: string): string {
    // Apply regex patterns
    let masked = value;

    for (const regex of this.config.maskRegex || []) {
      masked = masked.replace(regex, '████');
    }

    // Default masking: preserve length, replace with bullets
    if (masked === value) {
      masked = '•'.repeat(value.length);
    }

    return masked;
  }
}
```

### 7.2 rrweb Privacy Hooks

```typescript
const replayConfig: recordOptions = {
  maskAllInputs: config.privacy.maskAllInputs,

  maskInputOptions: {
    password: true,
    email: config.privacy.maskAllInputs,
    tel: config.privacy.maskAllInputs
  },

  maskTextFn: (text: string, element: HTMLElement) => {
    // Check if element should be masked
    if (element.closest('[data-gosight-mask]')) {
      return '████████';
    }

    // Apply regex masking
    return this.privacyManager.maskValue(text);
  },

  blockSelector: config.privacy.blockSelectors?.join(', '),

  blockClass: 'gosight-block',

  ignoreClass: 'gosight-ignore',

  maskInputFn: (text: string, element: HTMLElement) => {
    if (this.privacyManager.shouldMaskInput(element as HTMLInputElement)) {
      return '•'.repeat(text.length);
    }
    return text;
  }
};
```

---

## 8. Framework Integrations

### 8.1 React

```typescript
// hooks/useGoSight.ts
import { useEffect } from 'react';
import { GoSight } from '@gosight/sdk';

export function useGoSight(config: GoSightConfig) {
  useEffect(() => {
    GoSight.init(config);

    return () => {
      GoSight.flush();
    };
  }, []);
}

export function useTrack() {
  return {
    track: GoSight.track,
    identify: GoSight.identify
  };
}

// Usage in App.tsx
function App() {
  useGoSight({
    projectKey: 'gs_abc123'
  });

  return <MainContent />;
}
```

### 8.2 Vue

```typescript
// plugins/gosight.ts
import { Plugin } from 'vue';
import { GoSight } from '@gosight/sdk';

export const gosightPlugin: Plugin = {
  install(app, config: GoSightConfig) {
    GoSight.init(config);

    app.config.globalProperties.$gosight = GoSight;

    app.provide('gosight', GoSight);
  }
};

// main.ts
import { gosightPlugin } from './plugins/gosight';

app.use(gosightPlugin, {
  projectKey: 'gs_abc123'
});
```

### 8.3 Next.js

```typescript
// app/providers.tsx
'use client';

import { useEffect } from 'react';
import { GoSight } from '@gosight/sdk';

export function GoSightProvider({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    GoSight.init({
      projectKey: process.env.NEXT_PUBLIC_GOSIGHT_KEY!
    });
  }, []);

  return <>{children}</>;
}

// app/layout.tsx
import { GoSightProvider } from './providers';

export default function RootLayout({ children }) {
  return (
    <html>
      <body>
        <GoSightProvider>{children}</GoSightProvider>
      </body>
    </html>
  );
}
```

---

## 9. Testing

### 9.1 Unit Test Example

```typescript
import { describe, it, expect, vi } from 'vitest';
import { EventCapture } from '../src/capture/events';

describe('EventCapture', () => {
  it('should capture click events', () => {
    const onEvent = vi.fn();
    const capture = new EventCapture({ onEvent });

    capture.init();

    // Simulate click
    const button = document.createElement('button');
    button.textContent = 'Click me';
    document.body.appendChild(button);
    button.click();

    expect(onEvent).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'click',
        target: expect.objectContaining({
          tag: 'button',
          text: 'Click me'
        })
      })
    );
  });

  it('should ignore elements with data-gosight-ignore', () => {
    const onEvent = vi.fn();
    const capture = new EventCapture({ onEvent });

    capture.init();

    const button = document.createElement('button');
    button.setAttribute('data-gosight-ignore', '');
    document.body.appendChild(button);
    button.click();

    expect(onEvent).not.toHaveBeenCalled();
  });
});
```

---

## 10. Build & Distribution

### 10.1 Build Configuration

```typescript
// vite.config.ts
import { defineConfig } from 'vite';
import { resolve } from 'path';
import dts from 'vite-plugin-dts';

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'GoSight',
      formats: ['es', 'umd', 'iife'],
      fileName: (format) => `sdk.${format}.js`
    },
    rollupOptions: {
      output: {
        globals: {
          rrweb: 'rrweb'
        }
      }
    },
    minify: 'terser',
    sourcemap: true
  },
  plugins: [dts()]
});
```

### 10.2 Output Files

```
dist/
├── sdk.es.js       # ES Module (npm)
├── sdk.es.js.map
├── sdk.umd.js      # UMD (CDN)
├── sdk.umd.js.map
├── sdk.iife.js     # IIFE (script tag)
├── sdk.iife.js.map
├── index.d.ts      # TypeScript definitions
└── sdk.min.js      # Minified bundle (< 30KB gzip)
```

---

## 11. References

- [Event Catalog](./06-event-catalog.md)
- [Data Models](./03-data-models.md)
- [Privacy Module](./08-privacy-module.md)
- [Session Replay](./09-session-replay.md)
