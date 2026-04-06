<script lang="ts">
  import { api } from '../api/client';
  import { X, Folder, ArrowUp, Zap, Trash2 } from 'lucide-svelte';

  let { onDone, onOpenDeleted }: { onDone: () => void; onOpenDeleted?: () => void } = $props();

  let currentPath = $state('/');
  let parentPath = $state('/');
  let entries = $state<{ name: string; path: string; is_dir: boolean }[]>([]);
  let loading = $state(false);
  let saving = $state(false);
  let error = $state('');

  async function browse(path: string) {
    loading = true;
    error = '';
    try {
      const result = await api.browse(path);
      currentPath = result.current;
      parentPath = result.parent;
      entries = result.entries ?? [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to browse';
    } finally {
      loading = false;
    }
  }

  async function selectDir() {
    saving = true;
    error = '';
    try {
      await api.setMusicDir(currentPath);
      onDone();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save';
    } finally {
      saving = false;
    }
  }

  let shortcuts = $state<{ name: string; path: string }[]>([]);

  async function loadShortcuts() {
    try {
      const result = await api.shortcuts();
      shortcuts = result ?? [];
    } catch {
      // no shortcuts available
    }
  }

  let deletedCount = $state(0);

  async function loadDeletedCount() {
    try {
      const tracks = await api.listDeleted();
      deletedCount = tracks?.length ?? 0;
    } catch {
      // ignore
    }
  }

  // Start browsing from root and load shortcuts
  browse('/');
  loadShortcuts();
  loadDeletedCount();
</script>

<div class="settings">
  <header class="settings-header">
    <div class="settings-title-row">
      <h2>Choose Music Directory</h2>
      <button class="close-btn" onclick={onDone} aria-label="Close settings"><X size={20} /></button>
    </div>
    <p class="current-path">{currentPath}</p>
  </header>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  {#if shortcuts.length > 0 && currentPath === '/'}
    <div class="shortcuts">
      <p class="section-label">Quick access</p>
      {#each shortcuts as sc}
        <button class="dir-item shortcut" onclick={() => browse(sc.path)}>
          <span class="dir-icon"><Zap size={20} /></span>
          <span class="dir-name">{sc.name}</span>
        </button>
      {/each}
    </div>
    <p class="section-label">Browse filesystem</p>
  {/if}

  <div class="dir-list">
    {#if currentPath !== '/'}
      <button class="dir-item" onclick={() => browse(parentPath)}>
        <span class="dir-icon"><ArrowUp size={20} /></span>
        <span class="dir-name">..</span>
      </button>
    {/if}

    {#if loading}
      <div class="loading">Loading...</div>
    {:else}
      {#each entries as entry}
        <button class="dir-item" onclick={() => browse(entry.path)}>
          <span class="dir-icon"><Folder size={20} /></span>
          <span class="dir-name">{entry.name}</span>
        </button>
      {/each}

      {#if entries.length === 0}
        <div class="empty">No subdirectories</div>
      {/if}
    {/if}
  </div>

  <div class="actions">
    <button class="select-btn" onclick={selectDir} disabled={saving}>
      {saving ? 'Saving...' : `Use "${currentPath.split('/').pop() || currentPath}"`}
    </button>
  </div>

  {#if onOpenDeleted}
    <div class="section-divider"></div>
    <button class="deleted-btn" onclick={onOpenDeleted}>
      <Trash2 size={20} />
      <span>Marked for Deletion</span>
      {#if deletedCount > 0}
        <span class="deleted-count">{deletedCount}</span>
      {/if}
    </button>
  {/if}
</div>

<style>
  .settings {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 1rem;
  }

  .settings-header {
    margin-bottom: 1rem;
  }

  .settings-title-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .settings-header h2 {
    font-size: 1.3rem;
    margin: 0;
  }

  .close-btn {
    background: none;
    border: none;
    color: #888;
    font-size: 1.2rem;
    cursor: pointer;
    padding: 0.5rem;
    border-radius: 50%;
  }

  .close-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #f0f0f0;
  }

  .current-path {
    font-size: 0.8rem;
    color: #888;
    margin: 0.25rem 0 0;
    word-break: break-all;
  }

  .dir-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .dir-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a1a1a;
    border: none;
    border-radius: 8px;
    padding: 0.75rem 1rem;
    color: #f0f0f0;
    font-size: 1rem;
    cursor: pointer;
    text-align: left;
  }

  .dir-item:hover {
    background: #252525;
  }

  .dir-icon {
    font-size: 1.2rem;
    flex-shrink: 0;
  }

  .dir-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .actions {
    padding-top: 1rem;
  }

  .select-btn {
    width: 100%;
    background: #1db954;
    border: none;
    color: white;
    padding: 1rem;
    border-radius: 12px;
    font-size: 1rem;
    font-weight: 600;
    cursor: pointer;
  }

  .select-btn:hover {
    background: #1ed760;
  }

  .select-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .shortcuts {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin-bottom: 0.5rem;
  }

  .shortcut {
    border-left: 3px solid #1db954;
  }

  .section-label {
    font-size: 0.75rem;
    color: #666;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin: 0.5rem 0 0.25rem;
  }

  .loading, .empty, .error {
    text-align: center;
    padding: 2rem;
    color: #888;
  }

  .error {
    color: #ff6b6b;
    padding: 0.5rem;
  }

  .section-divider {
    height: 1px;
    background: #222;
    margin: 1rem 0;
  }

  .deleted-btn {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a1a1a;
    border: 1px solid #333;
    border-radius: 12px;
    padding: 1rem;
    color: #f0f0f0;
    font-size: 1rem;
    cursor: pointer;
    width: 100%;
    text-align: left;
  }

  .deleted-btn:hover {
    border-color: #ff4444;
    background: #1e1e1e;
  }

  .deleted-count {
    margin-left: auto;
    background: #ff4444;
    color: white;
    font-size: 0.75rem;
    font-weight: 700;
    padding: 0.15rem 0.5rem;
    border-radius: 10px;
    min-width: 1.5rem;
    text-align: center;
  }
</style>
