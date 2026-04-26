export type SkipForwardDecision =
  | { kind: 'next' }
  | { kind: 'seek'; positionMs: number };

export function decideSkipForward(
  positionMs: number,
  durationMs: number,
  skipMs: number,
): SkipForwardDecision {
  if (durationMs <= 0) return { kind: 'next' };
  const remaining = durationMs - positionMs;
  if (remaining <= skipMs) return { kind: 'next' };
  return { kind: 'seek', positionMs: positionMs + skipMs };
}

export function decideCoalescedSkip(
  positionMs: number,
  durationMs: number,
  accumulatedSkipMs: number,
): SkipForwardDecision {
  const target = positionMs + accumulatedSkipMs;
  if (durationMs > 0 && target >= durationMs) return { kind: 'next' };
  if (durationMs <= 0 && accumulatedSkipMs > 0) return { kind: 'next' };
  return { kind: 'seek', positionMs: Math.max(0, target) };
}
