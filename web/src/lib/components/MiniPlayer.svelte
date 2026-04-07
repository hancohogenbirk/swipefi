<script lang="ts">
  import { getPlayerState } from '../stores/player.svelte';
  import { Play, Pause } from 'lucide-svelte';

  let { onClick }: { onClick: () => void } = $props();

  let ps = $derived(getPlayerState());
  let visible = $derived(ps.state !== 'idle' && ps.track);
</script>

{#if visible && ps.track}
  <button class="mini-player" onclick={onClick}>
    <img
      src="/api/tracks/{ps.track.id}/art"
      alt=""
      class="mini-art"
      onerror={(e) => (e.currentTarget as HTMLImageElement).style.display = 'none'}
    />
    <div class="mini-info">
      <span class="mini-title">{ps.track.title}</span>
      <span class="mini-artist">{ps.track.artist || 'Unknown'}</span>
    </div>
    <span class="mini-state">
      {#if ps.state === 'playing'}
        <Pause size={18} fill="currentColor" />
      {:else}
        <Play size={18} fill="currentColor" />
      {/if}
    </span>
    <div class="mini-progress-track">
      <div class="mini-progress" style="width: {ps.duration_ms ? (ps.position_ms / ps.duration_ms * 100) : 0}%"></div>
    </div>
  </button>
{/if}

<style>
  .mini-player {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: var(--color-bg-card);
    border: none;
    border-top: 1px solid #333;
    color: var(--color-text);
    padding: 0.6rem 1rem;
    cursor: pointer;
    width: 100%;
    text-align: left;
    position: relative;
    overflow: hidden;
    flex-shrink: 0;
  }

  .mini-art {
    width: 42px;
    height: 42px;
    border-radius: 6px;
    object-fit: cover;
    flex-shrink: 0;
  }

  .mini-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }

  .mini-title {
    font-size: 0.9rem;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .mini-artist {
    font-size: 0.75rem;
    color: var(--color-text-secondary);
  }

  .mini-state {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    color: var(--color-text);
  }

  .mini-progress-track {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: transparent;
  }

  .mini-progress {
    height: 100%;
    background: linear-gradient(90deg, #1db954, #7cb3ff);
    background-size: 100vw 100%;
    transition: width 1s linear;
  }
</style>
