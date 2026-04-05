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
  state: 'idle' | 'playing' | 'paused';
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
  scanLibrary: () => request<{ status: string; tracks: number }>('POST', '/api/library/scan'),

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

  // Devices
  devices: () => request<Device[]>('GET', '/api/devices'),
  selectDevice: (udn: string) => request<{ status: string; device: string }>('POST', '/api/devices/select', { udn }),
  scanDevices: () => request<Device[]>('POST', '/api/devices/scan'),
};
