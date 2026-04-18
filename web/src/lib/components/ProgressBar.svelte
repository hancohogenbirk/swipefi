<script lang="ts">
  import { onDestroy } from 'svelte';
  import { api } from '../api/client';
  import { getPlayerState } from '../stores/player.svelte';

  const SEEK_SYNC_TOLERANCE_MS = 3000;

  let seeking = $state(false);
  let seekValue = $state(0);
  let pendingSeekMs = $state<number | null>(null);

  let ps = $derived(getPlayerState());
  let idle = $derived(ps.state === 'idle');

  let formatInfo = $derived.by(() => {
    const t = ps.track;
    if (!t) return '';
    const parts: string[] = [];
    if (t.format) parts.push(t.format.toUpperCase());
    if (t.sample_rate_hz) {
      const khz = t.sample_rate_hz / 1000;
      parts.push(Number.isInteger(khz) ? `${khz} kHz` : `${khz.toFixed(1)} kHz`);
    }
    if (t.bit_depth) parts.push(`${t.bit_depth}-bit`);
    if (t.bitrate_kbps) parts.push(`${Math.round(t.bitrate_kbps)} kbps`);
    return parts.join(' \u00B7 ');
  });

  // Interpolation state
  let interpolatedMs = $state(0);
  let lastWsPositionMs = $state(0);
  let lastWsTimestamp = $state(0);
  let rafId: number | null = null;

  // Track when WS position changes
  $effect(() => {
    const wsPos = ps.position_ms;
    if (wsPos !== lastWsPositionMs) {
      lastWsPositionMs = wsPos;
      lastWsTimestamp = performance.now();
      interpolatedMs = wsPos;
    }
  });

  // rAF loop for smooth interpolation
  function tick() {
    if (ps.state === 'playing' && !seeking && pendingSeekMs === null) {
      const elapsed = performance.now() - lastWsTimestamp;
      interpolatedMs = lastWsPositionMs + elapsed;
    }
    rafId = requestAnimationFrame(tick);
  }
  rafId = requestAnimationFrame(tick);
  onDestroy(() => { if (rafId !== null) cancelAnimationFrame(rafId); });

  let positionMs = $derived(
    idle ? 0 :
    ps.state === 'loading' ? 0 :
    seeking ? seekValue :
    pendingSeekMs !== null ? pendingSeekMs :
    interpolatedMs
  );
  let durationMs = $derived(idle ? 1 : (ps.duration_ms || 1));
  let progress = $derived(Math.min((positionMs / durationMs) * 100, 100));

  // Clear pending seek when WS position catches up (within 3s tolerance)
  $effect(() => {
    if (pendingSeekMs !== null && Math.abs(ps.position_ms - pendingSeekMs) < SEEK_SYNC_TOLERANCE_MS) {
      pendingSeekMs = null;
    }
  });

  function formatTime(ms: number): string {
    const totalSec = Math.floor(ms / 1000);
    const min = Math.floor(totalSec / 60);
    const sec = totalSec % 60;
    return `${min}:${sec.toString().padStart(2, '0')}`;
  }

  function handlePointerDown(e: PointerEvent) {
    if (idle) return;
    seeking = true;
    updateSeekValue(e);
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }

  function handlePointerMove(e: PointerEvent) {
    if (!seeking) return;
    updateSeekValue(e);
  }

  async function handlePointerUp() {
    if (!seeking) return;
    const target = seekValue;
    seeking = false;
    pendingSeekMs = target;
    await api.seek(target);
  }

  function updateSeekValue(e: PointerEvent) {
    const bar = (e.currentTarget as HTMLElement);
    const rect = bar.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    seekValue = Math.floor(ratio * durationMs);
  }
</script>

<div class="progress-container" class:disabled={idle}>
  <div
    class="progress-bar"
    role="slider"
    tabindex="0"
    aria-label="Seek"
    aria-valuemin={0}
    aria-valuemax={durationMs}
    aria-valuenow={positionMs}
    onpointerdown={handlePointerDown}
    onpointermove={handlePointerMove}
    onpointerup={handlePointerUp}
  >
    <div class="progress-fill" style="width: {progress}%"></div>
    <div class="progress-thumb" style="left: {progress}%"></div>
  </div>
  <div class="time-row">
    <span class="time elapsed">{formatTime(positionMs)}</span>
    {#if formatInfo}
      <span class="format-info">{formatInfo}</span>
    {/if}
    <span class="time remaining">-{formatTime(durationMs - positionMs)}</span>
  </div>
</div>

<style>
  .progress-container {
    width: 100%;
    padding: 0.5rem 1rem 0 1rem;
  }

  .progress-bar {
    width: 100%;
    height: 24px;
    display: flex;
    align-items: center;
    position: relative;
    cursor: pointer;
    touch-action: none;
  }

  .progress-bar::before {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    height: 4px;
    background: #333;
    border-radius: 2px;
  }

  .progress-fill {
    position: absolute;
    left: 0;
    height: 4px;
    background: linear-gradient(90deg, #1db954, #7cb3ff);
    border-radius: 2px;
    pointer-events: none;
  }

  .progress-thumb {
    position: absolute;
    width: 14px;
    height: 14px;
    background: #fff;
    border-radius: 50%;
    transform: translateX(-50%);
    pointer-events: none;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.5);
  }

  .time-row {
    display: flex;
    justify-content: space-between;
    padding-top: 0.25rem;
  }

  .time {
    font-size: 0.75rem;
    color: #999;
    font-variant-numeric: tabular-nums;
  }

  .format-info {
    font-size: 0.75rem;
    color: #666;
    letter-spacing: 0.02em;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }

  .progress-container.disabled {
    opacity: 0.3;
    pointer-events: none;
  }
</style>
