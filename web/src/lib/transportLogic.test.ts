import { describe, expect, it } from 'vitest';
import { decideSkipForward, decideCoalescedSkip } from './transportLogic';

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

describe('decideCoalescedSkip', () => {
  const D = 180_000;
  it('zero accumulator is a no-op seek to current position', () => {
    expect(decideCoalescedSkip(30_000, D, 0)).toEqual({ kind: 'seek', positionMs: 30_000 });
  });
  it('single +skip', () => {
    expect(decideCoalescedSkip(30_000, D, 15_000)).toEqual({ kind: 'seek', positionMs: 45_000 });
  });
  it('coalesced 5x+skip', () => {
    expect(decideCoalescedSkip(30_000, D, 75_000)).toEqual({ kind: 'seek', positionMs: 105_000 });
  });
  it('coalesced jump past end returns next', () => {
    expect(decideCoalescedSkip(120_000, D, 75_000)).toEqual({ kind: 'next' });
  });
  it('boundary: target equals duration returns next', () => {
    expect(decideCoalescedSkip(165_000, D, 15_000)).toEqual({ kind: 'next' });
  });
  it('coalesced 3x-skip below zero clamps to 0', () => {
    expect(decideCoalescedSkip(10_000, D, -45_000)).toEqual({ kind: 'seek', positionMs: 0 });
  });
  it('mixed +/- nets to current position', () => {
    expect(decideCoalescedSkip(50_000, D, 0)).toEqual({ kind: 'seek', positionMs: 50_000 });
  });
  it('duration=0 with positive accumulator returns next', () => {
    expect(decideCoalescedSkip(0, 0, 15_000)).toEqual({ kind: 'next' });
  });
  it('duration=0 with zero accumulator returns seek 0 (no-op)', () => {
    expect(decideCoalescedSkip(0, 0, 0)).toEqual({ kind: 'seek', positionMs: 0 });
  });
  it('duration=0 with negative accumulator returns seek 0', () => {
    expect(decideCoalescedSkip(0, 0, -15_000)).toEqual({ kind: 'seek', positionMs: 0 });
  });
});
