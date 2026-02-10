# Phase 8: Dashboard

## Mục Tiêu

Xây dựng web dashboard với Next.js.

## Prerequisites

- Phase 7 hoàn thành (API đang chạy)

## Tasks

### 8.1 Project Setup

```bash
npx create-next-app@latest dashboard --typescript --tailwind --eslint --app
cd dashboard
npm install @tanstack/react-query zustand rrweb-player date-fns recharts lucide-react
npm install -D @types/node
```

**Project Structure:**

```
dashboard/
├── src/
│   ├── app/
│   │   ├── (auth)/
│   │   │   ├── login/page.tsx
│   │   │   └── register/page.tsx
│   │   ├── (dashboard)/
│   │   │   ├── layout.tsx
│   │   │   ├── page.tsx
│   │   │   └── [projectId]/
│   │   │       ├── page.tsx
│   │   │       ├── sessions/
│   │   │       │   ├── page.tsx
│   │   │       │   └── [sessionId]/page.tsx
│   │   │       ├── heatmaps/page.tsx
│   │   │       ├── insights/page.tsx
│   │   │       ├── errors/page.tsx
│   │   │       ├── alerts/page.tsx
│   │   │       └── settings/page.tsx
│   │   ├── layout.tsx
│   │   └── globals.css
│   ├── components/
│   │   ├── ui/
│   │   ├── charts/
│   │   ├── replay/
│   │   └── heatmap/
│   ├── hooks/
│   ├── lib/
│   │   ├── api.ts
│   │   └── utils.ts
│   ├── stores/
│   └── types/
├── next.config.js
├── tailwind.config.js
└── package.json
```

---

### 8.2 API Client

**`src/lib/api.ts`**

```typescript
const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

class APIClient {
  private token: string | null = null;

  setToken(token: string) {
    this.token = token;
    localStorage.setItem('token', token);
  }

  getToken(): string | null {
    if (!this.token) {
      this.token = localStorage.getItem('token');
    }
    return this.token;
  }

  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...options.headers,
    };

    if (this.getToken()) {
      headers['Authorization'] = `Bearer ${this.getToken()}`;
    }

    const response = await fetch(`${API_URL}${path}`, {
      ...options,
      headers,
    });

    if (response.status === 401) {
      this.token = null;
      localStorage.removeItem('token');
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }

    const data = await response.json();

    if (!response.ok) {
      throw new Error(data.error || 'Request failed');
    }

    return data;
  }

  // Auth
  async login(email: string, password: string) {
    const data = await this.request<{ token: string; user: User }>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
    this.setToken(data.token);
    return data;
  }

  async register(email: string, password: string, name: string) {
    const data = await this.request<{ token: string; user: User }>('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, name }),
    });
    this.setToken(data.token);
    return data;
  }

  // Projects
  async getProjects() {
    return this.request<{ data: Project[] }>('/api/v1/projects');
  }

  async getProject(id: string) {
    return this.request<{ data: Project }>(`/api/v1/projects/${id}`);
  }

  // Analytics
  async getOverview(projectId: string, startDate: string, endDate: string) {
    return this.request<{ data: OverviewData }>(
      `/api/v1/projects/${projectId}/overview?start_date=${startDate}&end_date=${endDate}`
    );
  }

  // Sessions
  async getSessions(projectId: string, filters: SessionFilters) {
    const params = new URLSearchParams(filters as any);
    return this.request<{ data: Session[]; total: number }>(
      `/api/v1/projects/${projectId}/sessions?${params}`
    );
  }

  async getSession(projectId: string, sessionId: string) {
    return this.request<{ data: SessionDetail }>(
      `/api/v1/projects/${projectId}/sessions/${sessionId}`
    );
  }

  // Replay
  async getReplayMeta(projectId: string, sessionId: string) {
    return this.request<{ data: ReplayMeta }>(
      `/api/v1/projects/${projectId}/sessions/${sessionId}/replay`
    );
  }

  async getReplayChunk(projectId: string, sessionId: string, chunkIndex: number) {
    const response = await fetch(
      `${API_URL}/api/v1/projects/${projectId}/sessions/${sessionId}/replay/chunks/${chunkIndex}`,
      {
        headers: {
          'Authorization': `Bearer ${this.getToken()}`,
        },
      }
    );
    return response.arrayBuffer();
  }

  // Heatmaps
  async getClickHeatmap(projectId: string, path: string, viewportWidth: number) {
    return this.request<{ data: HeatmapPoint[] }>(
      `/api/v1/projects/${projectId}/heatmaps/clicks?path=${encodeURIComponent(path)}&viewport_width=${viewportWidth}`
    );
  }

  // Insights
  async getInsights(projectId: string, type?: string) {
    const params = type ? `?type=${type}` : '';
    return this.request<{ data: Insight[] }>(
      `/api/v1/projects/${projectId}/insights${params}`
    );
  }

  // Errors
  async getErrors(projectId: string) {
    return this.request<{ data: GroupedError[] }>(
      `/api/v1/projects/${projectId}/errors`
    );
  }
}

export const api = new APIClient();
```

---

### 8.3 Dashboard Layout

**`src/app/(dashboard)/layout.tsx`**

```typescript
'use client';

import { useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  LayoutDashboard,
  Users,
  MousePointer,
  AlertTriangle,
  Flame,
  Settings,
  ChevronDown,
} from 'lucide-react';

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [currentProject, setCurrentProject] = useState<Project | null>(null);

  const navigation = [
    { name: 'Overview', href: '', icon: LayoutDashboard },
    { name: 'Sessions', href: '/sessions', icon: Users },
    { name: 'Heatmaps', href: '/heatmaps', icon: MousePointer },
    { name: 'Insights', href: '/insights', icon: Flame },
    { name: 'Errors', href: '/errors', icon: AlertTriangle },
    { name: 'Settings', href: '/settings', icon: Settings },
  ];

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Sidebar */}
      <aside className="fixed inset-y-0 left-0 w-64 bg-white border-r">
        {/* Logo */}
        <div className="h-16 flex items-center px-6 border-b">
          <span className="text-xl font-bold">GoSight</span>
        </div>

        {/* Project Selector */}
        <div className="p-4 border-b">
          <button className="w-full flex items-center justify-between p-2 rounded-lg hover:bg-gray-100">
            <span className="font-medium">{currentProject?.name || 'Select Project'}</span>
            <ChevronDown className="w-4 h-4" />
          </button>
        </div>

        {/* Navigation */}
        <nav className="p-4 space-y-1">
          {navigation.map((item) => {
            const href = currentProject ? `/${currentProject.id}${item.href}` : '#';
            const isActive = pathname.includes(item.href);

            return (
              <Link
                key={item.name}
                href={href}
                className={`flex items-center gap-3 px-3 py-2 rounded-lg ${
                  isActive
                    ? 'bg-blue-50 text-blue-700'
                    : 'text-gray-600 hover:bg-gray-100'
                }`}
              >
                <item.icon className="w-5 h-5" />
                <span>{item.name}</span>
              </Link>
            );
          })}
        </nav>
      </aside>

      {/* Main Content */}
      <main className="ml-64 min-h-screen">
        <div className="p-8">{children}</div>
      </main>
    </div>
  );
}
```

---

### 8.4 Overview Page

**`src/app/(dashboard)/[projectId]/page.tsx`**

```typescript
'use client';

import { useQuery } from '@tanstack/react-query';
import { format, subDays } from 'date-fns';
import { api } from '@/lib/api';
import { StatCard } from '@/components/ui/stat-card';
import { LineChart } from '@/components/charts/line-chart';
import { BarChart } from '@/components/charts/bar-chart';

export default function OverviewPage({ params }: { params: { projectId: string } }) {
  const endDate = format(new Date(), 'yyyy-MM-dd');
  const startDate = format(subDays(new Date(), 7), 'yyyy-MM-dd');

  const { data, isLoading } = useQuery({
    queryKey: ['overview', params.projectId, startDate, endDate],
    queryFn: () => api.getOverview(params.projectId, startDate, endDate),
  });

  if (isLoading) return <div>Loading...</div>;

  const overview = data?.data;

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">Overview</h1>

      {/* Stats Grid */}
      <div className="grid grid-cols-4 gap-6">
        <StatCard
          title="Sessions"
          value={overview?.sessions || 0}
          change={overview?.sessions_change}
        />
        <StatCard
          title="Page Views"
          value={overview?.pageviews || 0}
          change={overview?.pageviews_change}
        />
        <StatCard
          title="Avg Duration"
          value={formatDuration(overview?.avg_duration_seconds || 0)}
        />
        <StatCard
          title="Bounce Rate"
          value={`${(overview?.bounce_rate || 0).toFixed(1)}%`}
        />
      </div>

      {/* Sessions Chart */}
      <div className="bg-white rounded-xl p-6 shadow-sm">
        <h2 className="text-lg font-semibold mb-4">Sessions Over Time</h2>
        <LineChart data={overview?.sessions_by_day || []} />
      </div>

      {/* Two Column Layout */}
      <div className="grid grid-cols-2 gap-6">
        {/* Top Pages */}
        <div className="bg-white rounded-xl p-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-4">Top Pages</h2>
          <table className="w-full">
            <thead>
              <tr className="text-left text-sm text-gray-500">
                <th className="pb-2">Page</th>
                <th className="pb-2 text-right">Views</th>
              </tr>
            </thead>
            <tbody>
              {overview?.top_pages?.map((page) => (
                <tr key={page.path} className="border-t">
                  <td className="py-2 truncate max-w-xs">{page.path}</td>
                  <td className="py-2 text-right">{page.views.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Device Breakdown */}
        <div className="bg-white rounded-xl p-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-4">Devices</h2>
          <BarChart data={overview?.devices || []} />
        </div>
      </div>
    </div>
  );
}
```

---

### 8.5 Sessions List Page

**`src/app/(dashboard)/[projectId]/sessions/page.tsx`**

```typescript
'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { format } from 'date-fns';
import { api } from '@/lib/api';
import { Badge } from '@/components/ui/badge';
import { Pagination } from '@/components/ui/pagination';

export default function SessionsPage({ params }: { params: { projectId: string } }) {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({
    has_error: undefined,
    has_rage_click: undefined,
  });

  const { data, isLoading } = useQuery({
    queryKey: ['sessions', params.projectId, page, filters],
    queryFn: () => api.getSessions(params.projectId, { page, limit: 20, ...filters }),
  });

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Sessions</h1>

        {/* Filters */}
        <div className="flex gap-2">
          <button
            onClick={() => setFilters(f => ({ ...f, has_error: !f.has_error || undefined }))}
            className={`px-3 py-1 rounded-full text-sm ${
              filters.has_error ? 'bg-red-100 text-red-700' : 'bg-gray-100'
            }`}
          >
            Has Error
          </button>
          <button
            onClick={() => setFilters(f => ({ ...f, has_rage_click: !f.has_rage_click || undefined }))}
            className={`px-3 py-1 rounded-full text-sm ${
              filters.has_rage_click ? 'bg-orange-100 text-orange-700' : 'bg-gray-100'
            }`}
          >
            Rage Click
          </button>
        </div>
      </div>

      {/* Sessions Table */}
      <div className="bg-white rounded-xl shadow-sm overflow-hidden">
        <table className="w-full">
          <thead className="bg-gray-50">
            <tr>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">User</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Duration</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Pages</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Entry</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Device</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Flags</th>
              <th className="px-4 py-3 text-left text-sm font-medium text-gray-500">Time</th>
            </tr>
          </thead>
          <tbody>
            {data?.data?.map((session) => (
              <tr
                key={session.session_id}
                onClick={() => router.push(`/${params.projectId}/sessions/${session.session_id}`)}
                className="border-t hover:bg-gray-50 cursor-pointer"
              >
                <td className="px-4 py-3">
                  <span className="font-mono text-sm">
                    {session.user_id || session.session_id.slice(0, 8)}
                  </span>
                </td>
                <td className="px-4 py-3">{formatDuration(session.duration_seconds)}</td>
                <td className="px-4 py-3">{session.page_count}</td>
                <td className="px-4 py-3 truncate max-w-xs">{session.entry_path}</td>
                <td className="px-4 py-3">
                  <span className="text-sm text-gray-500">
                    {session.browser} / {session.os}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <div className="flex gap-1">
                    {session.has_error && <Badge variant="error">Error</Badge>}
                    {session.has_rage_click && <Badge variant="warning">Rage</Badge>}
                  </div>
                </td>
                <td className="px-4 py-3 text-sm text-gray-500">
                  {format(new Date(session.started_at), 'MMM d, HH:mm')}
                </td>
              </tr>
            ))}
          </tbody>
        </table>

        <Pagination
          page={page}
          total={data?.total || 0}
          limit={20}
          onPageChange={setPage}
        />
      </div>
    </div>
  );
}
```

---

### 8.6 Session Replay Page

**`src/app/(dashboard)/[projectId]/sessions/[sessionId]/page.tsx`**

```typescript
'use client';

import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { ReplayPlayer } from '@/components/replay/player';
import { EventTimeline } from '@/components/replay/timeline';
import { SessionInfo } from '@/components/session-info';

export default function SessionDetailPage({
  params,
}: {
  params: { projectId: string; sessionId: string };
}) {
  const { data: session } = useQuery({
    queryKey: ['session', params.sessionId],
    queryFn: () => api.getSession(params.projectId, params.sessionId),
  });

  const { data: replayMeta } = useQuery({
    queryKey: ['replay', params.sessionId],
    queryFn: () => api.getReplayMeta(params.projectId, params.sessionId),
    enabled: session?.data?.has_replay,
  });

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex justify-between items-start">
        <div>
          <h1 className="text-2xl font-bold">Session Replay</h1>
          <p className="text-gray-500">
            {session?.data?.user_id || `Anonymous User`}
          </p>
        </div>
        <SessionInfo session={session?.data} />
      </div>

      {/* Replay Player */}
      <div className="grid grid-cols-3 gap-6">
        <div className="col-span-2">
          {replayMeta?.data && (
            <ReplayPlayer
              projectId={params.projectId}
              sessionId={params.sessionId}
              meta={replayMeta.data}
            />
          )}
        </div>

        {/* Sidebar */}
        <div className="space-y-6">
          {/* Event Timeline */}
          <div className="bg-white rounded-xl p-4 shadow-sm">
            <h3 className="font-semibold mb-4">Events</h3>
            <EventTimeline events={replayMeta?.data?.events || []} />
          </div>

          {/* Insights */}
          <div className="bg-white rounded-xl p-4 shadow-sm">
            <h3 className="font-semibold mb-4">Insights</h3>
            <div className="space-y-2">
              {session?.data?.insights?.map((insight) => (
                <div
                  key={insight.id}
                  className="flex items-center gap-2 p-2 rounded bg-orange-50 text-orange-700"
                >
                  <span>{insight.type}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
```

---

### 8.7 Replay Player Component

**`src/components/replay/player.tsx`**

```typescript
'use client';

import { useEffect, useRef, useState } from 'react';
import { Replayer } from 'rrweb';
import { api } from '@/lib/api';
import { Play, Pause, SkipForward, SkipBack } from 'lucide-react';

interface ReplayPlayerProps {
  projectId: string;
  sessionId: string;
  meta: ReplayMeta;
}

export function ReplayPlayer({ projectId, sessionId, meta }: ReplayPlayerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const replayerRef = useRef<Replayer | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [speed, setSpeed] = useState(1);
  const [loadedChunks, setLoadedChunks] = useState<Set<number>>(new Set());

  useEffect(() => {
    loadInitialChunks();

    return () => {
      replayerRef.current?.destroy();
    };
  }, []);

  async function loadInitialChunks() {
    // Load first chunk
    const data = await loadChunk(0);
    if (!data || !containerRef.current) return;

    // Initialize replayer
    replayerRef.current = new Replayer(data, {
      root: containerRef.current,
      skipInactive: true,
      showWarning: false,
      speed,
      mouseTail: {
        strokeStyle: '#3b82f6',
        lineWidth: 2,
      },
    });

    // Listen for time updates
    replayerRef.current.on('event-cast', () => {
      setCurrentTime(replayerRef.current?.getCurrentTime() || 0);
    });

    // Preload remaining chunks
    preloadChunks();
  }

  async function loadChunk(index: number): Promise<any[] | null> {
    if (loadedChunks.has(index)) return null;

    const buffer = await api.getReplayChunk(projectId, sessionId, index);
    const text = await decompress(buffer);
    const events = JSON.parse(text);

    setLoadedChunks((prev) => new Set([...prev, index]));

    // Add events to replayer if already initialized
    if (replayerRef.current && index > 0) {
      events.forEach((e: any) => replayerRef.current?.addEvent(e));
    }

    return events;
  }

  async function preloadChunks() {
    for (let i = 1; i < meta.chunk_count; i++) {
      await loadChunk(i);
    }
  }

  async function decompress(buffer: ArrayBuffer): Promise<string> {
    const ds = new DecompressionStream('gzip');
    const stream = new Response(buffer).body!.pipeThrough(ds);
    return new Response(stream).text();
  }

  function togglePlay() {
    if (isPlaying) {
      replayerRef.current?.pause();
    } else {
      replayerRef.current?.play();
    }
    setIsPlaying(!isPlaying);
  }

  function seek(time: number) {
    replayerRef.current?.pause();
    replayerRef.current?.play(time);
    setIsPlaying(true);
  }

  function setPlaybackSpeed(s: number) {
    setSpeed(s);
    replayerRef.current?.setConfig({ speed: s });
  }

  return (
    <div className="bg-white rounded-xl shadow-sm overflow-hidden">
      {/* Player Container */}
      <div
        ref={containerRef}
        className="relative bg-gray-100"
        style={{ aspectRatio: '16/9' }}
      />

      {/* Controls */}
      <div className="p-4 border-t">
        <div className="flex items-center gap-4">
          {/* Play/Pause */}
          <button
            onClick={togglePlay}
            className="p-2 rounded-full hover:bg-gray-100"
          >
            {isPlaying ? <Pause className="w-5 h-5" /> : <Play className="w-5 h-5" />}
          </button>

          {/* Skip buttons */}
          <button
            onClick={() => seek(Math.max(0, currentTime - 10000))}
            className="p-2 rounded-full hover:bg-gray-100"
          >
            <SkipBack className="w-5 h-5" />
          </button>
          <button
            onClick={() => seek(currentTime + 10000)}
            className="p-2 rounded-full hover:bg-gray-100"
          >
            <SkipForward className="w-5 h-5" />
          </button>

          {/* Timeline */}
          <div className="flex-1">
            <input
              type="range"
              min={0}
              max={meta.duration}
              value={currentTime}
              onChange={(e) => seek(Number(e.target.value))}
              className="w-full"
            />
          </div>

          {/* Time display */}
          <span className="text-sm text-gray-500">
            {formatTime(currentTime)} / {formatTime(meta.duration)}
          </span>

          {/* Speed */}
          <select
            value={speed}
            onChange={(e) => setPlaybackSpeed(Number(e.target.value))}
            className="text-sm border rounded px-2 py-1"
          >
            <option value={0.5}>0.5x</option>
            <option value={1}>1x</option>
            <option value={2}>2x</option>
            <option value={4}>4x</option>
            <option value={8}>8x</option>
          </select>
        </div>

        {/* Event markers on timeline */}
        <div className="relative h-2 mt-2">
          {meta.events?.map((event, i) => (
            <div
              key={i}
              className={`absolute w-2 h-2 rounded-full cursor-pointer ${
                event.type === 'error' ? 'bg-red-500' :
                event.type === 'rage_click' ? 'bg-orange-500' :
                'bg-blue-500'
              }`}
              style={{ left: `${(event.timestamp / meta.duration) * 100}%` }}
              onClick={() => seek(event.timestamp)}
              title={event.label}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function formatTime(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return `${minutes}:${secs.toString().padStart(2, '0')}`;
}
```

---

### 8.8 Heatmap Page

**`src/app/(dashboard)/[projectId]/heatmaps/page.tsx`**

```typescript
'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@/lib/api';
import { HeatmapOverlay } from '@/components/heatmap/overlay';

export default function HeatmapsPage({ params }: { params: { projectId: string } }) {
  const [selectedPath, setSelectedPath] = useState('/');
  const [viewportWidth, setViewportWidth] = useState(1920);

  const { data: heatmap } = useQuery({
    queryKey: ['heatmap', params.projectId, selectedPath, viewportWidth],
    queryFn: () => api.getClickHeatmap(params.projectId, selectedPath, viewportWidth),
  });

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">Click Heatmap</h1>

        <div className="flex gap-4">
          {/* Path selector */}
          <input
            type="text"
            value={selectedPath}
            onChange={(e) => setSelectedPath(e.target.value)}
            placeholder="Enter page path"
            className="border rounded-lg px-4 py-2"
          />

          {/* Viewport selector */}
          <select
            value={viewportWidth}
            onChange={(e) => setViewportWidth(Number(e.target.value))}
            className="border rounded-lg px-4 py-2"
          >
            <option value={1920}>Desktop (1920px)</option>
            <option value={1366}>Laptop (1366px)</option>
            <option value={768}>Tablet (768px)</option>
            <option value={375}>Mobile (375px)</option>
          </select>
        </div>
      </div>

      {/* Heatmap Visualization */}
      <div className="bg-white rounded-xl p-6 shadow-sm">
        <HeatmapOverlay
          data={heatmap?.data || []}
          viewportWidth={viewportWidth}
        />
      </div>
    </div>
  );
}
```

---

## Checklist

- [ ] Project setup (Next.js, TailwindCSS)
- [ ] API client
- [ ] Auth pages (login, register)
- [ ] Dashboard layout với sidebar
- [ ] Overview page với charts
- [ ] Sessions list với filters
- [ ] Session detail với replay player
- [ ] Replay player component (rrweb)
- [ ] Heatmap page
- [ ] Insights page
- [ ] Errors page
- [ ] Alerts management
- [ ] Settings page
- [ ] Responsive design
- [ ] Dark mode (optional)

## Kết Quả

Sau phase này:
- Full dashboard application
- Session replay player
- Heatmap visualization
- Real-time updates
