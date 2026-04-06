<script lang="ts">
  import { api, type Track } from '../api/client';
  import { ArrowLeft, RotateCcw, Trash2, CheckSquare, Square, CheckSquare2 } from 'lucide-svelte';

  let { onBack }: { onBack: () => void } = $props();

  let tracks = $state<Track[]>([]);
  let selected = $state<Set<number>>(new Set());
  let loading = $state(true);
  let error = $state('');
  let showPurgeConfirm = $state(false);

  let allSelected = $derived(tracks.length > 0 && selected.size === tracks.length);

  async function loadDeleted() {
    loading = true;
    error = '';
    try {
      tracks = (await api.listDeleted()) ?? [];
      selected = new Set();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load';
    } finally {
      loading = false;
    }
  }

  function toggleSelect(id: number) {
    const next = new Set(selected);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selected = next;
  }

  function toggleAll() {
    if (allSelected) {
      selected = new Set();
    } else {
      selected = new Set(tracks.map(t => t.id));
    }
  }

  async function restoreSelected() {
    if (selected.size === 0) return;
    error = '';
    try {
      const result = await api.restoreDeleted([...selected]);
      if (result.errors?.length) {
        error = result.errors.join('; ');
      }
      await loadDeleted();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Restore failed';
    }
  }

  async function purgeSelected() {
    if (selected.size === 0) return;
    error = '';
    try {
      await api.purgeDeleted([...selected]);
      showPurgeConfirm = false;
      await loadDeleted();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Delete failed';
    }
  }

  loadDeleted();
</script>

<div class="deleted-manager">
  <header class="dm-header">
    <button class="back-btn" onclick={onBack} aria-label="Back">
      <ArrowLeft size={24} />
    </button>
    <h2>Marked for Deletion</h2>
    <span class="count">{tracks.length} files</span>
  </header>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  {#if loading}
    <div class="loading">Loading...</div>
  {:else if tracks.length === 0}
    <div class="empty">No files marked for deletion</div>
  {:else}
    <div class="actions-bar">
      <button class="select-all-btn" onclick={toggleAll}>
        {#if allSelected}
          <CheckSquare2 size={18} />
        {:else}
          <Square size={18} />
        {/if}
        <span>{allSelected ? 'Deselect all' : 'Select all'}</span>
      </button>

      {#if selected.size > 0}
        <button class="restore-btn" onclick={restoreSelected}>
          <RotateCcw size={16} />
          <span>Restore ({selected.size})</span>
        </button>
        <button class="purge-btn" onclick={() => showPurgeConfirm = true}>
          <Trash2 size={16} />
          <span>Delete ({selected.size})</span>
        </button>
      {/if}
    </div>

    <div class="track-list">
      {#each tracks as track (track.id)}
        <button class="track-item" class:selected={selected.has(track.id)} onclick={() => toggleSelect(track.id)}>
          <div class="checkbox">
            {#if selected.has(track.id)}
              <CheckSquare size={20} />
            {:else}
              <Square size={20} />
            {/if}
          </div>
          <div class="track-details">
            <span class="track-title">{track.title}</span>
            <span class="track-meta">
              {track.artist || 'Unknown'}
              {#if track.album} · {track.album}{/if}
              {#if track.play_count > 0} · {track.play_count}×{/if}
            </span>
          </div>
        </button>
      {/each}
    </div>
  {/if}

  {#if showPurgeConfirm}
    <div class="confirm-overlay" onclick={() => showPurgeConfirm = false}>
      <div class="confirm-dialog" onclick={(e) => e.stopPropagation()}>
        <p>Permanently delete {selected.size} file{selected.size !== 1 ? 's' : ''}?</p>
        <p class="confirm-warning">This cannot be undone.</p>
        <div class="confirm-actions">
          <button class="confirm-cancel" onclick={() => showPurgeConfirm = false}>Cancel</button>
          <button class="confirm-delete" onclick={purgeSelected}>Delete Forever</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .deleted-manager {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 0.75rem;
  }

  .dm-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.25rem 0.5rem;
    margin-bottom: 0.5rem;
  }

  .dm-header h2 {
    font-size: 1.1rem;
    margin: 0;
    flex: 1;
  }

  .count {
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

  .actions-bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    flex-wrap: wrap;
  }

  .select-all-btn {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: none;
    border: none;
    color: #888;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
  }

  .select-all-btn:hover {
    color: #f0f0f0;
    background: rgba(255, 255, 255, 0.1);
  }

  .restore-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: #1db954;
    border: none;
    color: white;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.8rem;
    border-radius: 16px;
    font-weight: 600;
    margin-left: auto;
  }

  .purge-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: #ff4444;
    border: none;
    color: white;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.8rem;
    border-radius: 16px;
    font-weight: 600;
  }

  .track-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .track-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: #1a1a1a;
    border: none;
    border-radius: 8px;
    padding: 0.7rem 0.75rem;
    color: #f0f0f0;
    cursor: pointer;
    text-align: left;
  }

  .track-item:hover {
    background: #222;
  }

  .track-item.selected {
    background: #1a2a1a;
  }

  .checkbox {
    color: #555;
    flex-shrink: 0;
    display: flex;
  }

  .track-item.selected .checkbox {
    color: #1db954;
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

  .confirm-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .confirm-dialog {
    background: #1e1e1e;
    border-radius: 16px;
    padding: 1.5rem 2rem;
    text-align: center;
    min-width: 280px;
  }

  .confirm-dialog p {
    font-size: 1.05rem;
    margin: 0 0 0.5rem;
  }

  .confirm-warning {
    color: #ff6b6b;
    font-size: 0.85rem !important;
    margin-bottom: 1.25rem !important;
  }

  .confirm-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: center;
  }

  .confirm-cancel {
    background: #333;
    border: none;
    color: #f0f0f0;
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
  }

  .confirm-delete {
    background: #ff4444;
    border: none;
    color: white;
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
    font-weight: 600;
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
</style>
