<script lang="ts">
  import { api, type Folder, type Track } from '../api/client';
  import { getSort, getOrder, setSort, setOrder } from '../stores/library.svelte';
  import { updateState } from '../stores/player.svelte';

  let { onNavigateToPlayer, onOpenSettings }: {
    onNavigateToPlayer: () => void;
    onOpenSettings: () => void;
  } = $props();

  let currentPath = $state('');
  let folders = $state<Folder[]>([]);
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
      // Also check how many tracks are in this folder
      const tracks = await api.tracks(path || '', getSort(), getOrder());
      trackCount = tracks?.length ?? 0;
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
  }

  function navigateUp() {
    const parts = currentPath.split('/');
    parts.pop();
    loadFolders(parts.join('/'));
  }

  function navigateToBreadcrumb(index: number) {
    const path = pathParts.slice(0, index + 1).join('/');
    loadFolders(path);
  }

  function handleSortChange(e: Event) {
    const val = (e.target as HTMLSelectElement).value;
    const [sort, order] = val.split(':');
    setSort(sort);
    setOrder(order);
  }

  // Load root on mount
  loadFolders('');
</script>

<div class="folder-nav">
  <header class="nav-header">
    <div class="breadcrumbs">
      <button class="crumb" onclick={() => loadFolders('')}>Home</button>
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
      <button class="icon-btn" onclick={onOpenSettings} aria-label="Settings" title="Settings">
        <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
          <path d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58a.49.49 0 0 0 .12-.61l-1.92-3.32a.49.49 0 0 0-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54a.484.484 0 0 0-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96a.49.49 0 0 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.07.62-.07.94s.02.64.07.94l-2.03 1.58a.49.49 0 0 0-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6A3.6 3.6 0 1 1 12 8.4a3.6 3.6 0 0 1 0 7.2z"/>
        </svg>
      </button>
    </div>
  </header>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  <!-- Play all button for current folder -->
  {#if trackCount > 0}
    <button class="play-all-btn" onclick={() => playFolder(currentPath)}>
      <span class="play-all-icon">▶</span>
      <span>Play all {trackCount} tracks</span>
    </button>
  {/if}

  {#if loading}
    <div class="loading">Loading...</div>
  {:else}
    <div class="folder-list">
      {#if currentPath}
        <button class="folder-item" onclick={navigateUp}>
          <span class="folder-icon">⬆</span>
          <span class="folder-name">..</span>
        </button>
      {/if}

      {#each folders as folder}
        <div class="folder-item">
          <button class="folder-link" onclick={() => navigateTo(folder.path)}>
            <span class="folder-icon">📁</span>
            <span class="folder-name">{folder.name}</span>
          </button>
          <button class="play-btn" onclick={() => playFolder(folder.path)} title="Play all in folder">
            ▶
          </button>
        </div>
      {/each}

      {#if folders.length === 0 && trackCount === 0 && !currentPath}
        <div class="empty">No folders found. Check your music directory in settings.</div>
      {/if}
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

  .icon-btn {
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
  }

  .icon-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #f0f0f0;
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

  .play-all-icon {
    font-size: 0.9rem;
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

  .loading, .empty, .error {
    text-align: center;
    padding: 2rem;
    color: #888;
  }

  .error {
    color: #ff6b6b;
  }
</style>
