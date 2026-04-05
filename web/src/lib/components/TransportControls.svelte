<script lang="ts">
  import { api } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';

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
    <svg viewBox="0 0 24 24" fill="currentColor" width="32" height="32">
      <path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/>
    </svg>
  </button>

  <button class="transport-btn skip-btn" onclick={skipBack15} aria-label="Back 15 seconds">
    <svg viewBox="0 0 24 24" fill="currentColor" width="36" height="36">
      <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
    </svg>
    <span class="skip-label">15</span>
  </button>

  <button class="play-pause-btn" onclick={togglePlay} aria-label={isPlaying ? 'Pause' : 'Play'}>
    {#if isPlaying}
      <svg viewBox="0 0 24 24" fill="currentColor" width="40" height="40">
        <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
      </svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="currentColor" width="40" height="40">
        <path d="M8 5v14l11-7z"/>
      </svg>
    {/if}
  </button>

  <button class="transport-btn skip-btn" onclick={skipForward15} aria-label="Forward 15 seconds">
    <svg viewBox="0 0 24 24" fill="currentColor" width="36" height="36" style="transform: scaleX(-1);">
      <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
    </svg>
    <span class="skip-label">15</span>
  </button>

  <button class="transport-btn" onclick={next} aria-label="Next">
    <svg viewBox="0 0 24 24" fill="currentColor" width="32" height="32">
      <path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/>
    </svg>
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
    font-size: 0.7rem;
    font-weight: 800;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -20%);
    pointer-events: none;
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
