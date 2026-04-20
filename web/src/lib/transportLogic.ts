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
