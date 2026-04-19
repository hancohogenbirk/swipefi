import { describe, it, expect } from 'vitest';
import {
  initial,
  applyWsUpdate,
  tickPlaying,
  tickIdle,
  RESYNC_THRESHOLD_MS,
} from './progressInterpolator';

describe('progressInterpolator', () => {
  describe('initial', () => {
    it('starts at zero with no track', () => {
      const s = initial();
      expect(s.position).toBe(0);
      expect(s.trackId).toBeUndefined();
    });
  });

  describe('applyWsUpdate — first arrival / track change', () => {
    it('snaps to wsPos when a new track starts', () => {
      let s = initial();
      s = applyWsUpdate(s, 5000, 42, 1000);
      expect(s.position).toBe(5000);
      expect(s.trackId).toBe(42);
    });

    it('snaps to wsPos when switching tracks', () => {
      let s = initial();
      s = applyWsUpdate(s, 0, 1, 0);
      s = tickPlaying(s, 30_000); // bar is at 30s of track 1
      s = applyWsUpdate(s, 0, 2, 30_100); // track 2 starts
      expect(s.position).toBe(0);
      expect(s.trackId).toBe(2);
    });
  });

  describe('applyWsUpdate — small drift (the bug we are fixing)', () => {
    it('does NOT change position for small-drift WS updates', () => {
      let s = initial();
      s = applyWsUpdate(s, 0, 1, 0);
      s = tickPlaying(s, 1000); // position is now 1000 from interpolation
      expect(s.position).toBe(1000);

      // WS arrives reporting 950ms (renderer lagging interpolator by 50ms)
      s = applyWsUpdate(s, 950, 1, 1050);
      expect(s.position).toBe(1000); // unchanged — small drift absorbed
    });

    it('does NOT change position on a sequence of small-drift updates', () => {
      let s = initial();
      s = applyWsUpdate(s, 0, 1, 0);

      // Simulate 10 seconds of playback at 1Hz WS poll. Renderer is a
      // consistent 100ms behind the interpolator (clock skew).
      for (let sec = 1; sec <= 10; sec++) {
        s = tickPlaying(s, sec * 1000);
        const before = s.position;
        s = applyWsUpdate(s, sec * 1000 - 100, 1, sec * 1000 + 50);
        expect(s.position).toBe(before); // never snap back
      }

      // After 10 seconds of playback, position should be ~10000 from
      // interpolation alone, not clamped to the lagging WS value.
      expect(s.position).toBeGreaterThanOrEqual(10_000);
    });

    it('does not snap for jitter within threshold after a seek', () => {
      let s = initial();
      s = applyWsUpdate(s, 0, 1, 0);
      s = tickPlaying(s, 500); // pos=500

      // User seeks +15s
      s = applyWsUpdate(s, 15_500, 1, 550); // drift > threshold → hard sync
      expect(s.position).toBe(15_500);

      // Aggressive polling reports slightly-off values (e.g. renderer rounds)
      s = tickPlaying(s, 750); // pos advances to 15_700
      s = applyWsUpdate(s, 15_650, 1, 760); // drift 50ms — must not snap back
      expect(s.position).toBe(15_700);
    });
  });

  describe('applyWsUpdate — large drift', () => {
    it('snaps when drift exceeds threshold (seek forward)', () => {
      let s = initial();
      s = applyWsUpdate(s, 1000, 1, 0);
      s = tickPlaying(s, 500); // pos = 1500
      const target = 1500 + RESYNC_THRESHOLD_MS + 1000; // well above threshold
      s = applyWsUpdate(s, target, 1, 600);
      expect(s.position).toBe(target);
    });

    it('snaps when drift exceeds threshold (seek backward)', () => {
      let s = initial();
      s = applyWsUpdate(s, 60_000, 1, 0);
      s = tickPlaying(s, 500); // pos = 60_500
      s = applyWsUpdate(s, 5000, 1, 600); // huge backward drift
      expect(s.position).toBe(5000);
    });
  });

  describe('tickPlaying', () => {
    it('advances position by elapsed wall-clock time', () => {
      let s = initial();
      s = applyWsUpdate(s, 0, 1, 1000);
      s = tickPlaying(s, 1016); // +16ms (one frame)
      expect(s.position).toBe(16);
      s = tickPlaying(s, 1032);
      expect(s.position).toBe(32);
    });

    it('does nothing on the very first tick (no lastTickAt yet)', () => {
      // Safety: if tickPlaying is called before any applyWsUpdate, position
      // must not leap to the current clock time.
      let s = initial();
      s = tickPlaying(s, 500_000);
      expect(s.position).toBe(0);
    });
  });

  describe('tickIdle', () => {
    it('does not advance position', () => {
      let s = initial();
      s = applyWsUpdate(s, 10_000, 1, 0);
      s = tickIdle(s, 5000);
      expect(s.position).toBe(10_000);
    });
  });

  describe('pause and resume (no forward jump)', () => {
    it('does not leak paused wall-clock time into position on resume', () => {
      let s = initial();
      s = applyWsUpdate(s, 0, 1, 0);

      // Play for 5 seconds (mix of playing ticks)
      for (let t = 16; t <= 5000; t += 16) {
        s = tickPlaying(s, t);
      }
      expect(s.position).toBeGreaterThan(4900);
      expect(s.position).toBeLessThan(5100);
      const pausedAt = s.position;

      // Pause for 60 seconds (30000ms of idle ticks)
      for (let t = 5016; t <= 65_000; t += 16) {
        s = tickIdle(s, t);
      }
      expect(s.position).toBe(pausedAt); // unchanged during pause

      // Resume — next playing tick should advance by ~16ms, NOT by 60s
      s = tickPlaying(s, 65_016);
      expect(s.position - pausedAt).toBeLessThan(50);
      expect(s.position - pausedAt).toBeGreaterThan(0);
    });
  });
});
