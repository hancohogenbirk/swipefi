<script lang="ts">
  import { tick, onDestroy } from 'svelte';
  import { api, type Track } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';
  import { ArrowLeft, ChevronUp, ChevronDown, Play, GripVertical } from 'lucide-svelte';

  const HAPTIC_DURATION_MS = 30;
  const MOUSE_DRAG_THRESHOLD_PX = 3;

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
  let dragOriginalIndex = $state<number | null>(null);
  let targetIndex = $state<number | null>(null);
  let isDragging = $state(false);
  let dragDeltaY = $state(0);
  let dragOriginY = $state(0);
  let dragScrollStart = 0;
  let itemHeight = 56;

  // Mouse drag: track pending state before threshold is met
  let mouseDownY = 0;
  let mouseDragPending = false;
  let pendingMouseIndex: number | null = null;

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

  let listEl = $state<HTMLElement | null>(null);
  let autoScrollTimer: ReturnType<typeof setInterval> | null = null;

  function measureItemHeight() {
    if (listEl && listEl.children.length >= 2) {
      const first = (listEl.children[0] as HTMLElement).getBoundingClientRect();
      const second = (listEl.children[1] as HTMLElement).getBoundingClientRect();
      itemHeight = second.top - first.top;
    } else if (listEl && listEl.children.length === 1) {
      itemHeight = (listEl.children[0] as HTMLElement).getBoundingClientRect().height + 1;
    }
  }

  function handleEdgeScroll(clientY: number) {
    if (!listEl) return;
    const rect = listEl.getBoundingClientRect();
    const edgeZone = 60;

    stopAutoScroll();

    const distFromTop = clientY - rect.top;
    const distFromBottom = rect.bottom - clientY;

    if (distFromTop < edgeZone) {
      const speed = Math.round(2 + (1 - distFromTop / edgeZone) * 10);
      autoScrollTimer = setInterval(() => listEl?.scrollBy(0, -speed), 16);
    } else if (distFromBottom < edgeZone) {
      const speed = Math.round(2 + (1 - distFromBottom / edgeZone) * 10);
      autoScrollTimer = setInterval(() => listEl?.scrollBy(0, speed), 16);
    }
  }

  function stopAutoScroll() {
    if (autoScrollTimer) {
      clearInterval(autoScrollTimer);
      autoScrollTimer = null;
    }
  }

  // --- Shared drag helpers (used by both touch and mouse) ---

  function startDrag(idx: number, clientY: number) {
    isDragging = true;
    dragOriginalIndex = idx;
    targetIndex = idx;
    dragOriginY = clientY;
    dragScrollStart = listEl?.scrollTop ?? 0;
    dragDeltaY = 0;
    measureItemHeight();
    if (navigator.vibrate) navigator.vibrate(HAPTIC_DURATION_MS);
  }

  function updateDrag(clientY: number) {
    dragDeltaY = clientY - dragOriginY;
    handleEdgeScroll(clientY);
    const scrollDelta = (listEl?.scrollTop ?? 0) - dragScrollStart;
    const delta = dragDeltaY + scrollDelta;
    const indexOffset = Math.round(delta / itemHeight);
    targetIndex = Math.max(0, Math.min(tracks.length - 1, (dragOriginalIndex ?? 0) + indexOffset));
  }

  function finishDrag() {
    stopAutoScroll();
    if (isDragging && dragOriginalIndex !== null && targetIndex !== null) {
      if (dragOriginalIndex !== targetIndex) {
        moveTrack(dragOriginalIndex, targetIndex);
        saveOrder();
      }
    }
    isDragging = false;
    dragOriginalIndex = null;
    targetIndex = null;
    dragDeltaY = 0;
    dragOriginY = 0;
  }

  // --- Touch drag handlers (mobile) ---

  function handleGripTouchStart(e: TouchEvent, idx: number) {
    const touch = e.touches[0];
    startDrag(idx, touch.clientY);
    document.addEventListener('touchmove', handleDocTouchMove, { passive: false });
    document.addEventListener('touchend', handleDocTouchEnd);
    document.addEventListener('touchcancel', handleDocTouchEnd);
  }

  function handleDocTouchMove(e: TouchEvent) {
    if (!isDragging) return;
    e.preventDefault();
    updateDrag(e.touches[0].clientY);
  }

  function handleDocTouchEnd() {
    document.removeEventListener('touchmove', handleDocTouchMove);
    document.removeEventListener('touchend', handleDocTouchEnd);
    document.removeEventListener('touchcancel', handleDocTouchEnd);
    finishDrag();
  }

  // --- Mouse drag handlers (desktop) ---

  function handleGripMouseDown(e: MouseEvent, idx: number) {
    e.preventDefault();
    mouseDownY = e.clientY;
    mouseDragPending = true;
    pendingMouseIndex = idx;
    document.addEventListener('mousemove', handleDocMouseMove);
    document.addEventListener('mouseup', handleDocMouseUp);
  }

  function handleDocMouseMove(e: MouseEvent) {
    if (mouseDragPending && !isDragging) {
      if (Math.abs(e.clientY - mouseDownY) >= MOUSE_DRAG_THRESHOLD_PX) {
        mouseDragPending = false;
        startDrag(pendingMouseIndex!, mouseDownY);
      } else {
        return;
      }
    }
    if (!isDragging) return;
    e.preventDefault();
    updateDrag(e.clientY);
  }

  function handleDocMouseUp() {
    document.removeEventListener('mousemove', handleDocMouseMove);
    document.removeEventListener('mouseup', handleDocMouseUp);
    mouseDragPending = false;
    pendingMouseIndex = null;
    finishDrag();
  }

  // --- Cleanup ---

  onDestroy(() => {
    document.removeEventListener('touchmove', handleDocTouchMove);
    document.removeEventListener('touchend', handleDocTouchEnd);
    document.removeEventListener('touchcancel', handleDocTouchEnd);
    document.removeEventListener('mousemove', handleDocMouseMove);
    document.removeEventListener('mouseup', handleDocMouseUp);
    stopAutoScroll();
  });

  /** Compute inline style for an item during drag */
  function getItemStyle(idx: number): string {
    if (!isDragging || dragOriginalIndex === null || targetIndex === null) return '';

    if (idx === dragOriginalIndex) {
      // Dragged item follows the finger — no CSS transition
      return `transform: translateY(${dragDeltaY}px); z-index: 10;`;
    }

    // Shift other items to create a visual gap at the drop target
    if (dragOriginalIndex < targetIndex) {
      // Dragging down: items between original+1 and target shift up
      if (idx > dragOriginalIndex && idx <= targetIndex) {
        return `transform: translateY(${-itemHeight}px);`;
      }
    } else if (dragOriginalIndex > targetIndex) {
      // Dragging up: items between target and original-1 shift down
      if (idx >= targetIndex && idx < dragOriginalIndex) {
        return `transform: translateY(${itemHeight}px);`;
      }
    }
    return '';
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
    <div class="drag-hint subtle">Drag ≡ to reorder · Tap to play</div>
  {/if}

  {#if loading}
    <div class="loading">Loading queue...</div>
  {:else if tracks.length === 0}
    <div class="empty">Queue is empty</div>
  {:else}
    <div
      class="queue-list"
      class:dragging-active={isDragging}
      data-testid="queue-list"
      bind:this={listEl}
    >
      {#each tracks as track, idx (track.id)}
        <div
          class="queue-item"
          class:current={track.id === currentTrackId}
          class:dragging={isDragging && dragOriginalIndex === idx}
          data-testid="queue-item"
          data-track-id={track.id}
          role="button"
          tabindex="0"
          style={getItemStyle(idx)}
          onclick={() => skipTo(track.id)}
          onkeydown={(e) => { if (e.key === 'Enter') skipTo(track.id); }}
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

          <button
            type="button"
            class="drag-handle"
            data-testid="drag-handle"
            aria-label="Reorder"
            onclick={(e) => e.stopPropagation()}
            ontouchstart={(e) => handleGripTouchStart(e, idx)}
            onmousedown={(e) => handleGripMouseDown(e, idx)}
          >
            <GripVertical size={20} />
          </button>

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

  .queue-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: var(--color-bg-card);
    border-radius: 10px;
    padding: 0.75rem;
    cursor: pointer;
    transition: transform 0.15s ease, background 0.15s, box-shadow 0.15s;
    user-select: none;
  }

  /* During active drag: non-dragged items animate transforms smoothly */
  .queue-list.dragging-active .queue-item:not(.dragging) {
    transition: transform 0.15s ease, background 0.15s, box-shadow 0.15s;
  }

  /* Dragged item: no transform transition, follows finger directly */
  .queue-list.dragging-active .queue-item.dragging {
    transition: background 0.15s, box-shadow 0.15s;
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

  .drag-handle {
    touch-action: none;
    cursor: grab;
    color: #555;
    padding: 0.5rem;
    display: flex;
    align-items: center;
    flex-shrink: 0;
    border-radius: 4px;
    background: none;
    border: none;
  }

  .drag-handle:active {
    cursor: grabbing;
    color: var(--color-text);
  }

  .queue-item.dragging .drag-handle {
    cursor: grabbing;
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
