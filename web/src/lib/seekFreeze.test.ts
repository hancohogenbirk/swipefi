import { describe, expect, it } from 'vitest';
import { shouldClearPendingSeek } from './seekFreeze';

const TOL = 3000;

describe('shouldClearPendingSeek', () => {
  it('false when pending is null', () => {
    expect(
      shouldClearPendingSeek({
        pendingMs: null,
        frozenAtTrackId: 1,
        currentTrackId: 1,
        currentPositionMs: 0,
        toleranceMs: TOL,
      }),
    ).toBe(false);
  });

  it('true when track id changed', () => {
    expect(
      shouldClearPendingSeek({
        pendingMs: 180_000,
        frozenAtTrackId: 1,
        currentTrackId: 2,
        currentPositionMs: 0,
        toleranceMs: TOL,
      }),
    ).toBe(true);
  });

  it('true when track unchanged and position within tolerance', () => {
    expect(
      shouldClearPendingSeek({
        pendingMs: 180_000,
        frozenAtTrackId: 1,
        currentTrackId: 1,
        currentPositionMs: 181_000,
        toleranceMs: TOL,
      }),
    ).toBe(true);
  });

  it('false when track unchanged and position outside tolerance', () => {
    expect(
      shouldClearPendingSeek({
        pendingMs: 180_000,
        frozenAtTrackId: 1,
        currentTrackId: 1,
        currentPositionMs: 100_000,
        toleranceMs: TOL,
      }),
    ).toBe(false);
  });

  it('true when frozen-at and current both undefined and position matches', () => {
    expect(
      shouldClearPendingSeek({
        pendingMs: 0,
        frozenAtTrackId: undefined,
        currentTrackId: undefined,
        currentPositionMs: 0,
        toleranceMs: TOL,
      }),
    ).toBe(true);
  });

  it('true when track id transitioned undefined to defined', () => {
    expect(
      shouldClearPendingSeek({
        pendingMs: 0,
        frozenAtTrackId: undefined,
        currentTrackId: 5,
        currentPositionMs: 9999,
        toleranceMs: TOL,
      }),
    ).toBe(true);
  });
});
