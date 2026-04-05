<script lang="ts">
  import { api } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';
  import SwipeCard from './SwipeCard.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import TransportControls from './TransportControls.svelte';

  let { onBack, onOpenQueue }: { onBack: () => void; onOpenQueue: () => void } = $props();

  let state = $derived(getPlayerState());
  let track = $derived(state.track);

  async function handleSwipeLeft() {
    try {
      console.log('[swipefi] swipe left → reject');
      const s = await api.reject();
      updateState(s);
    } catch (e) {
      console.error('[swipefi] reject failed:', e);
    }
  }

  async function handleSwipeRight() {
    try {
      console.log('[swipefi] swipe right → next');
      const s = await api.next();
      updateState(s);
    } catch (e) {
      console.error('[swipefi] next failed:', e);
    }
  }
</script>

<div class="now-playing">
  <header class="np-header">
    <button class="back-btn" onclick={onBack} aria-label="Back to folders">
      <svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
        <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z"/>
      </svg>
    </button>
    <button class="queue-btn" onclick={onOpenQueue} aria-label="View queue" title="Queue">
      <svg viewBox="0 0 24 24" fill="currentColor" width="22" height="22">
        <path d="M15 6H3v2h12V6zm0 4H3v2h12v-2zM3 16h8v-2H3v2zM17 6v8.18c-.31-.11-.65-.18-1-.18-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3V8h3V6h-5z"/>
      </svg>
      {#if state.queue_length > 0}
        <span class="queue-count">{state.queue_position + 1}/{state.queue_length}</span>
      {/if}
    </button>
  </header>

  <div class="card-area">
    {#if track}
      {#key track.id}
        <SwipeCard
          {track}
          onSwipeLeft={handleSwipeLeft}
          onSwipeRight={handleSwipeRight}
        />
      {/key}
    {:else}
      <div class="idle-message">
        <p>No track playing</p>
        <button class="back-link" onclick={onBack}>Browse folders</button>
      </div>
    {/if}
  </div>

  <div class="controls-area">
    <ProgressBar />
    <TransportControls />
  </div>
</div>

<style>
  .now-playing {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 0.75rem;
  }

  .np-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.25rem 0.5rem;
    margin-bottom: 0.5rem;
  }

  .back-btn {
    background: none;
    border: none;
    color: #f0f0f0;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 50%;
  }

  .back-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .queue-btn {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
    font-size: 0.8rem;
  }

  .queue-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #f0f0f0;
  }

  .queue-count {
    font-variant-numeric: tabular-nums;
  }

  .card-area {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 0;
    padding: 0.5rem;
  }

  .controls-area {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding-bottom: 1rem;
  }

  .idle-message {
    text-align: center;
    color: #666;
  }

  .idle-message p {
    font-size: 1.2rem;
    margin-bottom: 1rem;
  }

  .back-link {
    background: #1db954;
    border: none;
    color: white;
    padding: 0.75rem 1.5rem;
    border-radius: 24px;
    font-size: 1rem;
    cursor: pointer;
  }
</style>
