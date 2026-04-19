import { api, type PlayerState } from '../api/client';

const WS_RECONNECT_DELAY_MS = 2000;
const WS_FALLBACK_TIMEOUT_MS = 1500;

let state = $state<PlayerState>({
  state: 'idle',
  connected: false,
  position_ms: 0,
  duration_ms: 0,
  queue_length: 0,
  queue_position: 0,
});

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let lastMessageAt = 0;

let pendingSeekMs = $state<number | null>(null);

export function getPlayerState(): PlayerState {
  return state;
}

export function updateState(newState: PlayerState) {
  state = newState;
}

export function getLastMessageAt(): number {
  return lastMessageAt;
}

export function getPendingSeekMs(): number | null {
  return pendingSeekMs;
}

export function setPendingSeekMs(v: number | null) {
  pendingSeekMs = v;
}

let visibilityHandlerSet = false;

export function setupVisibilityHandler() {
  if (visibilityHandlerSet) return;
  visibilityHandlerSet = true;
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible') return;

    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
    if (ws) {
      ws.onclose = null; // prevent cascading reconnects
      ws.close();
      ws = null;
    }

    const beforeAt = lastMessageAt;
    connectWebSocket();

    // Only fall back to HTTP if WS hasn't delivered a fresh message in time.
    setTimeout(() => {
      if (lastMessageAt === beforeAt) {
        loadInitialState();
      }
    }, WS_FALLBACK_TIMEOUT_MS);
  });
}

export function connectWebSocket() {
  if (ws?.readyState === WebSocket.OPEN) return;

  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(`${proto}//${location.host}/ws`);

  ws.onmessage = (event) => {
    try {
      const newState: PlayerState = JSON.parse(event.data);
      state = newState;
      lastMessageAt = Date.now();
    } catch {
      // ignore parse errors
    }
  };

  ws.onclose = () => {
    ws = null;
    // Reconnect after 2 seconds and re-fetch state
    reconnectTimer = setTimeout(async () => {
      connectWebSocket();
      await loadInitialState();
    }, WS_RECONNECT_DELAY_MS);
  };

  ws.onerror = () => {
    ws?.close();
  };

  setupVisibilityHandler();
}

export function disconnectWebSocket() {
  if (reconnectTimer) clearTimeout(reconnectTimer);
  ws?.close();
  ws = null;
}

export async function loadInitialState() {
  try {
    const s = await api.playerState();
    updateState(s);
  } catch {
    // Server might not be ready yet
  }
}
