<script lang="ts">
  import { tick } from 'svelte';
  import { api, type Track } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';
  import { ArrowLeft, ChevronUp, ChevronDown, Play } from 'lucide-svelte';

  const HOLD_DELAY_MS = 250;
  const HAPTIC_DURATION_MS = 30;
  const DRAG_JITTER_PX = 10;

  let { onBack }: { onBack: () => void } = $props();

  let tracks = $state<Track[]>([]);
  let queuePos = $state(0);
  let loading = $state(true);

  let ps = $derived(getPlayerState());
  let currentTrackId = $derived(ps.track?.id);

  let lastTrackId = $state<number | undefined>(undefined);

  $effect(() => {
    const currentId = ps.track?.id;
    if (currentId !== undefined && currentId !== lastTrackId) {
      lastTrackId = currentId;
      loadQueue();
    }
  });

  // Drag state
  let dragIndex = $state<number | null>(null);
  let dragOverIndex = $state<number | null>(null);
  let holdTimer: ReturnType<typeof setTimeout> | null = null;
  let isDragging = $state(false);
  let touchStartY = $state(0);
  let touchCurrentY = $state(0);
  let itemHeight = 56;
  let dragScrollStart = 0;

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
    scrollToCurrent();
  }

  async function scrollToCurrent() {
    await tick();
    if (!listEl) return;
    const currentEl = listEl.querySelector('.queue-item.current') as HTMLElement;
    if (currentEl) {
      currentEl.scrollIntoView({ block: 'center', behavior: 'instant' });
    }
  }

  async function skipTo(trackId: number) {
    if (isDragging) return;
    try {
      const s = await api.skipTo(trackId);
      updateState(s);
      await loadQueue();
    } catch (e) {
      console.error('[swipefi] skip to failed:', e);
    }
  }

  async function saveOrder() {
    try {
      await api.reorderQueue(tracks.map(t => t.id));
    } catch (e) {
      console.error('[swipefi] reorder failed:', e);
    }
  }

  function moveTrack(fromIdx: number, toIdx: number) {
    if (toIdx < 0 || toIdx >= tracks.length) return;
    const newTracks = [...tracks];
    const [item] = newTracks.splice(fromIdx, 1);
    newTracks.splice(toIdx, 0, item);
    tracks = newTracks;
    return toIdx;
  }

  // --- Touch-based long-press drag ---
  let listEl = $state<HTMLElement | null>(null);
  let autoScrollTimer: ReturnType<typeof setInterval> | null = null;

  function handleTouchStart(e: TouchEvent, idx: number) {
    const touch = e.touches[0];
    touchStartY = touch.clientY;
    touchCurrentY = touch.clientY;

    // Measure item height
    const el = (e.currentTarget as HTMLElement);
    if (el) itemHeight = el.offsetHeight + 1; // +1 for gap

    // Start hold timer for long-press
    holdTimer = setTimeout(() => {
      isDragging = true;
      dragIndex = idx;
      dragScrollStart = listEl?.scrollTop ?? 0;
      if (navigator.vibrate) navigator.vibrate(HAPTIC_DURATION_MS);
    }, HOLD_DELAY_MS);
  }

  function handleTouchMove(e: TouchEvent) {
    const touch = e.touches[0];
    const dy = Math.abs(touch.clientY - touchStartY);

    // Cancel long-press if finger moved too much before hold triggered
    if (!isDragging && dy > DRAG_JITTER_PX) {
      cancelHold();
      return;
    }

    if (!isDragging || dragIndex === null) return;
    e.preventDefault();
    touchCurrentY = touch.clientY;

    // Auto-scroll when dragging near top/bottom edge of visible list
    handleEdgeScroll(touch.clientY);

    // Calculate which index we're hovering over, accounting for container scroll
    const scrollDelta = (listEl?.scrollTop ?? 0) - dragScrollStart;
    const delta = (touchCurrentY - touchStartY) + scrollDelta;
    const indexOffset = Math.round(delta / itemHeight);
    const targetIdx = Math.max(0, Math.min(tracks.length - 1, dragIndex + indexOffset));

    if (targetIdx !== dragIndex) {
      dragIndex = moveTrack(dragIndex, targetIdx)!;
      touchStartY = touchCurrentY;
      dragScrollStart = listEl?.scrollTop ?? 0;
      dragOverIndex = dragIndex;
    }
  }

  function handleEdgeScroll(clientY: number) {
    if (!listEl) return;
    const rect = listEl.getBoundingClientRect();
    const edgeZone = 60; // px from edge to trigger scroll

    stopAutoScroll();
    if (clientY < rect.top + edgeZone) {
      // Near top — scroll up
      autoScrollTimer = setInterval(() => listEl?.scrollBy(0, -8), 16);
    } else if (clientY > rect.bottom - edgeZone) {
      // Near bottom — scroll down
      autoScrollTimer = setInterval(() => listEl?.scrollBy(0, 8), 16);
    }
  }

  function stopAutoScroll() {
    if (autoScrollTimer) {
      clearInterval(autoScrollTimer);
      autoScrollTimer = null;
    }
  }

  function handleTouchEnd() {
    cancelHold();
    stopAutoScroll();
    if (isDragging) {
      isDragging = false;
      dragIndex = null;
      dragOverIndex = null;
      saveOrder();
    }
  }

  function cancelHold() {
    if (holdTimer) {
      clearTimeout(holdTimer);
      holdTimer = null;
    }
  }

  // --- Move up/down buttons (reliable fallback) ---

  function moveUp(idx: number, e: Event) {
    e.stopPropagation();
    if (idx > 0) {
      moveTrack(idx, idx - 1);
      saveOrder();
    }
  }

  function moveDown(idx: number, e: Event) {
    e.stopPropagation();
    if (idx < tracks.length - 1) {
      moveTrack(idx, idx + 1);
      saveOrder();
    }
  }

  loadQueue();
</script>

<div class="queue-view">
  <header class="queue-header">
    <button class="back-btn" onclick={onBack} aria-label="Back">
      <ArrowLeft size={24} />
    </button>
    <h2>Queue</h2>
    <span class="queue-count">{tracks.length} tracks</span>
  </header>

  {#if isDragging}
    <div class="drag-hint">Release to drop</div>
  {:else}
    <div class="drag-hint subtle">Tap to play · Long press to reorder</div>
  {/if}

  {#if loading}
    <div class="loading">Loading queue...</div>
  {:else if tracks.length === 0}
    <div class="empty">Queue is empty</div>
  {:else}
    <div class="queue-list" data-testid="queue-list" bind:this={listEl} class:no-scroll={isDragging}>
      {#each tracks as track, idx (track.id)}
        <div
          class="queue-item"
          class:current={track.id === currentTrackId}
          class:dragging={isDragging && dragIndex === idx}
          class:drag-over={isDragging && dragIndex !== idx && dragOverIndex === idx}
          data-testid="queue-item"
          data-track-id={track.id}
          role="button"
          tabindex="0"
          onclick={() => skipTo(track.id)}
          onkeydown={(e) => { if (e.key === 'Enter') skipTo(track.id); }}
          ontouchstart={(e) => handleTouchStart(e, idx)}
          ontouchmove={handleTouchMove}
          ontouchend={handleTouchEnd}
        >
          <div class="track-indicator">
            {#if track.id === currentTrackId}
              <Play size={14} fill="#4ec484" color="#4ec484" />
            {:else}
              <span class="track-num">{idx + 1}</span>
            {/if}
          </div>

          <div class="track-details">
            <span class="track-title">{track.title}</span>
            <span class="track-meta">
              {track.artist || 'Unknown'}
              {#if track.play_count > 0}
                · {track.play_count}×
              {/if}
            </span>
          </div>

          <div class="move-buttons">
            <button
              class="move-btn"
              onclick={(e) => moveUp(idx, e)}
              disabled={idx === 0}
              aria-label="Move up"
              data-testid="move-up"
            >
              <ChevronUp size={18} />
            </button>
            <button
              class="move-btn"
              onclick={(e) => moveDown(idx, e)}
              disabled={idx === tracks.length - 1}
              aria-label="Move down"
              data-testid="move-down"
            >
              <ChevronDown size={18} />
            </button>
          </div>
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
  }

  .queue-header h2 {
    font-size: 1.1rem;
    margin: 0;
    flex: 1;
  }

  .queue-count {
    font-size: 0.8rem;
    color: var(--color-text-secondary);
  }

  .back-btn {
    background: none;
    border: none;
    color: var(--color-text);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 50%;
  }

  .back-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .drag-hint {
    text-align: center;
    font-size: 0.75rem;
    color: var(--color-accent);
    padding: 0.4rem 0;
    font-weight: 600;
  }

  .drag-hint.subtle {
    color: #555;
    font-weight: 400;
  }

  .queue-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
    -webkit-overflow-scrolling: touch;
  }

  .queue-list.no-scroll {
    overflow-y: hidden;
  }

  .queue-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: var(--color-bg-card);
    border-radius: 10px;
    padding: 0.75rem;
    cursor: pointer;
    transition: transform 0.15s, background 0.15s, box-shadow 0.15s;
    user-select: none;
    touch-action: pan-y;
  }

  .queue-item:hover {
    background: var(--color-bg-hover);
  }

  .queue-item:active {
    background: #252525;
  }

  .queue-item.current {
    background: #1a2e2a;
    border-left: 3px solid var(--color-accent);
  }

  .queue-item.dragging {
    background: #2a2a2a;
    transform: scale(1.03);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
    z-index: 10;
    border-radius: 12px;
    touch-action: none;
  }

  .track-indicator {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.05);
    font-size: 0.8rem;
    color: var(--color-text-secondary);
  }

  .queue-item.current .track-indicator {
    background: rgba(78, 196, 132, 0.2);
  }

  .track-num {
    font-variant-numeric: tabular-nums;
  }

  .track-details {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
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

  .move-buttons {
    display: flex;
    flex-direction: column;
    gap: 0;
    flex-shrink: 0;
  }

  .move-btn {
    background: none;
    border: none;
    color: #666;
    cursor: pointer;
    padding: 0.15rem 0.3rem;
    border-radius: 4px;
    display: flex;
    align-items: center;
    line-height: 1;
  }

  .move-btn:hover:not(:disabled) {
    color: var(--color-text);
    background: rgba(255, 255, 255, 0.1);
  }

  .move-btn:disabled {
    opacity: 0.2;
    cursor: default;
  }

  .loading, .empty {
    text-align: center;
    padding: 2rem;
    color: var(--color-text-secondary);
  }
</style>
