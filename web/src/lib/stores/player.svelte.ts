import { api, type PlayerState } from '../api/client';

const WS_RECONNECT_DELAY_MS = 2000;

let state = $state<PlayerState>({
  state: 'idle',
  position_ms: 0,
  duration_ms: 0,
  queue_length: 0,
  queue_position: 0,
});

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

export function getPlayerState(): PlayerState {
  return state;
}

export function updateState(newState: PlayerState) {
  state = newState;
}

export function connectWebSocket() {
  if (ws?.readyState === WebSocket.OPEN) return;

  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(`${proto}//${location.host}/ws`);

  ws.onmessage = (event) => {
    try {
      const newState: PlayerState = JSON.parse(event.data);
      state = newState;
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
}

export function disconnectWebSocket() {
  if (reconnectTimer) clearTimeout(reconnectTimer);
  ws?.close();
  ws = null;
}

export async function loadInitialState() {
  try {
    const s = await api.playerState();
    state = s;
  } catch {
    // Server might not be ready yet
  }
}
