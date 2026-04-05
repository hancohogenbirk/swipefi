<script lang="ts">
  import { api } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';

  let state = $derived(getPlayerState());
  let isPlaying = $derived(state.state === 'playing');

  async function togglePlay() {
    try {
      const newState = isPlaying ? await api.pause() : await api.resume();
      updateState(newState);
    } catch {
      // ignore
    }
  }

  async function skipBack15() {
    const pos = Math.max(0, state.position_ms - 15000);
    try {
      await api.seek(pos);
    } catch {
      // ignore
    }
  }

  async function skipForward15() {
    try {
      await api.seek(state.position_ms + 15000);
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
    <svg viewBox="0 0 24 24" fill="currentColor" width="28" height="28">
      <path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/>
    </svg>
  </button>

  <button class="transport-btn" onclick={skipBack15} aria-label="Back 15 seconds">
    <span class="skip-label">15</span>
    <svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
      <path d="M11.99 5V1l-5 5 5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6h-2c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/>
    </svg>
  </button>

  <button class="play-pause-btn" onclick={togglePlay} aria-label={isPlaying ? 'Pause' : 'Play'}>
    {#if isPlaying}
      <svg viewBox="0 0 24 24" fill="currentColor" width="36" height="36">
        <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
      </svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="currentColor" width="36" height="36">
        <path d="M8 5v14l11-7z"/>
      </svg>
    {/if}
  </button>

  <button class="transport-btn" onclick={skipForward15} aria-label="Forward 15 seconds">
    <span class="skip-label">15</span>
    <svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
      <path d="M18 13c0 3.31-2.69 6-6 6s-6-2.69-6-6h-2c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8V1l-5 5 5 5V7c3.31 0 6 2.69 6 6z"/>
    </svg>
  </button>

  <button class="transport-btn" onclick={next} aria-label="Next">
    <svg viewBox="0 0 24 24" fill="currentColor" width="28" height="28">
      <path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/>
    </svg>
  </button>
</div>

<style>
  .transport {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 1rem;
    padding: 0.5rem 0;
  }

  .transport-btn {
    background: none;
    border: none;
    color: #f0f0f0;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
  }

  .transport-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .skip-label {
    position: absolute;
    font-size: 0.55rem;
    font-weight: 700;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -30%);
    pointer-events: none;
  }

  .play-pause-btn {
    background: #f0f0f0;
    border: none;
    color: #111;
    width: 64px;
    height: 64px;
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
