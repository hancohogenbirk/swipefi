<script lang="ts">
  import type { Track } from '../api/client';
  import { Music } from 'lucide-svelte';

  let {
    track,
    onSwipeLeft,
    onSwipeRight,
  }: {
    track: Track;
    onSwipeLeft: () => void;
    onSwipeRight: () => void;
  } = $props();

  let showPlaceholder = $state(true);

  const SWIPE_THRESHOLD = 80;
  const ROTATION_FACTOR = 0.1;

  let startX = $state(0);
  let deltaX = $state(0);
  let dragging = $state(false);
  let swiping = $state(false);
  let swipeDirection = $state<'left' | 'right' | null>(null);

  let rotation = $derived(dragging ? deltaX * ROTATION_FACTOR : 0);
  let opacity = $derived(Math.max(0, 1 - Math.abs(deltaX) / 300));

  let cardStyle = $derived(
    swiping
      ? `transform: translateX(${swipeDirection === 'left' ? -500 : 500}px) rotate(${swipeDirection === 'left' ? -30 : 30}deg); opacity: 0; transition: transform 0.3s ease-out, opacity 0.3s ease-out;`
      : dragging
        ? `transform: translateX(${deltaX}px) rotate(${rotation}deg); opacity: ${opacity}; transition: none;`
        : 'transform: translateX(0) rotate(0deg); opacity: 1; transition: transform 0.3s ease, opacity 0.3s ease;'
  );

  let feedbackLabel = $derived(
    !dragging || swiping ? '' : deltaX > 30 ? 'keep' : deltaX < -30 ? 'reject' : ''
  );

  // Touch events (mobile)
  function handleTouchStart(e: TouchEvent) {
    if (swiping) return;
    dragging = true;
    startX = e.touches[0].clientX;
    deltaX = 0;
  }

  function handleTouchMove(e: TouchEvent) {
    if (!dragging || swiping) return;
    e.preventDefault();
    deltaX = e.touches[0].clientX - startX;
  }

  function handleTouchEnd() {
    if (!dragging || swiping) return;
    finishSwipe();
  }

  // Mouse events (desktop)
  function handleMouseDown(e: MouseEvent) {
    if (swiping) return;
    dragging = true;
    startX = e.clientX;
    deltaX = 0;
    window.addEventListener('mousemove', handleMouseMove);
    window.addEventListener('mouseup', handleMouseUp);
  }

  function handleMouseMove(e: MouseEvent) {
    if (!dragging || swiping) return;
    deltaX = e.clientX - startX;
  }

  function handleMouseUp() {
    window.removeEventListener('mousemove', handleMouseMove);
    window.removeEventListener('mouseup', handleMouseUp);
    if (!dragging || swiping) return;
    finishSwipe();
  }

  function finishSwipe() {
    dragging = false;
    if (deltaX > SWIPE_THRESHOLD) {
      triggerSwipe('right');
    } else if (deltaX < -SWIPE_THRESHOLD) {
      triggerSwipe('left');
    } else {
      deltaX = 0;
    }
  }

  function triggerSwipe(direction: 'left' | 'right') {
    swiping = true;
    swipeDirection = direction;

    setTimeout(() => {
      swiping = false;
      swipeDirection = null;
      deltaX = 0;

      if (direction === 'left') {
        onSwipeLeft();
      } else {
        onSwipeRight();
      }
    }, 300);
  }
</script>

<div
  class="swipe-card"
  style={cardStyle}
  ontouchstart={handleTouchStart}
  ontouchmove={handleTouchMove}
  ontouchend={handleTouchEnd}
  onmousedown={handleMouseDown}
  role="button"
  tabindex="0"
>
  {#if feedbackLabel === 'keep'}
    <div class="swipe-overlay keep">KEEP</div>
  {/if}
  {#if feedbackLabel === 'reject'}
    <div class="swipe-overlay reject">DELETE</div>
  {/if}

  <div class="art-container">
    <img
      src="/api/tracks/{track.id}/art"
      alt=""
      class="art-image"
      onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; showPlaceholder = true; }}
      onload={() => showPlaceholder = false}
    />
    {#if showPlaceholder}
      <div class="art-placeholder">
        <Music size={48} />
        <span class="no-art-label">No cover art</span>
      </div>
    {/if}
  </div>

  <div class="track-info">
    <h2 class="title">{track.title}</h2>
    <p class="artist">{track.artist || 'Unknown Artist'}</p>
    <p class="album">{track.album || 'Unknown Album'}</p>
    <p class="play-count">Played {track.play_count} time{track.play_count !== 1 ? 's' : ''}</p>
  </div>

  <div class="swipe-hints">
    <span class="hint hint-left">← Delete</span>
    <span class="hint hint-right">Keep →</span>
  </div>
</div>

<style>
  .swipe-card {
    background: linear-gradient(145deg, #1e1e1e, #2a2a2a);
    border-radius: 20px;
    padding: 1.5rem 1.5rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
    user-select: none;
    touch-action: pan-y;
    position: relative;
    overflow: hidden;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
    width: 100%;
    max-width: 340px;
    max-height: 100%;
    margin: 0 auto;
    cursor: grab;
  }

  .swipe-card:active {
    cursor: grabbing;
  }

  .swipe-overlay {
    position: absolute;
    top: 1.25rem;
    padding: 0.6rem 1.75rem;
    border-radius: 12px;
    font-size: 2rem;
    font-weight: 900;
    letter-spacing: 0.15em;
    z-index: 10;
    pointer-events: none;
    text-shadow: 0 2px 8px rgba(0, 0, 0, 0.5);
  }

  .swipe-overlay.keep {
    right: 0.75rem;
    color: #1db954;
    border: 4px solid #1db954;
    background: rgba(29, 185, 84, 0.1);
    transform: rotate(12deg);
  }

  .swipe-overlay.reject {
    left: 0.75rem;
    color: #ff4444;
    border: 4px solid #ff4444;
    background: rgba(255, 68, 68, 0.1);
    transform: rotate(-12deg);
  }

  .art-container {
    width: 220px;
    height: 220px;
    position: relative;
    border-radius: 12px;
    overflow: hidden;
    background: #222;
    flex-shrink: 0;
  }

  .art-image {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .art-placeholder {
    width: 100%;
    height: 100%;
    background: #333;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    color: #555;
  }

  .no-art-label {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .track-info {
    text-align: center;
    width: 100%;
  }

  .title {
    font-size: 1.3rem;
    font-weight: 700;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .artist {
    font-size: 1rem;
    color: #aaa;
    margin: 0.25rem 0 0;
  }

  .album {
    font-size: 0.85rem;
    color: #777;
    margin: 0.15rem 0 0;
  }

  .play-count {
    font-size: 0.75rem;
    color: #555;
    margin: 0.5rem 0 0;
  }

  .swipe-hints {
    display: flex;
    justify-content: space-between;
    width: 100%;
    padding: 0 0.5rem;
  }

  .hint {
    font-size: 0.8rem;
    color: #555;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-weight: 600;
  }

  .hint-left {
    color: #ff6b6b;
  }

  .hint-right {
    color: #1db954;
  }
</style>
