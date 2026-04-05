<script lang="ts">
  import { api } from '../api/client';
  import { getPlayerState } from '../stores/player.svelte';

  let seeking = $state(false);
  let seekValue = $state(0);

  let ps = $derived(getPlayerState());
  let positionMs = $derived(seeking ? seekValue : ps.position_ms);
  let durationMs = $derived(ps.duration_ms || 1);
  let progress = $derived(Math.min((positionMs / durationMs) * 100, 100));

  function formatTime(ms: number): string {
    const totalSec = Math.floor(ms / 1000);
    const min = Math.floor(totalSec / 60);
    const sec = totalSec % 60;
    return `${min}:${sec.toString().padStart(2, '0')}`;
  }

  function handlePointerDown(e: PointerEvent) {
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
    seeking = false;
    await api.seek(seekValue);
  }

  function updateSeekValue(e: PointerEvent) {
    const bar = (e.currentTarget as HTMLElement);
    const rect = bar.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    seekValue = Math.floor(ratio * durationMs);
  }
</script>

<div class="progress-container">
  <span class="time elapsed">{formatTime(positionMs)}</span>
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
  <span class="time remaining">-{formatTime(durationMs - positionMs)}</span>
</div>

<style>
  .progress-container {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0 1rem;
  }

  .time {
    font-size: 0.75rem;
    color: #999;
    min-width: 3rem;
    font-variant-numeric: tabular-nums;
  }

  .elapsed {
    text-align: right;
  }

  .remaining {
    text-align: left;
  }

  .progress-bar {
    flex: 1;
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
    background: #1db954;
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
</style>
