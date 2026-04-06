<script lang="ts">
  import { api } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';
  import { SkipBack, RotateCcw, Play, Pause, RotateCw, SkipForward } from 'lucide-svelte';

  let ps = $derived(getPlayerState());
  let isPlaying = $derived(ps.state === 'playing');

  async function togglePlay() {
    try {
      const newState = isPlaying ? await api.pause() : await api.resume();
      updateState(newState);
    } catch {
      // ignore
    }
  }

  async function skipBack15() {
    const pos = Math.max(0, ps.position_ms - 15000);
    try {
      await api.seek(pos);
    } catch {
      // ignore
    }
  }

  async function skipForward15() {
    try {
      await api.seek(ps.position_ms + 15000);
    } catch {
      // ignore
    }
  }

  async function prev() {
    try {
      const s = await api.prev();
      updateState(s);
    } catch {
      // ignore
    }
  }

  async function next() {
    try {
      const s = await api.next();
      updateState(s);
    } catch {
      // ignore
    }
  }
</script>

<div class="transport">
  <button class="transport-btn" onclick={prev} aria-label="Previous">
    <SkipBack size={28} />
  </button>

  <button class="transport-btn skip-btn" onclick={skipBack15} aria-label="Back 15 seconds">
    <RotateCcw size={32} />
    <span class="skip-label">15</span>
  </button>

  <button class="play-pause-btn" onclick={togglePlay} aria-label={isPlaying ? 'Pause' : 'Play'}>
    {#if isPlaying}
      <Pause size={36} fill="currentColor" />
    {:else}
      <Play size={36} fill="currentColor" />
    {/if}
  </button>

  <button class="transport-btn skip-btn" onclick={skipForward15} aria-label="Forward 15 seconds">
    <RotateCw size={32} />
    <span class="skip-label">15</span>
  </button>

  <button class="transport-btn" onclick={next} aria-label="Next">
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
    color: #f0f0f0;
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
    background: #f0f0f0;
    border: none;
    color: #111;
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
</style>
