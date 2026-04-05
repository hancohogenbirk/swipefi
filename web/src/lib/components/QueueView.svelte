<script lang="ts">
  import { api, type Track } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';

  let { onBack }: { onBack: () => void } = $props();

  let tracks = $state<Track[]>([]);
  let queuePos = $state(0);
  let loading = $state(true);

  // Drag state
  let dragIdx = $state<number | null>(null);
  let overIdx = $state<number | null>(null);

  let ps = $derived(getPlayerState());
  let currentTrackId = $derived(ps.track?.id);

  async function loadQueue() {
    loading = true;
    try {
      const q = await api.queue();
      tracks = q.tracks ?? [];
      queuePos = q.position;
    } catch {
      tracks = [];
    } finally {
      loading = false;
    }
  }

  async function skipTo(trackId: number) {
    try {
      const s = await api.skipTo(trackId);
      updateState(s);
      await loadQueue();
    } catch (e) {
      console.error('[swipefi] skip to failed:', e);
    }
  }

  async function saveOrder() {
    const ids = tracks.map(t => t.id);
    try {
      await api.reorderQueue(ids);
    } catch (e) {
      console.error('[swipefi] reorder failed:', e);
    }
  }

  // Touch drag-and-drop
  let touchStartY = 0;
  let touchItemHeight = 0;

  function handleDragStart(idx: number) {
    dragIdx = idx;
  }

  function handleDragOver(idx: number) {
    if (dragIdx === null || dragIdx === idx) return;
    overIdx = idx;

    // Reorder in place
    const item = tracks[dragIdx];
    const newTracks = [...tracks];
    newTracks.splice(dragIdx, 1);
    newTracks.splice(idx, 0, item);
    tracks = newTracks;
    dragIdx = idx;
  }

  function handleDragEnd() {
    if (dragIdx !== null) {
      saveOrder();
    }
    dragIdx = null;
    overIdx = null;
  }

  // Touch-based reordering
  function handleTouchStart(e: TouchEvent, idx: number) {
    dragIdx = idx;
    touchStartY = e.touches[0].clientY;
    const el = (e.currentTarget as HTMLElement).closest('.queue-item') as HTMLElement;
    if (el) touchItemHeight = el.offsetHeight;
  }

  function handleTouchMove(e: TouchEvent) {
    if (dragIdx === null) return;
    e.preventDefault();
    const deltaY = e.touches[0].clientY - touchStartY;
    const offset = Math.round(deltaY / (touchItemHeight || 56));
    const newIdx = Math.max(0, Math.min(tracks.length - 1, dragIdx + offset));
    if (newIdx !== dragIdx) {
      handleDragOver(newIdx);
      touchStartY = e.touches[0].clientY;
    }
  }

  function handleTouchEnd() {
    handleDragEnd();
  }

  function formatDuration(ms: number): string {
    if (!ms) return '';
    const totalSec = Math.floor(ms / 1000);
    const min = Math.floor(totalSec / 60);
    const sec = totalSec % 60;
    return `${min}:${sec.toString().padStart(2, '0')}`;
  }

  loadQueue();
</script>

<div class="queue-view">
  <header class="queue-header">
    <button class="back-btn" onclick={onBack} aria-label="Back">
      <svg viewBox="0 0 24 24" fill="currentColor" width="24" height="24">
        <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z"/>
      </svg>
    </button>
    <h2>Queue</h2>
    <span class="queue-count">{tracks.length} tracks</span>
  </header>

  {#if loading}
    <div class="loading">Loading queue...</div>
  {:else if tracks.length === 0}
    <div class="empty">Queue is empty</div>
  {:else}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="queue-list"
      ontouchmove={handleTouchMove}
      ontouchend={handleTouchEnd}
    >
      {#each tracks as track, idx (track.id)}
        <div
          class="queue-item"
          class:current={track.id === currentTrackId}
          class:dragging={dragIdx === idx}
          class:past={idx < queuePos}
        >
          <button class="skip-to-btn" onclick={() => skipTo(track.id)} title="Skip to this track">
            {#if track.id === currentTrackId}
              <span class="now-indicator">▶</span>
            {:else}
              <span class="track-num">{idx + 1}</span>
            {/if}
          </button>

          <div class="track-details">
            <span class="track-title">{track.title}</span>
            <span class="track-meta">
              {track.artist || 'Unknown'}
              {#if track.play_count > 0}
                · {track.play_count} play{track.play_count !== 1 ? 's' : ''}
              {/if}
            </span>
          </div>

          <button
            class="drag-handle"
            aria-label="Reorder"
            draggable="true"
            ondragstart={() => handleDragStart(idx)}
            ondragover={(e) => { e.preventDefault(); handleDragOver(idx); }}
            ondragend={handleDragEnd}
            ontouchstart={(e) => handleTouchStart(e, idx)}
          >
            <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
              <path d="M3 15h18v-2H3v2zm0 4h18v-2H3v2zm0-8h18V9H3v2zm0-6v2h18V5H3z"/>
            </svg>
          </button>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .queue-view {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 0.75rem;
  }

  .queue-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.25rem 0.5rem;
    margin-bottom: 0.75rem;
  }

  .queue-header h2 {
    font-size: 1.1rem;
    margin: 0;
    flex: 1;
  }

  .queue-count {
    font-size: 0.8rem;
    color: #888;
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

  .queue-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
    touch-action: pan-y;
  }

  .queue-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: #1a1a1a;
    border-radius: 8px;
    padding: 0.6rem 0.75rem;
    transition: background 0.15s;
  }

  .queue-item.current {
    background: #1a2e1a;
    border-left: 3px solid #1db954;
  }

  .queue-item.dragging {
    opacity: 0.5;
    background: #252525;
  }

  .queue-item.past {
    opacity: 0.4;
  }

  .skip-to-btn {
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    border-radius: 50%;
    font-size: 0.8rem;
  }

  .skip-to-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #f0f0f0;
  }

  .now-indicator {
    color: #1db954;
    font-size: 0.7rem;
  }

  .track-num {
    font-variant-numeric: tabular-nums;
  }

  .track-details {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .track-title {
    font-size: 0.9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .track-meta {
    font-size: 0.75rem;
    color: #666;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .drag-handle {
    background: none;
    border: none;
    color: #555;
    cursor: grab;
    padding: 0.4rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    touch-action: none;
  }

  .drag-handle:hover {
    color: #aaa;
    background: rgba(255, 255, 255, 0.05);
  }

  .drag-handle:active {
    cursor: grabbing;
  }

  .loading, .empty {
    text-align: center;
    padding: 2rem;
    color: #888;
  }
</style>
