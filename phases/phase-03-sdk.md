# Phase 3: JavaScript SDK

## Mục Tiêu

Xây dựng client-side SDK để capture events và session replay.

## Prerequisites

- Phase 1 hoàn thành (Protobuf definitions)
- Phase 2 đang chạy (Ingestor service) - optional for testing

## Tasks

### 3.1 Project Setup

```bash
cd sdk
npm init -y
```

**`package.json`**

```json
{
  "name": "@gosight/sdk",
  "version": "1.0.0",
  "description": "GoSight Analytics SDK",
  "main": "dist/gosight.cjs.js",
  "module": "dist/gosight.esm.js",
  "browser": "dist/gosight.iife.js",
  "types": "dist/index.d.ts",
  "files": [
    "dist"
  ],
  "scripts": {
    "build": "rollup -c",
    "dev": "rollup -c -w",
    "test": "vitest",
    "lint": "eslint src --ext .ts",
    "prepublishOnly": "npm run build"
  },
  "dependencies": {
    "rrweb": "^2.0.0-alpha.11"
  },
  "devDependencies": {
    "@rollup/plugin-commonjs": "^25.0.7",
    "@rollup/plugin-node-resolve": "^15.2.3",
    "@rollup/plugin-terser": "^0.4.4",
    "@rollup/plugin-typescript": "^11.1.6",
    "@types/node": "^20.10.0",
    "rollup": "^4.9.0",
    "typescript": "^5.3.0",
    "vitest": "^1.1.0"
  }
}
```

**`tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "declaration": true,
    "declarationDir": "dist",
    "strict": true,
    "noImplicitAny": true,
    "strictNullChecks": true,
    "moduleResolution": "node",
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "outDir": "dist",
    "rootDir": "src"
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
```

**`rollup.config.js`**

```javascript
import resolve from '@rollup/plugin-node-resolve';
import commonjs from '@rollup/plugin-commonjs';
import typescript from '@rollup/plugin-typescript';
import terser from '@rollup/plugin-terser';

const production = !process.env.ROLLUP_WATCH;

export default [
  // ESM build
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/gosight.esm.js',
      format: 'esm',
      sourcemap: true,
    },
    plugins: [
      resolve(),
      commonjs(),
      typescript({ tsconfig: './tsconfig.json' }),
      production && terser(),
    ],
  },
  // CJS build
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/gosight.cjs.js',
      format: 'cjs',
      sourcemap: true,
    },
    plugins: [
      resolve(),
      commonjs(),
      typescript({ tsconfig: './tsconfig.json' }),
      production && terser(),
    ],
  },
  // IIFE build (for CDN)
  {
    input: 'src/index.ts',
    output: {
      file: 'dist/gosight.iife.js',
      format: 'iife',
      name: 'GoSight',
      sourcemap: true,
    },
    plugins: [
      resolve(),
      commonjs(),
      typescript({ tsconfig: './tsconfig.json' }),
      production && terser(),
    ],
  },
];
```

---

### 3.2 Core SDK

**`src/index.ts`**

```typescript
import { GoSightCore } from './core/gosight';
import type { GoSightConfig } from './types';

// Global instance
let instance: GoSightCore | null = null;

// Main API
const GoSight = {
  init(config: GoSightConfig): void {
    if (instance) {
      console.warn('GoSight already initialized');
      return;
    }
    instance = new GoSightCore(config);
    instance.start();
  },

  identify(userId: string, traits?: Record<string, any>): void {
    instance?.identify(userId, traits);
  },

  track(eventName: string, properties?: Record<string, any>): void {
    instance?.track(eventName, properties);
  },

  setCustom(key: string, value: any): void {
    instance?.setCustom(key, value);
  },

  stop(): void {
    instance?.stop();
    instance = null;
  },

  getSessionId(): string | null {
    return instance?.getSessionId() ?? null;
  },

  getSessionUrl(): string | null {
    return instance?.getSessionUrl() ?? null;
  },
};

export default GoSight;
export { GoSight };
export type { GoSightConfig };
```

**`src/types.ts`**

```typescript
export interface GoSightConfig {
  projectKey: string;
  ingestUrl?: string;

  // Session
  sessionTimeout?: number;  // Default: 30 minutes

  // Privacy
  privacy?: PrivacyConfig;

  // Features
  events?: EventConfig;
  replay?: ReplayConfig;

  // Debug
  debug?: boolean;
}

export interface PrivacyConfig {
  maskAllInputs?: boolean;
  maskInputTypes?: string[];
  maskSelectors?: string[];
  blockSelectors?: string[];
  ignoreSelectors?: string[];
  blockUrls?: string[];
  allowUrls?: string[];
  maskAllText?: boolean;
  maskRegex?: RegExp[];
  anonymizeIp?: boolean;
}

export interface EventConfig {
  clicks?: boolean;
  scroll?: boolean;
  input?: boolean;
  errors?: boolean;
  performance?: boolean;
  network?: boolean;
}

export interface ReplayConfig {
  enabled?: boolean;
  sampling?: number;  // 0-1, percentage of sessions to record
  checkoutEveryNms?: number;
  checkoutEveryNth?: number;
}

export interface EventPayload {
  eventId: string;
  type: string;
  timestamp: number;
  url: string;
  path: string;
  title: string;
  referrer: string;
  payload?: any;
}

export interface SessionMeta {
  sessionId: string;
  userId?: string;
  startedAt: number;
  device: DeviceInfo;
}

export interface DeviceInfo {
  browser: string;
  browserVersion: string;
  os: string;
  osVersion: string;
  deviceType: string;
  screenWidth: number;
  screenHeight: number;
  viewportWidth: number;
  viewportHeight: number;
  language: string;
  timezone: string;
}
```

**`src/core/gosight.ts`**

```typescript
import { EventCapture } from '../events/capture';
import { ReplayRecorder } from '../replay/recorder';
import { Transport } from '../transport/transport';
import { SessionManager } from './session';
import { EventBuffer } from './buffer';
import type { GoSightConfig, EventPayload } from '../types';

export class GoSightCore {
  private config: Required<GoSightConfig>;
  private session: SessionManager;
  private transport: Transport;
  private buffer: EventBuffer;
  private eventCapture: EventCapture;
  private replayRecorder: ReplayRecorder | null = null;
  private isRunning = false;

  constructor(config: GoSightConfig) {
    this.config = this.normalizeConfig(config);
    this.session = new SessionManager(this.config.sessionTimeout);
    this.transport = new Transport(this.config.ingestUrl, this.config.projectKey);
    this.buffer = new EventBuffer(this.transport, this.session);
    this.eventCapture = new EventCapture(this.buffer, this.config);

    if (this.config.replay?.enabled) {
      this.replayRecorder = new ReplayRecorder(this.transport, this.config);
    }
  }

  private normalizeConfig(config: GoSightConfig): Required<GoSightConfig> {
    return {
      projectKey: config.projectKey,
      ingestUrl: config.ingestUrl ?? 'https://ingest.gosight.io',
      sessionTimeout: config.sessionTimeout ?? 30 * 60 * 1000,
      privacy: {
        maskAllInputs: true,
        maskInputTypes: ['password', 'email', 'tel'],
        maskSelectors: [],
        blockSelectors: [],
        ignoreSelectors: [],
        blockUrls: [],
        allowUrls: [],
        maskAllText: false,
        maskRegex: [],
        anonymizeIp: false,
        ...config.privacy,
      },
      events: {
        clicks: true,
        scroll: true,
        input: true,
        errors: true,
        performance: true,
        network: false,
        ...config.events,
      },
      replay: {
        enabled: true,
        sampling: 1.0,
        checkoutEveryNms: 60000,
        checkoutEveryNth: 200,
        ...config.replay,
      },
      debug: config.debug ?? false,
    };
  }

  start(): void {
    if (this.isRunning) return;

    // Check if should record (sampling)
    if (this.config.replay?.enabled && this.config.replay.sampling < 1) {
      const shouldRecord = Math.random() < this.config.replay.sampling;
      if (!shouldRecord) {
        this.config.replay.enabled = false;
      }
    }

    // Check URL blocking
    if (!this.shouldRecordUrl(window.location.href)) {
      this.log('URL blocked from recording');
      return;
    }

    this.isRunning = true;
    this.session.start();
    this.eventCapture.start();
    this.replayRecorder?.start();

    // Track initial page view
    this.trackPageView();

    // Listen for page visibility changes
    document.addEventListener('visibilitychange', this.handleVisibilityChange);

    // Listen for beforeunload
    window.addEventListener('beforeunload', this.handleBeforeUnload);

    this.log('GoSight started', { sessionId: this.session.getId() });
  }

  stop(): void {
    if (!this.isRunning) return;

    this.isRunning = false;
    this.eventCapture.stop();
    this.replayRecorder?.stop();
    this.buffer.flush();

    document.removeEventListener('visibilitychange', this.handleVisibilityChange);
    window.removeEventListener('beforeunload', this.handleBeforeUnload);

    this.log('GoSight stopped');
  }

  identify(userId: string, traits?: Record<string, any>): void {
    this.session.setUserId(userId);
    if (traits) {
      this.buffer.addEvent({
        eventId: this.generateId(),
        type: 'identify',
        timestamp: Date.now(),
        url: window.location.href,
        path: window.location.pathname,
        title: document.title,
        referrer: document.referrer,
        payload: { userId, traits },
      });
    }
  }

  track(eventName: string, properties?: Record<string, any>): void {
    this.buffer.addEvent({
      eventId: this.generateId(),
      type: 'custom',
      timestamp: Date.now(),
      url: window.location.href,
      path: window.location.pathname,
      title: document.title,
      referrer: document.referrer,
      payload: { name: eventName, properties },
    });
  }

  setCustom(key: string, value: any): void {
    this.session.setCustom(key, value);
  }

  getSessionId(): string {
    return this.session.getId();
  }

  getSessionUrl(): string {
    const dashboardUrl = this.config.ingestUrl.replace('ingest.', 'app.');
    return `${dashboardUrl}/sessions/${this.session.getId()}`;
  }

  private trackPageView(): void {
    this.buffer.addEvent({
      eventId: this.generateId(),
      type: 'page_view',
      timestamp: Date.now(),
      url: window.location.href,
      path: window.location.pathname,
      title: document.title,
      referrer: document.referrer,
    });
  }

  private handleVisibilityChange = (): void => {
    if (document.visibilityState === 'hidden') {
      this.buffer.flush();
    }
  };

  private handleBeforeUnload = (): void => {
    this.buffer.flush();
  };

  private shouldRecordUrl(url: string): boolean {
    const { blockUrls, allowUrls } = this.config.privacy;
    const path = new URL(url).pathname;

    // Whitelist mode
    if (allowUrls && allowUrls.length > 0) {
      return allowUrls.some(pattern => this.matchPattern(path, pattern));
    }

    // Blacklist mode
    if (blockUrls && blockUrls.length > 0) {
      return !blockUrls.some(pattern => this.matchPattern(path, pattern));
    }

    return true;
  }

  private matchPattern(path: string, pattern: string): boolean {
    const regex = pattern
      .replace(/[.+^${}()|[\]\\]/g, '\\$&')
      .replace(/\*/g, '.*')
      .replace(/\?/g, '.');
    return new RegExp(`^${regex}$`).test(path);
  }

  private generateId(): string {
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = (Math.random() * 16) | 0;
      const v = c === 'x' ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    });
  }

  private log(...args: any[]): void {
    if (this.config.debug) {
      console.log('[GoSight]', ...args);
    }
  }
}
```

---

### 3.3 Session Management

**`src/core/session.ts`**

```typescript
import type { SessionMeta, DeviceInfo } from '../types';

const STORAGE_KEY = 'gosight_session';

export class SessionManager {
  private sessionId: string;
  private userId?: string;
  private startedAt: number;
  private lastActivity: number;
  private timeout: number;
  private custom: Record<string, any> = {};

  constructor(timeout: number) {
    this.timeout = timeout;
    this.sessionId = '';
    this.startedAt = 0;
    this.lastActivity = 0;
  }

  start(): void {
    const stored = this.loadSession();

    if (stored && !this.isExpired(stored.lastActivity)) {
      // Resume existing session
      this.sessionId = stored.sessionId;
      this.userId = stored.userId;
      this.startedAt = stored.startedAt;
      this.lastActivity = Date.now();
    } else {
      // Start new session
      this.sessionId = this.generateSessionId();
      this.startedAt = Date.now();
      this.lastActivity = Date.now();
    }

    this.saveSession();
  }

  touch(): void {
    this.lastActivity = Date.now();
    this.saveSession();
  }

  getId(): string {
    return this.sessionId;
  }

  getUserId(): string | undefined {
    return this.userId;
  }

  setUserId(userId: string): void {
    this.userId = userId;
    this.saveSession();
  }

  setCustom(key: string, value: any): void {
    this.custom[key] = value;
  }

  getCustom(): Record<string, any> {
    return this.custom;
  }

  getMeta(): SessionMeta {
    return {
      sessionId: this.sessionId,
      userId: this.userId,
      startedAt: this.startedAt,
      device: this.getDeviceInfo(),
    };
  }

  private getDeviceInfo(): DeviceInfo {
    const ua = navigator.userAgent;

    return {
      browser: this.getBrowser(ua),
      browserVersion: this.getBrowserVersion(ua),
      os: this.getOS(ua),
      osVersion: this.getOSVersion(ua),
      deviceType: this.getDeviceType(ua),
      screenWidth: window.screen.width,
      screenHeight: window.screen.height,
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
      language: navigator.language,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    };
  }

  private getBrowser(ua: string): string {
    if (ua.includes('Firefox')) return 'Firefox';
    if (ua.includes('Chrome')) return 'Chrome';
    if (ua.includes('Safari')) return 'Safari';
    if (ua.includes('Edge')) return 'Edge';
    if (ua.includes('Opera')) return 'Opera';
    return 'Unknown';
  }

  private getBrowserVersion(ua: string): string {
    const match = ua.match(/(Chrome|Firefox|Safari|Edge|Opera)\/(\d+)/);
    return match ? match[2] : '';
  }

  private getOS(ua: string): string {
    if (ua.includes('Windows')) return 'Windows';
    if (ua.includes('Mac OS')) return 'macOS';
    if (ua.includes('Linux')) return 'Linux';
    if (ua.includes('Android')) return 'Android';
    if (ua.includes('iOS')) return 'iOS';
    return 'Unknown';
  }

  private getOSVersion(ua: string): string {
    const match = ua.match(/(Windows NT|Mac OS X|Android|iOS) ([\d._]+)/);
    return match ? match[2].replace(/_/g, '.') : '';
  }

  private getDeviceType(ua: string): string {
    if (/Mobile|Android|iPhone|iPad/.test(ua)) {
      return /iPad|Tablet/.test(ua) ? 'tablet' : 'mobile';
    }
    return 'desktop';
  }

  private isExpired(lastActivity: number): boolean {
    return Date.now() - lastActivity > this.timeout;
  }

  private generateSessionId(): string {
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = (Math.random() * 16) | 0;
      const v = c === 'x' ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    });
  }

  private loadSession(): { sessionId: string; userId?: string; startedAt: number; lastActivity: number } | null {
    try {
      const data = sessionStorage.getItem(STORAGE_KEY);
      return data ? JSON.parse(data) : null;
    } catch {
      return null;
    }
  }

  private saveSession(): void {
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
        sessionId: this.sessionId,
        userId: this.userId,
        startedAt: this.startedAt,
        lastActivity: this.lastActivity,
      }));
    } catch {
      // Storage not available
    }
  }
}
```

---

### 3.4 Event Buffer

**`src/core/buffer.ts`**

```typescript
import type { EventPayload } from '../types';
import type { Transport } from '../transport/transport';
import type { SessionManager } from './session';

const MAX_BUFFER_SIZE = 50;
const FLUSH_INTERVAL = 5000; // 5 seconds

export class EventBuffer {
  private buffer: EventPayload[] = [];
  private transport: Transport;
  private session: SessionManager;
  private flushTimer: ReturnType<typeof setInterval> | null = null;

  constructor(transport: Transport, session: SessionManager) {
    this.transport = transport;
    this.session = session;
    this.startFlushTimer();
  }

  addEvent(event: EventPayload): void {
    this.buffer.push(event);
    this.session.touch();

    if (this.buffer.length >= MAX_BUFFER_SIZE) {
      this.flush();
    }
  }

  flush(): void {
    if (this.buffer.length === 0) return;

    const events = [...this.buffer];
    this.buffer = [];

    this.transport.sendEvents(events, this.session.getMeta());
  }

  private startFlushTimer(): void {
    this.flushTimer = setInterval(() => {
      this.flush();
    }, FLUSH_INTERVAL);
  }

  stop(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
      this.flushTimer = null;
    }
    this.flush();
  }
}
```

---

### 3.5 Event Capture

**`src/events/capture.ts`**

```typescript
import type { EventBuffer } from '../core/buffer';
import type { GoSightConfig } from '../types';
import { ClickHandler } from './handlers/click';
import { ScrollHandler } from './handlers/scroll';
import { InputHandler } from './handlers/input';
import { ErrorHandler } from './handlers/error';
import { PerformanceHandler } from './handlers/performance';

export class EventCapture {
  private buffer: EventBuffer;
  private config: Required<GoSightConfig>;
  private handlers: { start: () => void; stop: () => void }[] = [];

  constructor(buffer: EventBuffer, config: Required<GoSightConfig>) {
    this.buffer = buffer;
    this.config = config;
  }

  start(): void {
    const { events, privacy } = this.config;

    if (events.clicks) {
      const handler = new ClickHandler(this.buffer, privacy);
      handler.start();
      this.handlers.push(handler);
    }

    if (events.scroll) {
      const handler = new ScrollHandler(this.buffer);
      handler.start();
      this.handlers.push(handler);
    }

    if (events.input) {
      const handler = new InputHandler(this.buffer, privacy);
      handler.start();
      this.handlers.push(handler);
    }

    if (events.errors) {
      const handler = new ErrorHandler(this.buffer);
      handler.start();
      this.handlers.push(handler);
    }

    if (events.performance) {
      const handler = new PerformanceHandler(this.buffer);
      handler.start();
      this.handlers.push(handler);
    }
  }

  stop(): void {
    this.handlers.forEach(h => h.stop());
    this.handlers = [];
  }
}
```

**`src/events/handlers/click.ts`**

```typescript
import type { EventBuffer } from '../../core/buffer';
import type { PrivacyConfig } from '../../types';

export class ClickHandler {
  private buffer: EventBuffer;
  private privacy: PrivacyConfig;

  constructor(buffer: EventBuffer, privacy: PrivacyConfig) {
    this.buffer = buffer;
    this.privacy = privacy;
  }

  start(): void {
    document.addEventListener('click', this.handleClick, true);
  }

  stop(): void {
    document.removeEventListener('click', this.handleClick, true);
  }

  private handleClick = (e: MouseEvent): void => {
    const target = e.target as HTMLElement;

    // Check if should ignore
    if (this.shouldIgnore(target)) return;

    this.buffer.addEvent({
      eventId: this.generateId(),
      type: 'click',
      timestamp: Date.now(),
      url: window.location.href,
      path: window.location.pathname,
      title: document.title,
      referrer: document.referrer,
      payload: {
        x: e.clientX,
        y: e.clientY,
        target: this.serializeTarget(target),
      },
    });
  };

  private serializeTarget(el: HTMLElement): object {
    return {
      tag: el.tagName.toLowerCase(),
      id: el.id || undefined,
      classes: Array.from(el.classList),
      selector: this.getSelector(el),
      text: this.getText(el),
      href: (el as HTMLAnchorElement).href || undefined,
    };
  }

  private getSelector(el: HTMLElement): string {
    if (el.id) return `#${el.id}`;

    const parts: string[] = [];
    let current: HTMLElement | null = el;

    while (current && current !== document.body) {
      let selector = current.tagName.toLowerCase();

      if (current.id) {
        selector = `#${current.id}`;
        parts.unshift(selector);
        break;
      }

      if (current.className) {
        const classes = Array.from(current.classList).slice(0, 2).join('.');
        if (classes) selector += `.${classes}`;
      }

      parts.unshift(selector);
      current = current.parentElement;
    }

    return parts.slice(-3).join(' > ');
  }

  private getText(el: HTMLElement): string {
    const text = el.textContent?.trim() || '';
    return text.slice(0, 100);
  }

  private shouldIgnore(el: HTMLElement): boolean {
    const { ignoreSelectors } = this.privacy;
    if (!ignoreSelectors?.length) return false;

    return ignoreSelectors.some(sel => el.closest(sel));
  }

  private generateId(): string {
    return Math.random().toString(36).substring(2, 15);
  }
}
```

**`src/events/handlers/error.ts`**

```typescript
import type { EventBuffer } from '../../core/buffer';

export class ErrorHandler {
  private buffer: EventBuffer;

  constructor(buffer: EventBuffer) {
    this.buffer = buffer;
  }

  start(): void {
    window.addEventListener('error', this.handleError);
    window.addEventListener('unhandledrejection', this.handleRejection);
  }

  stop(): void {
    window.removeEventListener('error', this.handleError);
    window.removeEventListener('unhandledrejection', this.handleRejection);
  }

  private handleError = (e: ErrorEvent): void => {
    this.buffer.addEvent({
      eventId: this.generateId(),
      type: 'js_error',
      timestamp: Date.now(),
      url: window.location.href,
      path: window.location.pathname,
      title: document.title,
      referrer: document.referrer,
      payload: {
        message: e.message,
        source: e.filename,
        line: e.lineno,
        column: e.colno,
        stack: e.error?.stack,
        errorType: e.error?.name || 'Error',
      },
    });
  };

  private handleRejection = (e: PromiseRejectionEvent): void => {
    const error = e.reason;

    this.buffer.addEvent({
      eventId: this.generateId(),
      type: 'js_error',
      timestamp: Date.now(),
      url: window.location.href,
      path: window.location.pathname,
      title: document.title,
      referrer: document.referrer,
      payload: {
        message: error?.message || String(error),
        stack: error?.stack,
        errorType: 'UnhandledRejection',
      },
    });
  };

  private generateId(): string {
    return Math.random().toString(36).substring(2, 15);
  }
}
```

**`src/events/handlers/performance.ts`**

```typescript
import type { EventBuffer } from '../../core/buffer';

export class PerformanceHandler {
  private buffer: EventBuffer;

  constructor(buffer: EventBuffer) {
    this.buffer = buffer;
  }

  start(): void {
    // Web Vitals
    this.observeWebVitals();

    // Page load timing
    if (document.readyState === 'complete') {
      this.capturePageLoad();
    } else {
      window.addEventListener('load', () => this.capturePageLoad());
    }
  }

  stop(): void {
    // Observers are automatically cleaned up
  }

  private observeWebVitals(): void {
    // LCP
    this.observeLCP();

    // FID
    this.observeFID();

    // CLS
    this.observeCLS();
  }

  private observeLCP(): void {
    try {
      const observer = new PerformanceObserver((list) => {
        const entries = list.getEntries();
        const lastEntry = entries[entries.length - 1] as any;

        this.buffer.addEvent({
          eventId: this.generateId(),
          type: 'web_vitals',
          timestamp: Date.now(),
          url: window.location.href,
          path: window.location.pathname,
          title: document.title,
          referrer: document.referrer,
          payload: {
            metric: 'LCP',
            value: lastEntry.startTime,
          },
        });
      });

      observer.observe({ type: 'largest-contentful-paint', buffered: true });
    } catch {
      // Not supported
    }
  }

  private observeFID(): void {
    try {
      const observer = new PerformanceObserver((list) => {
        const entries = list.getEntries() as any[];

        for (const entry of entries) {
          this.buffer.addEvent({
            eventId: this.generateId(),
            type: 'web_vitals',
            timestamp: Date.now(),
            url: window.location.href,
            path: window.location.pathname,
            title: document.title,
            referrer: document.referrer,
            payload: {
              metric: 'FID',
              value: entry.processingStart - entry.startTime,
            },
          });
        }
      });

      observer.observe({ type: 'first-input', buffered: true });
    } catch {
      // Not supported
    }
  }

  private observeCLS(): void {
    try {
      let clsValue = 0;

      const observer = new PerformanceObserver((list) => {
        const entries = list.getEntries() as any[];

        for (const entry of entries) {
          if (!entry.hadRecentInput) {
            clsValue += entry.value;
          }
        }
      });

      observer.observe({ type: 'layout-shift', buffered: true });

      // Report on visibility change
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') {
          this.buffer.addEvent({
            eventId: this.generateId(),
            type: 'web_vitals',
            timestamp: Date.now(),
            url: window.location.href,
            path: window.location.pathname,
            title: document.title,
            referrer: document.referrer,
            payload: {
              metric: 'CLS',
              value: clsValue,
            },
          });
        }
      });
    } catch {
      // Not supported
    }
  }

  private capturePageLoad(): void {
    const timing = performance.timing;

    this.buffer.addEvent({
      eventId: this.generateId(),
      type: 'page_load',
      timestamp: Date.now(),
      url: window.location.href,
      path: window.location.pathname,
      title: document.title,
      referrer: document.referrer,
      payload: {
        dnsLookup: timing.domainLookupEnd - timing.domainLookupStart,
        tcpConnect: timing.connectEnd - timing.connectStart,
        ttfb: timing.responseStart - timing.requestStart,
        responseTime: timing.responseEnd - timing.responseStart,
        domInteractive: timing.domInteractive - timing.navigationStart,
        domComplete: timing.domComplete - timing.navigationStart,
        loadComplete: timing.loadEventEnd - timing.navigationStart,
      },
    });
  }

  private generateId(): string {
    return Math.random().toString(36).substring(2, 15);
  }
}
```

---

### 3.6 Replay Recording

**`src/replay/recorder.ts`**

```typescript
import { record, EventType } from 'rrweb';
import type { Transport } from '../transport/transport';
import type { GoSightConfig } from '../types';

export class ReplayRecorder {
  private transport: Transport;
  private config: Required<GoSightConfig>;
  private stopFn: (() => void) | null = null;
  private events: any[] = [];
  private chunkIndex = 0;

  constructor(transport: Transport, config: Required<GoSightConfig>) {
    this.transport = transport;
    this.config = config;
  }

  start(): void {
    const { privacy, replay } = this.config;

    this.stopFn = record({
      emit: (event, isCheckout) => {
        this.handleEvent(event, isCheckout);
      },

      checkoutEveryNms: replay.checkoutEveryNms,
      checkoutEveryNth: replay.checkoutEveryNth,

      // Privacy
      maskAllInputs: privacy.maskAllInputs,
      maskInputOptions: {
        password: true,
        email: privacy.maskAllInputs,
        tel: privacy.maskAllInputs,
      },

      blockClass: 'gosight-block',
      blockSelector: privacy.blockSelectors?.join(', '),

      maskTextClass: 'gosight-mask',
      maskTextSelector: privacy.maskSelectors?.join(', '),

      // Custom masking
      maskTextFn: (text) => {
        if (privacy.maskAllText) {
          return '█'.repeat(text.length);
        }

        let masked = text;
        for (const regex of privacy.maskRegex || []) {
          masked = masked.replace(regex, match => '█'.repeat(match.length));
        }
        return masked;
      },

      // Performance
      sampling: {
        mousemove: true,
        mouseInteraction: true,
        scroll: 150,
        media: 800,
        input: 'last',
      },

      inlineStylesheet: true,
      inlineImages: false,
      collectFonts: true,
      recordCanvas: false,
    });
  }

  stop(): void {
    if (this.stopFn) {
      this.stopFn();
      this.flush();
    }
  }

  private handleEvent(event: any, isCheckout: boolean): void {
    this.events.push(event);

    // Flush on checkout or buffer full
    if (isCheckout || this.events.length >= 50) {
      this.flush();
    }
  }

  private flush(): void {
    if (this.events.length === 0) return;

    const events = [...this.events];
    this.events = [];

    const hasFullSnapshot = events.some(e => e.type === EventType.FullSnapshot);

    this.transport.sendReplayChunk({
      chunkIndex: this.chunkIndex++,
      timestampStart: events[0].timestamp,
      timestampEnd: events[events.length - 1].timestamp,
      events,
      hasFullSnapshot,
    });
  }
}
```

---

### 3.7 Transport Layer

**`src/transport/transport.ts`**

```typescript
import type { EventPayload, SessionMeta } from '../types';

export class Transport {
  private ingestUrl: string;
  private projectKey: string;

  constructor(ingestUrl: string, projectKey: string) {
    this.ingestUrl = ingestUrl;
    this.projectKey = projectKey;
  }

  async sendEvents(events: EventPayload[], session: SessionMeta): Promise<void> {
    const payload = {
      project_key: this.projectKey,
      session_id: session.sessionId,
      user_id: session.userId,
      device: session.device,
      events,
      sent_at: Date.now(),
    };

    try {
      // Use sendBeacon for reliability
      const blob = new Blob([JSON.stringify(payload)], { type: 'application/json' });
      const sent = navigator.sendBeacon(`${this.ingestUrl}/v1/events`, blob);

      if (!sent) {
        // Fallback to fetch
        await this.fetchWithRetry(`${this.ingestUrl}/v1/events`, payload);
      }
    } catch (error) {
      console.error('[GoSight] Failed to send events:', error);
      // TODO: Queue for retry
    }
  }

  async sendReplayChunk(chunk: any): Promise<void> {
    const compressed = await this.compress(JSON.stringify(chunk.events));

    const payload = {
      project_key: this.projectKey,
      chunk_index: chunk.chunkIndex,
      timestamp_start: chunk.timestampStart,
      timestamp_end: chunk.timestampEnd,
      data: compressed,
      has_full_snapshot: chunk.hasFullSnapshot,
    };

    try {
      await this.fetchWithRetry(`${this.ingestUrl}/v1/replay`, payload);
    } catch (error) {
      console.error('[GoSight] Failed to send replay chunk:', error);
    }
  }

  private async fetchWithRetry(url: string, data: any, retries = 3): Promise<Response> {
    for (let i = 0; i < retries; i++) {
      try {
        const response = await fetch(url, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(data),
          keepalive: true,
        });

        if (response.ok) return response;

        if (response.status >= 400 && response.status < 500) {
          throw new Error(`Client error: ${response.status}`);
        }
      } catch (error) {
        if (i === retries - 1) throw error;
        await this.delay(Math.pow(2, i) * 1000);
      }
    }

    throw new Error('Max retries exceeded');
  }

  private async compress(data: string): Promise<string> {
    if (typeof CompressionStream !== 'undefined') {
      const encoder = new TextEncoder();
      const stream = new CompressionStream('gzip');
      const writer = stream.writable.getWriter();

      writer.write(encoder.encode(data));
      writer.close();

      const compressed = await new Response(stream.readable).arrayBuffer();
      return btoa(String.fromCharCode(...new Uint8Array(compressed)));
    }

    // Fallback: return uncompressed
    return btoa(data);
  }

  private delay(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
```

---

## Checklist

- [ ] Project setup (package.json, tsconfig, rollup)
- [ ] Core GoSight class
- [ ] Session management
- [ ] Event buffer with batching
- [ ] Click event handler
- [ ] Scroll event handler
- [ ] Input event handler
- [ ] Error event handler
- [ ] Performance/Web Vitals handler
- [ ] Replay recorder (rrweb)
- [ ] Privacy masking
- [ ] Transport layer
- [ ] Build NPM package
- [ ] Build CDN bundle
- [ ] Unit tests

## Kết Quả

Sau phase này:
- SDK hoạt động hoàn chỉnh
- Capture tất cả events
- Session replay recording
- Privacy controls
- NPM package và CDN bundle

## Usage Example

```html
<!-- CDN -->
<script src="https://cdn.gosight.io/v1/gosight.iife.js"></script>
<script>
  GoSight.init({
    projectKey: 'gs_xxxxxxxxxxxxxxxx',
    privacy: {
      maskAllInputs: true,
      blockSelectors: ['.payment-form'],
    },
  });

  // Identify user
  GoSight.identify('user-123', { plan: 'pro' });

  // Track custom event
  GoSight.track('purchase', { amount: 99.99 });
</script>
```

```typescript
// NPM
import GoSight from '@gosight/sdk';

GoSight.init({
  projectKey: 'gs_xxxxxxxxxxxxxxxx',
});
```
