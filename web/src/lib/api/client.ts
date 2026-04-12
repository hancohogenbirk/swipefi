const BASE = '';

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const opts: RequestInit = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (body) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(`${BASE}${path}`, opts);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

export interface Track {
  id: number;
  path: string;
  title: string;
  artist: string;
  album: string;
  duration_ms: number;
  format: string;
  play_count: number;
  added_at: number;
  last_played?: number;
  sample_rate_hz?: number;
  bit_depth?: number;
  bitrate_kbps?: number;
  transcode_score?: number;
  transcode_source?: string;
}

export interface Folder {
  name: string;
  path: string;
}

export interface Device {
  name: string;
  udn: string;
  location: string;
}

export interface PlayerState {
  state: 'idle' | 'loading' | 'playing' | 'paused';
  connected: boolean;
  reconnecting?: boolean;
  track?: Track;
  position_ms: number;
  duration_ms: number;
  queue_length: number;
  queue_position: number;
}

export const api = {
  // Library
  folders: (path = '') => request<Folder[]>('GET', `/api/folders?path=${encodeURIComponent(path)}`),
  tracks: (folder: string, sort = 'added_at', order = 'asc') =>
    request<Track[]>('GET', `/api/tracks?folder=${encodeURIComponent(folder)}&sort=${sort}&order=${order}`),
  tracksDirectOnly: (folder: string, sort = 'added_at', order = 'asc') =>
    request<Track[]>('GET', `/api/tracks?folder=${encodeURIComponent(folder)}&sort=${sort}&order=${order}&direct=true`),
  scanLibrary: () => request<{ status: string; tracks: number }>('POST', '/api/library/scan'),
  rescanLibrary: () => request<{ status: string }>('POST', '/api/library/rescan'),
  scanStatus: () => request<{ scanning: boolean; scanned: number; total: number; phase: string }>('GET', '/api/library/scan/status'),

  // Player
  play: (folder: string, sort: string, order: string) =>
    request<PlayerState>('POST', '/api/player/play', { folder, sort, order }),
  pause: () => request<PlayerState>('POST', '/api/player/pause'),
  resume: () => request<PlayerState>('POST', '/api/player/resume'),
  next: () => request<PlayerState>('POST', '/api/player/next'),
  prev: () => request<PlayerState>('POST', '/api/player/prev'),
  seek: (position_ms: number) => request<PlayerState>('POST', '/api/player/seek', { position_ms }),
  reject: () => request<PlayerState>('POST', '/api/player/reject'),
  playerState: () => request<PlayerState>('GET', '/api/player/state'),
  queue: () => request<{ tracks: Track[]; position: number }>('GET', '/api/player/queue'),
  reorderQueue: (ids: number[]) => request<{ status: string }>('POST', '/api/player/queue/reorder', { ids }),
  skipTo: (track_id: number) => request<PlayerState>('POST', '/api/player/queue/skip-to', { track_id }),

  // Config
  config: () => request<{ music_dir: string; delete_dir: string; connected_device: string }>('GET', '/api/config'),
  setMusicDir: (path: string) =>
    request<{ status: string; music_dir: string; delete_dir: string }>('POST', '/api/config/music-dir', { path }),
  shortcuts: () => request<{ name: string; path: string }[]>('GET', '/api/browse/shortcuts'),
  browse: (path = '/') =>
    request<{ current: string; parent: string; entries: { name: string; path: string; is_dir: boolean }[] }>(
      'GET', `/api/browse?path=${encodeURIComponent(path)}`
    ),

  // Devices
  devices: () => request<Device[]>('GET', '/api/devices'),
  selectDevice: (udn: string) => request<{ status: string; device: string }>('POST', '/api/devices/select', { udn }),
  disconnectDevice: () => request<{ status: string }>('POST', '/api/devices/disconnect'),
  scanDevices: () => request<Device[]>('POST', '/api/devices/scan'),

  // Deleted tracks
  listDeleted: () => request<Track[]>('GET', '/api/deleted'),
  restoreDeleted: (ids: number[]) =>
    request<{ status: string; restored?: number; errors?: string[] }>('POST', '/api/deleted/restore', { ids }),
  purgeDeleted: (ids: number[], all = false) =>
    request<{ status: string; purged?: number }>('POST', '/api/deleted/purge', all ? { all: true } : { ids }),
  deletedProcessing: () =>
    request<{ active: boolean; operation?: string; total?: number; completed?: number; errors?: string[] }>('GET', '/api/deleted/processing'),
};
