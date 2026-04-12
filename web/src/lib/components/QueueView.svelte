<script lang="ts">
  import { tick, onDestroy } from 'svelte';
  import { api, type Track } from '../api/client';
  import { getPlayerState, updateState } from '../stores/player.svelte';
  import { getSort } from '../stores/library.svelte';
  import { ArrowLeft, ChevronUp, ChevronDown, Play, GripVertical, Clock, Trash2, X } from 'lucide-svelte';

  const HAPTIC_DURATION_MS = 30;
  const MOUSE_DRAG_THRESHOLD_PX = 3;

  let { onBack }: { onBack: () => void } = $props();

  let tracks = $state<Track[]>([]);
  let queuePos = $state(0);
  let loading = $state(true);
  let queueFolder = $state('');
  let queueSortBy = $state('');
  let queueSortOrder = $state('');

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
  let dragScrollOffset = $state(0);
  let dragScrollStart = 0;
  let itemHeight = 56;

  // Mouse drag: track pending state before threshold is met
  let mouseDownY = 0;
  let mouseDragPending = false;
  let pendingMouseIndex: number | null = null;

  // Swipe state
  let swipeIdx = $state<number | null>(null);
  let swipeDeltaX = $state(0);
  let swipeDragging = $state(false);
  let swipeSwiping = $state(false);
  let swipeDirection = $state<'left' | 'right' | null>(null);
  let swipeStartX = $state(0);
  let swipeStartY = $state(0);
  let swipeLocked = $state<'horizontal' | 'vertical' | null>(null);
  let collapsingId = $state<number | null>(null);

  const SWIPE_THRESHOLD = 80;
  const SWIPE_LOCK_THRESHOLD = 10;

  async function loadQueue() {
    // Only show loading spinner on initial load (no tracks yet)
    const isInitial = tracks.length === 0;
    if (isInitial) loading = true;
    try {
      const q = await api.queue();
      tracks = q.tracks ?? [];
      queuePos = q.position;
      queueFolder = q.folder ?? '';
      queueSortBy = q.sort_by ?? '';
      queueSortOrder = q.sort_order ?? '';
    } catch {
      tracks = [];
    } finally {
      loading = false;
    }
    scrollToCurrent();
  }

  function formatSortLabel(sortBy: string, sortOrder: string): string {
    const labels: Record<string, string> = {
      'added_at': 'Date added',
      'play_count': 'Play count',
      'last_played': 'Last played',
    };
    const label = labels[sortBy] || sortBy;
    const arrow = sortOrder === 'asc' ? '\u2191' : '\u2193';
    return `${label} ${arrow}`;
  }

  function formatDate(timestamp: number): string {
    const d = new Date(timestamp * 1000);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffDays === 0) return 'Today';
    if (diffDays === 1) return 'Yesterday';
    if (diffDays < 7) return `${diffDays}d ago`;

    const month = d.toLocaleDateString('en', { month: 'short' });
    const day = d.getDate();
    if (d.getFullYear() !== now.getFullYear()) {
      return `${month} '${String(d.getFullYear()).slice(2)}`;
    }
    return `${month} ${day}`;
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
      autoScrollTimer = setInterval(() => {
        if (!listEl || listEl.scrollTop <= 0) { stopAutoScroll(); return; }
        listEl.scrollBy(0, -speed);
        updateScrollOffset();
      }, 16);
    } else if (distFromBottom < edgeZone) {
      const speed = Math.round(2 + (1 - distFromBottom / edgeZone) * 10);
      autoScrollTimer = setInterval(() => {
        if (!listEl || listEl.scrollTop >= listEl.scrollHeight - listEl.clientHeight) { stopAutoScroll(); return; }
        listEl.scrollBy(0, speed);
        updateScrollOffset();
      }, 16);
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
    dragScrollOffset = 0;
    dragDeltaY = 0;
    measureItemHeight();
    if (navigator.vibrate) navigator.vibrate(HAPTIC_DURATION_MS);
  }

  function updateScrollOffset() {
    dragScrollOffset = (listEl?.scrollTop ?? 0) - dragScrollStart;
    // Recompute target index with updated scroll
    const delta = dragDeltaY + dragScrollOffset;
    const indexOffset = Math.round(delta / itemHeight);
    targetIndex = Math.max(0, Math.min(tracks.length - 1, (dragOriginalIndex ?? 0) + indexOffset));
  }

  function updateDrag(clientY: number) {
    dragDeltaY = clientY - dragOriginY;
    handleEdgeScroll(clientY);
    updateScrollOffset();
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
    dragScrollOffset = 0;
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
    document.removeEventListener('mousemove', handleSwipeMouseMove);
    document.removeEventListener('mouseup', handleSwipeMouseUp);
    stopAutoScroll();
  });

  /** Compute inline style for the swipe-container during drag reorder */
  function getDragStyle(idx: number): string {
    if (!isDragging || dragOriginalIndex === null || targetIndex === null) return '';

    if (idx === dragOriginalIndex) {
      // Clamp so the dragged item can't go past the first or last position
      const minY = -dragOriginalIndex * itemHeight;
      const maxY = (tracks.length - 1 - dragOriginalIndex) * itemHeight;
      const rawY = dragDeltaY + dragScrollOffset;
      const clampedY = Math.max(minY, Math.min(maxY, rawY));
      return `transform: translateY(${clampedY}px); z-index: 10;`;
    }

    if (dragOriginalIndex < targetIndex) {
      if (idx > dragOriginalIndex && idx <= targetIndex) {
        return `transform: translateY(${-itemHeight}px);`;
      }
    } else if (dragOriginalIndex > targetIndex) {
      if (idx >= targetIndex && idx < dragOriginalIndex) {
        return `transform: translateY(${itemHeight}px);`;
      }
    }
    return '';
  }

  /** Compute inline style for the queue-item during swipe */
  function getSwipeStyle(idx: number): string {
    if (swipeIdx === idx && (swipeDragging || swipeSwiping)) {
      const x = swipeSwiping ? (swipeDirection === 'left' ? -500 : 500) : swipeDeltaX;
      return `transform: translateX(${x}px);`;
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

  // --- Swipe handlers (horizontal, on queue items) ---

  function handleSwipeTouchStart(e: TouchEvent, idx: number) {
    if (isDragging || swipeSwiping) return;
    swipeStartX = e.touches[0].clientX;
    swipeStartY = e.touches[0].clientY;
    swipeDeltaX = 0;
    swipeLocked = null;
    swipeIdx = idx;
  }

  function handleSwipeTouchMove(e: TouchEvent) {
    if (swipeIdx === null || isDragging || swipeSwiping) return;
    const dx = e.touches[0].clientX - swipeStartX;
    const dy = e.touches[0].clientY - swipeStartY;

    if (swipeLocked === null) {
      if (Math.abs(dx) > SWIPE_LOCK_THRESHOLD || Math.abs(dy) > SWIPE_LOCK_THRESHOLD) {
        swipeLocked = Math.abs(dx) > Math.abs(dy) ? 'horizontal' : 'vertical';
        if (swipeLocked === 'horizontal') swipeDragging = true;
      }
      return;
    }

    if (swipeLocked === 'vertical') return;
    e.preventDefault();
    swipeDeltaX = dx;
  }

  function handleSwipeTouchEnd() {
    if (swipeLocked === 'vertical' || !swipeDragging || swipeSwiping) {
      resetSwipeState();
      return;
    }
    finishSwipeGesture();
  }

  function handleSwipeMouseDown(e: MouseEvent, idx: number) {
    if (isDragging || swipeSwiping) return;
    swipeStartX = e.clientX;
    swipeDeltaX = 0;
    swipeLocked = null;
    swipeIdx = idx;
    swipeDragging = true;
    document.addEventListener('mousemove', handleSwipeMouseMove);
    document.addEventListener('mouseup', handleSwipeMouseUp);
  }

  function handleSwipeMouseMove(e: MouseEvent) {
    if (!swipeDragging || swipeSwiping) return;
    e.preventDefault();
    swipeDeltaX = e.clientX - swipeStartX;
  }

  function handleSwipeMouseUp() {
    document.removeEventListener('mousemove', handleSwipeMouseMove);
    document.removeEventListener('mouseup', handleSwipeMouseUp);
    if (!swipeDragging || swipeSwiping) {
      resetSwipeState();
      return;
    }
    finishSwipeGesture();
  }

  function resetSwipeState() {
    swipeIdx = null;
    swipeDeltaX = 0;
    swipeDragging = false;
    swipeLocked = null;
  }

  function finishSwipeGesture() {
    swipeDragging = false;
    if (swipeDeltaX < -SWIPE_THRESHOLD) {
      triggerSwipeAction('left');
    } else if (swipeDeltaX > SWIPE_THRESHOLD) {
      triggerSwipeAction('right');
    } else {
      swipeDeltaX = 0;
      swipeIdx = null;
    }
  }

  async function triggerSwipeAction(direction: 'left' | 'right') {
    if (swipeIdx === null) return;
    const track = tracks[swipeIdx];
    if (!track) return;

    swipeSwiping = true;
    swipeDirection = direction;

    // Animate slide off screen
    await new Promise(r => setTimeout(r, 300));

    // Collapse the container (keep swipeSwiping true to block interactions)
    collapsingId = track.id;
    await new Promise(r => setTimeout(r, 200));

    // Perform the action
    try {
      let state;
      if (direction === 'left') {
        state = await api.queueReject(track.id);
      } else {
        state = await api.queueRemove(track.id);
      }
      updateState(state);
    } catch (e) {
      console.error('[swipefi] queue swipe action failed:', e);
    }

    // Reload queue, then reset all state
    await loadQueue();
    collapsingId = null;
    swipeSwiping = false;
    swipeDirection = null;
    resetSwipeState();
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

  {#if queueFolder || queueSortBy}
    <div class="queue-context">
      {#if queueFolder}Playing from {queueFolder}{/if}
      {#if queueFolder && queueSortBy} &middot; {/if}
      {#if queueSortBy}{formatSortLabel(queueSortBy, queueSortOrder)}{/if}
    </div>
  {/if}

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
          class="swipe-container"
          class:collapsing={collapsingId === track.id}
          class:dragging={isDragging && dragOriginalIndex === idx}
          style={getDragStyle(idx)}
        >
          <!-- Reveal backgrounds: left bg shows when swiping left (reject), right bg when swiping right (remove) -->
          {#if swipeIdx === idx && swipeDeltaX < 0}
            <div class="swipe-bg swipe-bg-left">
              <Trash2 size={18} />
            </div>
          {/if}
          {#if swipeIdx === idx && swipeDeltaX > 0}
            <div class="swipe-bg swipe-bg-right">
              <X size={18} />
            </div>
          {/if}

          <div
            class="queue-item"
            class:current={track.id === currentTrackId}
            class:swiping-out={swipeSwiping && swipeIdx === idx}
            data-testid="queue-item"
            data-track-id={track.id}
            role="button"
            tabindex="0"
            style={getSwipeStyle(idx)}
            onclick={() => { if (!swipeDragging && !swipeSwiping) skipTo(track.id); }}
            onkeydown={(e) => { if (e.key === 'Enter' && !swipeDragging) skipTo(track.id); }}
            ontouchstart={(e) => handleSwipeTouchStart(e, idx)}
            ontouchmove={handleSwipeTouchMove}
            ontouchend={handleSwipeTouchEnd}
            onmousedown={(e) => {
              const target = e.target as HTMLElement;
              if (!target.closest('.drag-handle') && !target.closest('.move-btn')) {
                handleSwipeMouseDown(e, idx);
              }
            }}
          >
            <button
              type="button"
              class="drag-handle"
              data-testid="drag-handle"
              aria-label="Reorder"
              onclick={(e) => e.stopPropagation()}
              ontouchstart={(e) => { e.stopPropagation(); handleGripTouchStart(e, idx); }}
              onmousedown={(e) => handleGripMouseDown(e, idx)}
            >
              <GripVertical size={20} />
            </button>

            <div class="track-indicator">
              {#if track.id === currentTrackId}
                <Play size={14} fill="#4ec484" color="#4ec484" />
              {:else}
                <span class="track-num">{idx + 1}</span>
              {/if}
            </div>

            <div class="track-details">
              <span class="track-title">{track.title}</span>
              <span class="track-meta">{track.artist || 'Unknown'}</span>
            </div>
            <div class="sort-value">
              {#if getSort() === 'play_count'}
                {#if track.play_count > 0}
                  <span class="pcount">▶ {track.play_count}</span>
                {:else}
                  <span class="pcount zero">—</span>
                {/if}
              {:else}
                <span class="date-val"><Clock size={12} /> {formatDate(track.added_at)}</span>
              {/if}
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

  .queue-context {
    text-align: center;
    font-size: 0.75rem;
    color: #666;
    padding: 0.2rem 0.5rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    position: relative;
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

  /* During active drag: non-dragged containers animate transforms smoothly */
  .queue-list.dragging-active .swipe-container:not(.dragging) {
    transition: transform 0.15s ease;
  }

  /* Dragged container: no transform transition, follows finger directly */
  .queue-list.dragging-active .swipe-container.dragging {
    transition: none;
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

  .swipe-container.dragging .queue-item {
    background: #2a2a2a;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
    border-radius: 12px;
    touch-action: none;
  }

  .queue-item.swiping-out {
    transition: transform 0.3s ease-out;
  }

  .swipe-container {
    position: relative;
    overflow: hidden;
    border-radius: 10px;
    flex-shrink: 0;
    transition: height 0.2s ease, opacity 0.2s ease, margin 0.2s ease;
  }

  .swipe-container.collapsing {
    height: 0 !important;
    opacity: 0;
    margin: 0;
    overflow: hidden;
  }

  .swipe-bg {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 100%;
    display: flex;
    align-items: center;
    padding: 0 1.25rem;
    color: white;
    font-weight: 600;
    font-size: 0.85rem;
    gap: 0.5rem;
    pointer-events: none;
  }

  .swipe-bg-left {
    background: var(--color-danger, #e53935);
    justify-content: flex-end;
  }

  .swipe-bg-right {
    background: #444;
    justify-content: flex-start;
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
    margin-left: -0.25rem;
    margin-right: 0.25rem;
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

  .swipe-container.dragging .drag-handle {
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

  .sort-value {
    flex-shrink: 0;
    min-width: 3.5rem;
    text-align: right;
    font-size: 0.75rem;
    color: #666;
    font-variant-numeric: tabular-nums;
    display: flex;
    align-items: center;
    justify-content: flex-end;
  }

  .pcount {
    color: var(--color-accent, #4ec484);
    font-weight: 600;
    letter-spacing: 0.02em;
  }

  .pcount.zero {
    color: #444;
    font-weight: 400;
  }

  .date-val {
    color: var(--color-accent, #4ec484);
    font-weight: 600;
    letter-spacing: 0.02em;
    white-space: nowrap;
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
  }

  .loading, .empty {
    text-align: center;
    padding: 2rem;
    color: var(--color-text-secondary);
  }
</style>
