export interface ClearPendingSeekArgs {
  pendingMs: number | null;
  frozenAtTrackId: number | undefined;
  currentTrackId: number | undefined;
  currentPositionMs: number;
  toleranceMs: number;
}

export function shouldClearPendingSeek(a: ClearPendingSeekArgs): boolean {
  if (a.pendingMs === null) return false;
  if (a.frozenAtTrackId !== a.currentTrackId) return true;
  return Math.abs(a.currentPositionMs - a.pendingMs) < a.toleranceMs;
}
