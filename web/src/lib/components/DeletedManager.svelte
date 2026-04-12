<script lang="ts">
  import { api, type Track } from '../api/client';
  import { ArrowLeft, RotateCcw, Trash2, CheckSquare, Square, CheckSquare2 } from 'lucide-svelte';

  let { onBack, onBusyChange }: { onBack: () => void; onBusyChange?: (busy: boolean) => void } = $props();

  $effect(() => { onBusyChange?.(busy); });

  let tracks = $state<Track[]>([]);
  let selected = $state<Set<number>>(new Set());
  let loading = $state(true);
  let busy = $state(false);
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
    if (selected.size === 0 || busy) return;
    error = '';
    busy = true;
    try {
      const result = await api.restoreDeleted([...selected]);
      if (result.errors?.length) {
        error = result.errors.join('; ');
      }
      await loadDeleted();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Restore failed';
    } finally {
      busy = false;
    }
  }

  async function purgeSelected() {
    if (selected.size === 0 || busy) return;
    error = '';
    showPurgeConfirm = false;
    busy = true;
    try {
      await api.purgeDeleted([...selected]);
      await loadDeleted();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Delete failed';
    } finally {
      busy = false;
    }
  }

  loadDeleted();
</script>

<div class="deleted-manager">
  <header class="dm-header">
    <button class="back-btn" onclick={onBack} disabled={busy} aria-label="Back">
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
      <button class="select-all-btn" onclick={toggleAll} disabled={busy}>
        {#if allSelected}
          <CheckSquare2 size={18} />
        {:else}
          <Square size={18} />
        {/if}
        <span>{allSelected ? 'Deselect all' : 'Select all'}</span>
      </button>

      {#if selected.size > 0}
        <button class="restore-btn" onclick={restoreSelected} disabled={busy}>
          <RotateCcw size={16} />
          <span>Restore ({selected.size})</span>
        </button>
        <button class="purge-btn" onclick={() => showPurgeConfirm = true} disabled={busy}>
          <Trash2 size={16} />
          <span>Delete ({selected.size})</span>
        </button>
      {/if}
    </div>

    <div class="track-list-container">
      {#if busy}
        <div class="busy-overlay">
          <div class="busy-spinner"></div>
          <span>Processing...</span>
        </div>
      {/if}
      <div class="track-list" class:dimmed={busy}>
        {#each tracks as track (track.id)}
          <button class="track-item" class:selected={selected.has(track.id)} onclick={() => { if (!busy) toggleSelect(track.id); }} disabled={busy}>
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
    </div>
  {/if}

  {#if showPurgeConfirm}
    <div class="confirm-overlay" role="button" tabindex="-1" onclick={() => showPurgeConfirm = false} onkeydown={(e) => { if (e.key === 'Escape') showPurgeConfirm = false; }}>
      <div class="confirm-dialog" role="presentation" onclick={(e) => e.stopPropagation()}>
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

  .back-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .actions-bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
  }

  .select-all-btn {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    background: none;
    border: none;
    color: var(--color-text-secondary);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.6rem;
    border-radius: 6px;
  }

  .select-all-btn:hover {
    color: var(--color-text);
    background: rgba(255, 255, 255, 0.1);
  }

  .restore-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: var(--color-secondary);
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
    background: var(--color-danger);
    border: none;
    color: white;
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.8rem;
    border-radius: 16px;
    font-weight: 600;
  }

  .restore-btn:disabled, .purge-btn:disabled, .select-all-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .track-list-container {
    flex: 1;
    min-height: 0;
    position: relative;
  }

  .busy-overlay {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
    z-index: 10;
    border-radius: 8px;
    color: #ccc;
    font-size: 0.9rem;
  }

  .busy-spinner {
    width: 24px;
    height: 24px;
    border: 3px solid #333;
    border-top-color: var(--color-secondary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .track-list {
    height: 100%;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .track-list.dimmed {
    opacity: 0.4;
    pointer-events: none;
  }

  .track-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: var(--color-bg-card);
    border: none;
    border-radius: 8px;
    padding: 0.7rem 0.75rem;
    color: var(--color-text);
    cursor: pointer;
    text-align: left;
  }

  .track-item:hover {
    background: var(--color-bg-hover);
  }

  .track-item.selected {
    background: #1a1a2a;
  }

  .checkbox {
    color: #555;
    flex-shrink: 0;
    display: flex;
  }

  .track-item.selected .checkbox {
    color: var(--color-secondary);
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
    color: var(--color-danger-hover);
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
    color: var(--color-text);
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
  }

  .confirm-delete {
    background: var(--color-danger);
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
    color: var(--color-text-secondary);
  }

  .error {
    color: var(--color-danger-hover);
    padding: 0.5rem;
  }
</style>
