import { describe, expect, it } from 'vitest';
import { decideSkipForward } from './transportLogic';

const SKIP_MS = 15_000;

describe('decideSkipForward', () => {
  it('returns seek with positionMs+skip when remaining is well above skipMs', () => {
    expect(decideSkipForward(30_000, 180_000, SKIP_MS)).toEqual({
      kind: 'seek',
      positionMs: 45_000,
    });
  });

  it('returns next at the boundary (remaining === skipMs)', () => {
    expect(decideSkipForward(85_000, 100_000, SKIP_MS)).toEqual({ kind: 'next' });
  });

  it('returns next when remaining is below skipMs', () => {
    expect(decideSkipForward(95_000, 100_000, SKIP_MS)).toEqual({ kind: 'next' });
  });

  it('returns next when durationMs is 0 (unknown duration)', () => {
    expect(decideSkipForward(0, 0, SKIP_MS)).toEqual({ kind: 'next' });
  });

  it('returns next at exact end of track', () => {
    expect(decideSkipForward(100_000, 100_000, SKIP_MS)).toEqual({ kind: 'next' });
  });

  it('returns seek when one millisecond above the boundary', () => {
    expect(decideSkipForward(84_999, 100_000, SKIP_MS)).toEqual({
      kind: 'seek',
      positionMs: 99_999,
    });
  });
});
