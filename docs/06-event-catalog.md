# GoSight - Event Catalog

## 1. Overview

This document catalogs all event types captured by the GoSight SDK. Events are organized by category, with each event having a standardized structure.

---

## 2. Base Event Structure

Every event contains these base fields:

```typescript
interface BaseEvent {
  event_id: string;        // UUID v4
  session_id: string;      // UUID v4
  user_id: string | null;  // Custom user ID (from identify())
  project_id: string;      // From API key
  event_type: string;      // Event type enum
  timestamp: number;       // Unix timestamp (milliseconds)
  page: PageContext;       // Current page info
  device: DeviceInfo;      // Browser/device info
  custom: Record<string, any>;  // Custom attributes
}

interface PageContext {
  url: string;           // Full URL
  path: string;          // Path only
  title: string;         // Page title
  referrer: string;      // Referrer URL
  query_params: Record<string, string>;
}

interface DeviceInfo {
  user_agent: string;
  browser: string;
  browser_version: string;
  os: string;
  os_version: string;
  device_type: 'desktop' | 'tablet' | 'mobile';
  screen_width: number;
  screen_height: number;
  viewport_width: number;
  viewport_height: number;
  device_pixel_ratio: number;
  language: string;
  timezone: string;
}
```

---

## 3. Event Categories

| Category | Events | Default |
|----------|--------|---------|
| SESSION | session_start, session_end | ✅ ON |
| PAGE | page_view, page_exit, page_visible, page_hidden | ✅ ON |
| MOUSE | click, dblclick, context_menu, mouse_move, mouse_enter, mouse_leave | ✅ ON |
| SCROLL | scroll, scroll_milestone | ✅ ON |
| INPUT | input_focus, input_blur, input_change | ✅ ON |
| FORM | form_start, form_submit, form_abandon, form_error | ✅ ON |
| ERROR | js_error, unhandled_rejection, resource_error, console_error | ✅ ON |
| PERFORMANCE | page_load, web_vitals, long_task, memory | ✅ ON |
| NETWORK | xhr_request, fetch_request, websocket_open, websocket_close, websocket_error | ❌ OFF |
| MEDIA | media_play, media_pause, media_seek, media_complete, media_error | ❌ OFF |
| CLIPBOARD | copy, cut, paste | ❌ OFF |
| SELECTION | text_select, text_deselect | ❌ OFF |
| RESIZE | window_resize, orientation_change | ✅ ON |
| REPLAY | dom_snapshot, dom_mutation, style_mutation, canvas_snapshot, font_load | ✅ ON |
| CUSTOM | custom_event, identify, group | ✅ ON |
| INSIGHT | rage_click, dead_click, error_click, thrashed_cursor, u_turn, slow_page | Auto |

---

## 4. SESSION Events

### session_start

Fired when a new session begins.

```typescript
interface SessionStartPayload {
  is_new_user: boolean;          // First time visitor
  entry_url: string;             // Landing page URL
  utm_source: string | null;     // UTM source
  utm_medium: string | null;     // UTM medium
  utm_campaign: string | null;   // UTM campaign
  utm_term: string | null;       // UTM term
  utm_content: string | null;    // UTM content
  referrer_domain: string | null; // Referrer domain
}
```

**Example:**
```json
{
  "event_type": "session_start",
  "payload": {
    "is_new_user": false,
    "entry_url": "https://example.com/pricing",
    "utm_source": "google",
    "utm_medium": "cpc",
    "utm_campaign": "winter_sale",
    "referrer_domain": "google.com"
  }
}
```

---

### session_end

Fired when a session ends.

```typescript
interface SessionEndPayload {
  duration_ms: number;           // Total session duration
  page_count: number;            // Pages visited
  event_count: number;           // Total events
  end_reason: 'timeout' | 'navigation' | 'close';
}
```

**Triggers:**
- 30 minutes of inactivity (timeout)
- Navigation to external domain
- Tab/window close

---

## 5. PAGE Events

### page_view

Fired when a page is loaded or navigated to (SPA).

```typescript
interface PageViewPayload {
  page_title: string;
  page_path: string;
  page_hash: string | null;
}
```

---

### page_exit

Fired when leaving a page.

```typescript
interface PageExitPayload {
  time_on_page_ms: number;
  max_scroll_depth: number;     // Percentage (0-100)
  exit_url: string | null;      // Next page URL (if internal)
}
```

---

### page_visible

Fired when tab becomes visible (focus).

```typescript
interface PageVisiblePayload {
  hidden_duration_ms: number;   // Time tab was hidden
}
```

---

### page_hidden

Fired when tab becomes hidden (blur/minimize).

```typescript
interface PageHiddenPayload {
  // No additional fields
}
```

---

## 6. MOUSE Events

### click

Fired on mouse click.

```typescript
interface ClickPayload {
  x: number;                    // Click X coordinate
  y: number;                    // Click Y coordinate
  target: TargetElement;
  button: 'left' | 'middle' | 'right';
}

interface TargetElement {
  tag: string;                  // HTML tag name
  id: string | null;            // Element ID
  classes: string[];            // CSS classes
  text: string | null;          // Inner text (truncated 50 chars)
  href: string | null;          // For <a> tags
  name: string;                 // GoSight name (see naming algorithm)
  selector: string;             // Unique CSS selector
  attributes: Record<string, string>;  // data-* attributes
  rect: {
    top: number;
    left: number;
    width: number;
    height: number;
  };
}
```

**Example:**
```json
{
  "event_type": "click",
  "payload": {
    "x": 450,
    "y": 320,
    "target": {
      "tag": "button",
      "id": "submit-btn",
      "classes": ["btn", "btn-primary"],
      "text": "Sign Up",
      "href": null,
      "name": "submit-btn",
      "selector": "#submit-btn",
      "attributes": {
        "data-testid": "signup-submit"
      },
      "rect": {
        "top": 300,
        "left": 400,
        "width": 120,
        "height": 40
      }
    },
    "button": "left"
  }
}
```

---

### dblclick

Fired on double click. Same payload as `click`.

---

### context_menu

Fired on right-click. Same payload as `click`.

---

### mouse_move

Fired periodically with mouse positions (batched).

```typescript
interface MouseMovePayload {
  positions: Array<{
    x: number;
    y: number;
    t: number;  // Relative timestamp (ms from event start)
  }>;
}
```

**Sampling:** Every 50ms (configurable)

---

### mouse_enter

Fired when mouse enters an element.

```typescript
interface MouseEnterPayload {
  target: TargetElement;
}
```

**Note:** Only tracked for interactive elements.

---

### mouse_leave

Fired when mouse leaves an element.

```typescript
interface MouseLeavePayload {
  target: TargetElement;
  hover_duration_ms: number;
}
```

---

## 7. SCROLL Events

### scroll

Fired on scroll (throttled).

```typescript
interface ScrollPayload {
  scroll_top: number;           // Pixels from top
  scroll_depth_px: number;      // Max scroll depth in pixels
  scroll_depth_percent: number; // Max scroll depth as percentage (0-100)
  page_height: number;          // Total page height
  direction: 'up' | 'down';
  velocity: number | null;      // Pixels per second
}
```

**Throttle:** 100ms (configurable)

---

### scroll_milestone

Fired when user reaches scroll milestones.

```typescript
interface ScrollMilestonePayload {
  milestone: 25 | 50 | 75 | 90 | 100;
}
```

**Note:** Each milestone fires only once per page.

---

## 8. INPUT Events

### input_focus

Fired when an input receives focus.

```typescript
interface InputFocusPayload {
  target: {
    tag: string;              // input, textarea, select
    type: string;             // text, email, password, etc.
    name: string | null;      // Field name attribute
    id: string | null;
    selector: string;
  };
}
```

---

### input_blur

Fired when an input loses focus.

```typescript
interface InputBlurPayload {
  target: InputTarget;
  focus_duration_ms: number;
  value_changed: boolean;
}
```

---

### input_change

Fired when input value changes.

```typescript
interface InputChangePayload {
  target: InputTarget;
  value_length: number;       // Length of value (not the value itself)
  is_masked: boolean;         // Was the value masked?
}
```

**Privacy:** Actual values are never captured. Only length is recorded.

---

## 9. FORM Events

### form_start

Fired when user starts filling a form.

```typescript
interface FormStartPayload {
  form_id: string | null;
  form_name: string | null;
  form_selector: string;
  field_count: number;
}
```

---

### form_submit

Fired when form is submitted.

```typescript
interface FormSubmitPayload {
  form_id: string | null;
  form_name: string | null;
  form_selector: string;
  success: boolean;           // Did submission succeed?
  time_to_complete_ms: number;
  field_count: number;
  fields_filled: number;
}
```

---

### form_abandon

Fired when user leaves page with incomplete form.

```typescript
interface FormAbandonPayload {
  form_id: string | null;
  form_name: string | null;
  fields_filled: number;
  fields_total: number;
  last_field: string;         // Last focused field name
  time_spent_ms: number;
}
```

---

### form_error

Fired when form validation error occurs.

```typescript
interface FormErrorPayload {
  form_id: string | null;
  field_name: string;
  field_selector: string;
  error_message: string;
  error_type: 'validation' | 'required' | 'format' | 'custom';
}
```

---

## 10. ERROR Events

### js_error

Fired on JavaScript error.

```typescript
interface JsErrorPayload {
  message: string;
  stack: string | null;
  filename: string | null;
  lineno: number | null;
  colno: number | null;
  error_type: string;         // TypeError, ReferenceError, etc.
}
```

**Example:**
```json
{
  "event_type": "js_error",
  "payload": {
    "message": "Cannot read property 'map' of undefined",
    "stack": "TypeError: Cannot read property 'map' of undefined\n    at render (app.js:142:5)\n    at ...",
    "filename": "https://example.com/js/app.js",
    "lineno": 142,
    "colno": 5,
    "error_type": "TypeError"
  }
}
```

---

### unhandled_rejection

Fired on unhandled Promise rejection.

```typescript
interface UnhandledRejectionPayload {
  reason: string;
  stack: string | null;
}
```

---

### resource_error

Fired when a resource fails to load.

```typescript
interface ResourceErrorPayload {
  resource_url: string;
  resource_type: 'script' | 'stylesheet' | 'image' | 'font' | 'other';
  status_code: number | null;
}
```

---

### console_error

Fired on console.error() calls.

```typescript
interface ConsoleErrorPayload {
  message: string;
  args: string[];             // Serialized arguments
  stack: string | null;
}
```

---

## 11. PERFORMANCE Events

### page_load

Fired when page finishes loading.

```typescript
interface PageLoadPayload {
  ttfb: number;                    // Time to First Byte
  dom_content_loaded: number;      // DOMContentLoaded
  load_complete: number;           // Load event
  transfer_size: number | null;    // Bytes transferred
}
```

---

### web_vitals

Fired with Core Web Vitals metrics.

```typescript
interface WebVitalsPayload {
  lcp: number | null;              // Largest Contentful Paint
  fid: number | null;              // First Input Delay
  cls: number | null;              // Cumulative Layout Shift
  fcp: number | null;              // First Contentful Paint
  inp: number | null;              // Interaction to Next Paint
  ttfb: number | null;             // Time to First Byte
}
```

---

### long_task

Fired when a JavaScript task exceeds 50ms.

```typescript
interface LongTaskPayload {
  duration: number;                // Task duration (ms)
  start_time: number;              // Start timestamp
  attribution: string | null;      // Script URL if available
}
```

---

### memory

Fired periodically with memory usage.

```typescript
interface MemoryPayload {
  used_js_heap: number;            // Bytes
  total_js_heap: number;           // Bytes
}
```

**Frequency:** Every 30 seconds

---

## 12. NETWORK Events (Optional)

### xhr_request / fetch_request

Fired on XHR/Fetch completion.

```typescript
interface NetworkRequestPayload {
  method: string;                  // GET, POST, etc.
  url: string;
  status: number;
  duration_ms: number;
  request_size: number | null;
  response_size: number | null;
  request_type: 'xhr' | 'fetch';
}
```

---

### websocket_open

Fired when WebSocket connects.

```typescript
interface WebSocketOpenPayload {
  url: string;
}
```

---

### websocket_close

Fired when WebSocket disconnects.

```typescript
interface WebSocketClosePayload {
  url: string;
  code: number;
  reason: string;
  duration_ms: number;
}
```

---

## 13. MEDIA Events (Optional)

### media_play

Fired when video/audio starts playing.

```typescript
interface MediaPlayPayload {
  media_type: 'video' | 'audio';
  media_src: string;
  media_id: string | null;
  current_time: number;            // Seconds
  duration: number;                // Total duration
}
```

---

### media_pause

Fired when video/audio is paused.

```typescript
interface MediaPausePayload {
  media_type: 'video' | 'audio';
  media_src: string;
  media_id: string | null;
  current_time: number;
  watch_duration: number;          // Time spent watching
}
```

---

### media_complete

Fired when video/audio ends.

```typescript
interface MediaCompletePayload {
  media_type: 'video' | 'audio';
  media_src: string;
  media_id: string | null;
  total_duration: number;
  watch_duration: number;
  completion_rate: number;         // Percentage watched
}
```

---

## 14. RESIZE Events

### window_resize

Fired on window resize (debounced).

```typescript
interface WindowResizePayload {
  width: number;
  height: number;
  previous_width: number;
  previous_height: number;
}
```

**Debounce:** 200ms

---

### orientation_change

Fired on mobile orientation change.

```typescript
interface OrientationChangePayload {
  orientation: 'portrait' | 'landscape';
}
```

---

## 15. REPLAY Events

These events are generated by rrweb for session replay.

### dom_snapshot

Full DOM snapshot (initial or after major change).

```typescript
interface DomSnapshotPayload {
  rrweb_type: 2;                   // rrweb FullSnapshot type
  data: {
    node: SerializedNode;          // Full DOM tree
    initialOffset: {
      top: number;
      left: number;
    };
  };
}
```

---

### dom_mutation

Incremental DOM changes.

```typescript
interface DomMutationPayload {
  rrweb_type: 3;                   // rrweb IncrementalSnapshot type
  data: {
    source: number;                // Mutation source type
    texts: TextMutation[];
    attributes: AttributeMutation[];
    removes: RemoveNode[];
    adds: AddNode[];
  };
}
```

---

## 16. CUSTOM Events

### custom_event

User-defined event via `GoSight.track()`.

```typescript
interface CustomEventPayload {
  name: string;                    // Event name
  properties: Record<string, any>; // Custom properties
  category: string | null;         // Optional category
}
```

**Example:**
```json
{
  "event_type": "custom_event",
  "payload": {
    "name": "purchase_completed",
    "properties": {
      "order_id": "ORD-123",
      "total": 99.99,
      "currency": "USD",
      "items": 3
    },
    "category": "ecommerce"
  }
}
```

---

### identify

User identification via `GoSight.identify()`.

```typescript
interface IdentifyPayload {
  user_id: string;
  traits: Record<string, any>;
}
```

**Example:**
```json
{
  "event_type": "identify",
  "payload": {
    "user_id": "user_123",
    "traits": {
      "email": "user@example.com",
      "name": "John Doe",
      "plan": "premium",
      "company": "Acme Inc"
    }
  }
}
```

---

### group

Group association via `GoSight.group()`.

```typescript
interface GroupPayload {
  group_id: string;
  traits: Record<string, any>;
}
```

---

## 17. INSIGHT Events (Server-Generated)

These events are computed server-side from raw events.

### rage_click

User clicking rapidly in frustration.

```typescript
interface RageClickPayload {
  click_count: number;             // Number of clicks
  time_window_ms: number;          // Time window
  center_x: number;                // Center of click cluster
  center_y: number;
  radius: number;                  // Cluster radius
  target_selector: string;
  related_event_ids: string[];     // Original click event IDs
}
```

**Detection:** ≥5 clicks within 2 seconds in 50px radius.

---

### dead_click

Click on non-interactive element.

```typescript
interface DeadClickPayload {
  x: number;
  y: number;
  target_selector: string;
  reason: 'no_handler' | 'no_navigation' | 'no_mutation';
}
```

**Detection:** Click with no resulting DOM change, navigation, or event handler.

---

### error_click

Click that leads to JavaScript error.

```typescript
interface ErrorClickPayload {
  x: number;
  y: number;
  target_selector: string;
  error_event_id: string;          // Related error event
  time_to_error_ms: number;        // Time between click and error
}
```

**Detection:** JS error within 1 second of click.

---

### thrashed_cursor

Erratic mouse movement indicating confusion.

```typescript
interface ThrashedCursorPayload {
  duration_ms: number;
  distance_px: number;             // Total distance traveled
  direction_changes: number;       // Number of direction changes
  area_bounds: {
    top: number;
    left: number;
    width: number;
    height: number;
  };
}
```

**Detection:** High velocity movement with many direction changes.

---

### u_turn

User quickly returns to previous page.

```typescript
interface UTurnPayload {
  from_url: string;
  to_url: string;
  return_url: string;
  time_away_ms: number;
}
```

**Detection:** Return to previous page within 10 seconds.

---

### slow_page

Page with slow load time.

```typescript
interface SlowPagePayload {
  url: string;
  load_time_ms: number;
  threshold_ms: number;
}
```

**Detection:** Load time exceeds threshold (default 3 seconds).

---

## 18. Event Defaults & Sampling

| Event Category | Default State | Sampling |
|---------------|---------------|----------|
| SESSION | ON | None |
| PAGE | ON | None |
| MOUSE (click) | ON | None |
| MOUSE (move) | ON | 50ms throttle |
| SCROLL | ON | 100ms throttle |
| INPUT | ON | None |
| FORM | ON | None |
| ERROR | ON | None (priority) |
| PERFORMANCE | ON | None |
| NETWORK | OFF | None |
| MEDIA | OFF | None |
| CLIPBOARD | OFF | None |
| SELECTION | OFF | None |
| RESIZE | ON | 200ms debounce |
| REPLAY | ON | rrweb default |
| CUSTOM | ON | None |

---

## 19. References

- [SDK Specification](./04-sdk-specification.md)
- [Data Models](./03-data-models.md)
- [UX Insights Algorithms](./07-ux-insights-algorithms.md)
