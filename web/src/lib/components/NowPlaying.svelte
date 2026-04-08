<script lang="ts">
  import { api } from '../api/client';
  import { getPlayerState, updateState, getPlayerLoading } from '../stores/player.svelte';
  import { ListMusic } from 'lucide-svelte';
  import SwipeCard from './SwipeCard.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import TransportControls from './TransportControls.svelte';

  let { onOpenQueue }: { onOpenQueue: () => void } = $props();

  let ps = $derived(getPlayerState());
  let track = $derived(ps.track);
  let playerLoading = $derived(getPlayerLoading());
  let transitioning = $state(false);

  async function handleSwipeLeft() {
    transitioning = true;
    try {
      const s = await api.reject();
      updateState(s);
    } catch (e) {
      console.error('[swipefi] reject failed:', e);
    }
    transitioning = false;
  }

  async function handleSwipeRight() {
    transitioning = true;
    try {
      const s = await api.next();
      updateState(s);
    } catch (e) {
      console.error('[swipefi] next failed:', e);
    }
    transitioning = false;
  }
</script>

<div class="now-playing">
  <header class="np-header">
    <button class="queue-btn" onclick={onOpenQueue} aria-label="View queue" title="Queue">
      <ListMusic size={22} />
      {#if ps.queue_length > 0}
        <span class="queue-count">{ps.queue_position + 1}/{ps.queue_length}</span>
      {/if}
    </button>
  </header>

  <div class="card-area">
    {#if playerLoading && !track}
      <div class="loading-message">
        <div class="loading-spinner"></div>
        <p>Starting playback...</p>
      </div>
    {:else if track && !transitioning}
      {#key track.id}
        <SwipeCard
          {track}
          onSwipeLeft={handleSwipeLeft}
          onSwipeRight={handleSwipeRight}
        />
      {/key}
    {:else if !track}
      <div class="idle-message">
        <p>No track playing</p>
        <p class="idle-hint">Browse your folders to start</p>
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
    justify-content: flex-end;
    padding: 0.25rem 0.5rem;
    margin-bottom: 0.5rem;
  }

  .queue-btn {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: none;
    border: none;
    color: var(--color-text-secondary);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 8px;
    font-size: 0.8rem;
  }

  .queue-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: var(--color-text);
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
    padding: 0.25rem 0.5rem;
    overflow: hidden;
  }

  .controls-area {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding-bottom: 0.25rem;
  }

  .idle-message {
    text-align: center;
    color: #666;
  }

  .idle-message p {
    font-size: 1.2rem;
    margin-bottom: 1rem;
  }

  .idle-hint {
    font-size: 0.9rem;
    color: #555;
  }

  .loading-message {
    text-align: center;
    color: var(--color-text-secondary);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
  }

  .loading-message p {
    font-size: 1rem;
  }

  .loading-spinner {
    width: 40px;
    height: 40px;
    border: 3px solid #333;
    border-top-color: var(--color-accent, #1db954);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
