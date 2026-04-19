export const RESYNC_THRESHOLD_MS = 2000;

export interface InterpolatorState {
  position: number;
  lastTickAt: number | null;
  trackId: number | undefined;
}

export function initial(): InterpolatorState {
  return { position: 0, lastTickAt: null, trackId: undefined };
}

export function applyWsUpdate(
  state: InterpolatorState,
  wsPos: number,
  trackId: number | undefined,
  now: number,
): InterpolatorState {
  const safePos = Math.max(0, wsPos);
  const trackChanged = trackId !== state.trackId;
  const drift = Math.abs(safePos - state.position);
  if (trackChanged || drift > RESYNC_THRESHOLD_MS) {
    return { position: safePos, lastTickAt: now, trackId };
  }
  return { ...state, trackId };
}

export function tickPlaying(state: InterpolatorState, now: number): InterpolatorState {
  if (state.lastTickAt === null) {
    return state;
  }
  const delta = now - state.lastTickAt;
  return { ...state, position: state.position + delta, lastTickAt: now };
}

export function tickIdle(state: InterpolatorState, now: number): InterpolatorState {
  return { ...state, lastTickAt: now };
}

export function computeProgress(positionMs: number, durationMs: number): number {
  if (durationMs <= 0) return 0;
  return Math.max(0, Math.min((positionMs / durationMs) * 100, 100));
}
