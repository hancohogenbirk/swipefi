<script lang="ts">
  import { api, type Folder, type Track } from '../api/client';
  import { getSort, getOrder, setSort, setOrder } from '../stores/library.svelte';
  import { updateState } from '../stores/player.svelte';
  import { Folder as FolderIcon, ArrowUp, Play, Music } from 'lucide-svelte';

  let { onNavigateToPlayer, onFolderNavigate, goBackSignal = 0 }: {
    onNavigateToPlayer: () => void;
    onFolderNavigate?: (path: string) => void;
    goBackSignal?: number;
  } = $props();

  let currentPath = $state('');
  let folders = $state<Folder[]>([]);
  let tracks = $state<Track[]>([]);
  let trackCount = $state(0);
  let loading = $state(false);
  let error = $state('');

  let pathParts = $derived(
    currentPath ? currentPath.split('/') : []
  );

  async function loadFolders(path: string) {
    loading = true;
    error = '';
    try {
      folders = (await api.folders(path)) ?? [];
      currentPath = path;
      // Direct children for display
      tracks = (await api.tracksDirectOnly(path || '', getSort(), getOrder())) ?? [];
      // Recursive count for the "Play all" button
      const allTracks = await api.tracks(path || '', getSort(), getOrder());
      trackCount = allTracks?.length ?? 0;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load folders';
    } finally {
      loading = false;
    }
  }

  async function playFolder(folderPath: string) {
    try {
      const state = await api.play(folderPath, getSort(), getOrder());
      updateState(state);
      onNavigateToPlayer();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start playback';
    }
  }

  function navigateTo(path: string) {
    loadFolders(path);
    onFolderNavigate?.(path);
  }

  function navigateUp() {
    const parts = currentPath.split('/');
    parts.pop();
    loadFolders(parts.join('/'));
  }

  function navigateToBreadcrumb(index: number) {
    const path = pathParts.slice(0, index + 1).join('/');
    loadFolders(path);
    onFolderNavigate?.(path);
  }

  function handleSortChange(e: Event) {
    const val = (e.target as HTMLSelectElement).value;
    const [sort, order] = val.split(':');
    setSort(sort);
    setOrder(order);
  }

  // React to back signal from App (browser back button)
  let lastBackSignal = $state(0);
  $effect(() => {
    if (goBackSignal > lastBackSignal) {
      lastBackSignal = goBackSignal;
      navigateUp();
    }
  });

  // Load root on mount
  loadFolders('');
</script>

<div class="folder-nav">
  <header class="nav-header">
    <div class="breadcrumbs">
      <button class="crumb" onclick={() => loadFolders('')}>Music</button>
      {#each pathParts as part, i}
        <span class="separator">/</span>
        <button class="crumb" onclick={() => navigateToBreadcrumb(i)}>{part}</button>
      {/each}
    </div>
    <div class="header-actions">
      <select class="sort-select" onchange={handleSortChange} value={`${getSort()}:${getOrder()}`}>
        <option value="added_at:desc">Newest first</option>
        <option value="added_at:asc">Oldest first</option>
        <option value="play_count:asc">Least played</option>
        <option value="play_count:desc">Most played</option>
      </select>
    </div>
  </header>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  <!-- Play all button for current folder -->
  {#if trackCount > 0}
    <button class="play-all-btn" onclick={() => playFolder(currentPath)}>
      <Play size={18} fill="currentColor" />
      <span>Play all {trackCount} tracks</span>
    </button>
  {/if}

  {#if loading}
    <div class="loading">Loading...</div>
  {:else}
    <div class="folder-list">
      {#if currentPath}
        <button class="folder-item" onclick={navigateUp}>
          <span class="folder-icon"><ArrowUp size={20} /></span>
          <span class="folder-name">..</span>
        </button>
      {/if}

      {#each folders as folder}
        <div class="folder-item">
          <button class="folder-link" onclick={() => navigateTo(folder.path)}>
            <span class="folder-icon"><FolderIcon size={20} /></span>
            <span class="folder-name">{folder.name}</span>
          </button>
          <button class="play-btn" onclick={() => playFolder(folder.path)} title="Play all in folder">
            <Play size={16} fill="currentColor" />
          </button>
        </div>
      {/each}

      {#if folders.length === 0 && trackCount === 0 && !currentPath}
        <div class="empty">No folders found. Check your music directory in settings.</div>
      {/if}

      {#each tracks as track (track.id)}
        <div class="track-item">
          <Music size={18} />
          <div class="track-details">
            <span class="track-title">{track.title}</span>
            <span class="track-meta">
              {track.artist || 'Unknown'}
              {#if track.play_count > 0} · {track.play_count}×{/if}
            </span>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .folder-nav {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 1rem;
  }

  .nav-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .breadcrumbs {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    flex-wrap: wrap;
    min-width: 0;
  }

  .crumb {
    background: none;
    border: none;
    color: #7cb3ff;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
  }

  .crumb:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .separator {
    color: #666;
    font-size: 0.8rem;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .sort-select {
    background: #222;
    color: #f0f0f0;
    border: 1px solid #444;
    border-radius: 8px;
    padding: 0.5rem;
    font-size: 0.85rem;
  }

  .play-all-btn {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: linear-gradient(135deg, #1db954, #17a34a);
    border: none;
    border-radius: 12px;
    padding: 0.85rem 1.25rem;
    color: white;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
    margin-bottom: 0.75rem;
  }

  .play-all-btn:hover {
    filter: brightness(1.1);
  }

  .folder-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-y: auto;
    flex: 1;
  }

  .folder-item {
    display: flex;
    align-items: center;
    background: #1a1a1a;
    border: none;
    border-radius: 8px;
    padding: 0.75rem 1rem;
    color: #f0f0f0;
    font-size: 1rem;
    cursor: pointer;
    text-align: left;
    gap: 0.75rem;
  }

  .folder-item:hover {
    background: #252525;
  }

  .folder-link {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: none;
    border: none;
    color: #f0f0f0;
    font-size: 1rem;
    cursor: pointer;
    flex: 1;
    text-align: left;
    padding: 0;
  }

  .folder-icon {
    font-size: 1.2rem;
    flex-shrink: 0;
  }

  .folder-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .play-btn {
    background: #1db954;
    border: none;
    color: white;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    font-size: 0.9rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .play-btn:hover {
    background: #1ed760;
  }

  .track-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #151515;
    border-radius: 8px;
    padding: 0.6rem 1rem;
    color: #ccc;
    font-size: 0.9rem;
  }

  .track-details {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
  }

  .track-title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .track-meta {
    font-size: 0.7rem;
    color: #666;
  }

  .loading, .empty, .error {
    text-align: center;
    padding: 2rem;
    color: #888;
  }

  .error {
    color: #ff6b6b;
  }
</style>
