<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api } from '../api/client';
  import { decideCoalescedSkip } from '../transportLogic';
  import { getPlayerState, updateState, setPendingSeekMs } from '../stores/player.svelte';
  import { SkipBack, RotateCcw, Play, Pause, RotateCw, SkipForward } from 'lucide-svelte';

  const SKIP_SECONDS = 15;
  const SKIP_MS = SKIP_SECONDS * 1000;
  const SKIP_DEBOUNCE_MS = 300;

  let ps = $derived(getPlayerState());
  let isPlaying = $derived(ps.state === 'playing' || ps.state === 'loading');
  let idle = $derived(ps.state === 'idle' && !ps.track);

  // Skip coalescing: rapid +/-15 taps accumulate over a 300ms window and
  // fire a single seek (or next, if the cumulative jump lands past end).
  // The anchor is the position at the FIRST tap of a burst, so multi-tap math
  // is consistent regardless of any optimistic UI updates in between.
  let pendingSkipMs = 0;
  let positionAtFirstClick: number | null = null;
  let skipTimer: ReturnType<typeof setTimeout> | null = null;

  async function togglePlay() {
    try {
      const newState = isPlaying ? await api.pause() : await api.resume();
      updateState(newState);
    } catch {
      // ignore
    }
  }

  function queueSkip(deltaMs: number) {
    if (idle) return;
    if (positionAtFirstClick === null) positionAtFirstClick = ps.position_ms;
    pendingSkipMs += deltaMs;
    const duration = ps.duration_ms || 0;
    const optimistic = duration > 0
      ? Math.max(0, Math.min(positionAtFirstClick + pendingSkipMs, duration))
      : Math.max(0, positionAtFirstClick + pendingSkipMs);
    setPendingSeekMs(optimistic);
    if (skipTimer !== null) clearTimeout(skipTimer);
    skipTimer = setTimeout(fireSkip, SKIP_DEBOUNCE_MS);
  }

  async function fireSkip() {
    skipTimer = null;
    const anchor = positionAtFirstClick;
    const acc = pendingSkipMs;
    pendingSkipMs = 0;
    positionAtFirstClick = null;
    if (anchor === null) return;
    const decision = decideCoalescedSkip(anchor, ps.duration_ms, acc);
    if (decision.kind === 'next') {
      await next();
      return;
    }
    setPendingSeekMs(decision.positionMs);
    try {
      await api.seek(decision.positionMs);
    } catch {
      setPendingSeekMs(null);
    }
  }

  onDestroy(() => {
    if (skipTimer !== null) clearTimeout(skipTimer);
  });

  async function prev() {
    setPendingSeekMs(0);
    try {
      const s = await api.prev();
      updateState(s);
    } catch {
      setPendingSeekMs(null);
    }
  }

  async function next() {
    setPendingSeekMs(0);
    try {
      const s = await api.next();
      updateState(s);
    } catch {
      setPendingSeekMs(null);
    }
  }
</script>

<div class="transport" class:disabled={idle}>
  <button class="transport-btn" onclick={prev} aria-label="Previous" disabled={idle}>
    <SkipBack size={28} />
  </button>

  <button class="transport-btn skip-btn" onclick={() => queueSkip(-SKIP_MS)} aria-label="Back 15 seconds" disabled={idle}>
    <RotateCcw size={32} />
    <span class="skip-label">15</span>
  </button>

  <button class="play-pause-btn" onclick={togglePlay} aria-label={isPlaying ? 'Pause' : 'Play'} disabled={idle}>
    {#if isPlaying}
      <Pause size={36} fill="currentColor" />
    {:else}
      <Play size={36} fill="currentColor" />
    {/if}
  </button>

  <button
    class="transport-btn skip-btn"
    onclick={() => queueSkip(SKIP_MS)}
    aria-label="Forward 15 seconds"
    disabled={idle}
  >
    <RotateCw size={32} />
    <span class="skip-label">15</span>
  </button>

  <button class="transport-btn" onclick={next} aria-label="Next" disabled={idle}>
    <SkipForward size={28} />
  </button>
</div>

<style>
  .transport {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    padding: 0.5rem 0;
  }

  .transport-btn {
    background: none;
    border: none;
    color: var(--color-text);
    cursor: pointer;
    padding: 0.6rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .transport-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .transport-btn:disabled {
    opacity: 0.3;
    cursor: default;
  }

  .transport-btn:disabled:hover {
    background: none;
  }

  .skip-btn {
    position: relative;
  }

  .skip-label {
    position: absolute;
    font-size: 0.6rem;
    font-weight: 800;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -15%);
    pointer-events: none;
    line-height: 1;
  }

  .play-pause-btn {
    background: var(--color-text);
    border: none;
    color: var(--color-bg);
    width: 72px;
    height: 72px;
    border-radius: 50%;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .play-pause-btn:hover {
    transform: scale(1.05);
  }

  .transport.disabled {
    opacity: 0.3;
    pointer-events: none;
  }
</style>
