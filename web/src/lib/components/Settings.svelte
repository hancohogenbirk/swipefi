<script lang="ts">
  import { api } from '../api/client';
  import { Folder, ArrowUp, Zap, Trash2, Speaker, Unplug, FolderOpen, ChevronDown, ChevronUp, RefreshCw, AudioLines } from 'lucide-svelte';
  import type { Device } from '../api/client';

  let { onDone, onOpenDeleted, onDisconnect, onSelectDevice, onStartPolling, visible = false, scanning = false, analyzing = false, analyzed = 0, analysisTotal = 0 }: { onDone: () => void; onOpenDeleted?: () => void; onDisconnect?: () => void; onSelectDevice?: () => void; onStartPolling?: () => void; visible?: boolean; scanning?: boolean; analyzing?: boolean; analyzed?: number; analysisTotal?: number } = $props();

  // Refresh counts when tab becomes visible
  $effect(() => {
    if (visible) {
      loadDeletedCount();
      loadDeviceInfo();
    }
  });

  // Music dir browser state
  let musicDir = $state('');
  let browseOpen = $state(false);
  let currentPath = $state('/');
  let parentPath = $state('/');
  let entries = $state<{ name: string; path: string; is_dir: boolean }[]>([]);
  let loading = $state(false);
  let saving = $state(false);
  let error = $state('');

  let flacalyzerAvailable = $state(false);
  let flacalyzerEnabled = $state(false);

  async function loadConfig() {
    try {
      const config = await api.config();
      musicDir = config.music_dir || '';
      flacalyzerAvailable = config.flacalyzer_available ?? false;
      flacalyzerEnabled = config.flacalyzer_enabled ?? false;
    } catch {
      // ignore
    }
  }

  async function toggleFlacalyzer() {
    const newVal = !flacalyzerEnabled;
    try {
      await api.setFlacalyzerEnabled(newVal);
      flacalyzerEnabled = newVal;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save setting';
    }
  }

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
      musicDir = currentPath;
      browseOpen = false;
      onDone();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save';
    } finally {
      saving = false;
    }
  }

  function openBrowser() {
    browseOpen = true;
    browse(musicDir || '/');
    loadShortcuts();
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

  let connectedDevice = $state('');
  let deviceLoaded = $state(false);
  let disconnecting = $state(false);

  async function loadDeviceInfo() {
    try {
      const config = await api.config();
      connectedDevice = config.connected_device || '';
    } catch {
      // ignore
    } finally {
      deviceLoaded = true;
    }
  }

  async function disconnect() {
    if (disconnecting) return;
    disconnecting = true;
    try {
      await api.disconnectDevice();
      onDisconnect?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Disconnect failed';
    } finally {
      disconnecting = false;
    }
  }

  async function rescanLibrary() {
    error = '';
    try {
      await api.rescanLibrary();
      onStartPolling?.();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Rescan failed';
    }
  }

  loadConfig();
  loadDeletedCount();
  loadDeviceInfo();
</script>

<div class="settings">
  {#if error}
    <div class="error">{error}</div>
  {/if}

  <!-- Music Directory -->
  <button class="settings-item" onclick={() => browseOpen ? browseOpen = false : openBrowser()}>
    <FolderOpen size={20} />
    <div class="item-content">
      <span class="item-label">Music Directory</span>
      <span class="item-value">{musicDir || 'Not set'}</span>
    </div>
    {#if browseOpen}
      <ChevronUp size={18} />
    {:else}
      <ChevronDown size={18} />
    {/if}
  </button>

  {#if browseOpen}
    <div class="browser">
      {#if shortcuts.length > 0 && currentPath === '/'}
        <div class="shortcuts">
          {#each shortcuts as sc}
            <button class="dir-item shortcut" onclick={() => browse(sc.path)}>
              <Zap size={18} />
              <span class="dir-name">{sc.name}</span>
            </button>
          {/each}
        </div>
      {/if}

      <div class="browser-path">{currentPath}</div>

      <div class="dir-list">
        {#if currentPath !== '/'}
          <button class="dir-item" onclick={() => browse(parentPath)}>
            <ArrowUp size={18} />
            <span class="dir-name">..</span>
          </button>
        {/if}

        {#if loading}
          <div class="loading">Loading...</div>
        {:else}
          {#each entries as entry}
            <button class="dir-item" onclick={() => browse(entry.path)}>
              <Folder size={18} />
              <span class="dir-name">{entry.name}</span>
            </button>
          {/each}

          {#if entries.length === 0}
            <div class="empty">No subdirectories</div>
          {/if}
        {/if}
      </div>

      <button class="select-btn" onclick={selectDir} disabled={saving || scanning || currentPath === musicDir}>
        {saving ? 'Saving...' : scanning ? 'Scanning...' : currentPath === musicDir ? 'Already selected' : `Use "${currentPath.split('/').pop() || currentPath}"`}
      </button>
    </div>
  {/if}

  {#if musicDir && !browseOpen}
    <button class="settings-item rescan-item" onclick={rescanLibrary} disabled={scanning}>
      <RefreshCw size={20} />
      <div class="item-content">
        <span class="item-label">{scanning ? 'Rescanning...' : 'Rescan Library'}</span>
        <span class="item-value">Force re-read all metadata</span>
      </div>
    </button>
  {/if}

  {#if flacalyzerAvailable}
    <button class="settings-item" onclick={toggleFlacalyzer}>
      <AudioLines size={20} />
      <div class="item-content">
        <span class="item-label">Transcode Detection</span>
        <span class="item-value">
          {#if analyzing}
            Analyzing: {analyzed} / {analysisTotal} files
          {:else}
            Flag fake lossless files with flacalyzer
          {/if}
        </span>
        {#if analyzing && analysisTotal > 0}
          <div class="analysis-bar">
            <div class="analysis-fill" style="width: {Math.round((analyzed / analysisTotal) * 100)}%"></div>
          </div>
        {/if}
      </div>
      <div class="toggle" class:on={flacalyzerEnabled}>
        <div class="toggle-knob"></div>
      </div>
    </button>
  {/if}

  <div class="section-divider"></div>

  <!-- Marked for Deletion -->
  {#if onOpenDeleted}
    <button class="settings-item" onclick={onOpenDeleted}>
      <Trash2 size={20} />
      <div class="item-content">
        <span class="item-label">Marked for Deletion</span>
        <span class="item-value">{deletedCount} file{deletedCount !== 1 ? 's' : ''}</span>
      </div>
      {#if deletedCount > 0}
        <span class="badge">{deletedCount}</span>
      {/if}
    </button>
  {/if}

  <!-- Audio Device -->
  <div class="section-divider"></div>
  {#if deviceLoaded}
    <div class="settings-item device-item">
      <Speaker size={20} />
      <div class="item-content">
        <span class="item-label">Audio Device</span>
        <span class="item-value">{connectedDevice || 'Not connected'}</span>
      </div>
      {#if disconnecting}
        <button class="disconnect-btn" disabled aria-label="Disconnecting">
          <span class="spinner"></span>
        </button>
      {:else if connectedDevice && onDisconnect}
        <button class="disconnect-btn" onclick={disconnect}>
          <Unplug size={14} />
          <span>Disconnect</span>
        </button>
      {:else if onSelectDevice}
        <button class="select-device-btn" onclick={onSelectDevice}>
          <span>Select</span>
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .settings {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 1rem;
    gap: 2px;
    overflow-y: auto;
  }

  .settings-item {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    background: var(--color-bg-card);
    border: none;
    border-radius: 12px;
    padding: 1rem;
    color: var(--color-text);
    font-size: 1rem;
    cursor: pointer;
    text-align: left;
    width: 100%;
  }

  .settings-item:hover {
    background: var(--color-bg-hover);
  }

  .device-item {
    cursor: default;
  }

  .device-item:hover {
    background: var(--color-bg-card);
  }

  .item-content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .item-label {
    font-size: 0.95rem;
    font-weight: 500;
  }

  .item-value {
    font-size: 0.75rem;
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .badge {
    background: var(--color-danger);
    color: white;
    font-size: 0.7rem;
    font-weight: 700;
    padding: 0.15rem 0.5rem;
    border-radius: 10px;
    min-width: 1.5rem;
    text-align: center;
  }

  .section-divider {
    height: 1px;
    background: var(--color-bg-hover);
    margin: 0.75rem 0;
  }

  /* Browser section */
  .browser {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.5rem;
    background: var(--color-bg);
    border-radius: 0 0 12px 12px;
    margin-top: -2px;
  }

  .browser-path {
    font-size: 0.75rem;
    color: var(--color-text-secondary);
    word-break: break-all;
    padding: 0 0.5rem;
  }

  .dir-list {
    max-height: 250px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .dir-item {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    background: var(--color-bg-card);
    border: none;
    border-radius: 6px;
    padding: 0.6rem 0.75rem;
    color: var(--color-text);
    font-size: 0.9rem;
    cursor: pointer;
    text-align: left;
  }

  .dir-item:hover {
    background: #252525;
  }

  .dir-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .shortcuts {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .shortcut {
    border-left: 3px solid var(--color-secondary);
  }

  .select-btn {
    width: 100%;
    background: var(--color-secondary);
    border: none;
    color: white;
    padding: 0.75rem;
    border-radius: 10px;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
  }

  .select-btn:hover {
    background: var(--color-secondary-hover);
  }

  .select-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .disconnect-btn {
    display: flex;
    align-items: center;
    gap: 0.3rem;
    background: #333;
    border: none;
    color: var(--color-danger-hover);
    cursor: pointer;
    font-size: 0.8rem;
    padding: 0.4rem 0.8rem;
    border-radius: 16px;
    font-weight: 600;
    flex-shrink: 0;
  }

  .disconnect-btn:hover {
    background: #444;
  }

  .select-device-btn {
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
    flex-shrink: 0;
  }

  .select-device-btn:hover {
    background: var(--color-secondary-hover);
  }

  .loading, .empty {
    text-align: center;
    padding: 1rem;
    color: var(--color-text-secondary);
    font-size: 0.85rem;
  }

  .analysis-bar {
    width: 100%;
    height: 3px;
    background: #333;
    border-radius: 2px;
    margin-top: 0.3rem;
    overflow: hidden;
  }

  .analysis-fill {
    height: 100%;
    background: var(--color-accent, #4ec484);
    border-radius: 2px;
    transition: width 0.3s ease;
  }

  .toggle {
    width: 44px;
    height: 24px;
    background: #444;
    border-radius: 12px;
    position: relative;
    flex-shrink: 0;
    transition: background 0.2s;
  }

  .toggle.on {
    background: var(--color-accent, #4ec484);
  }

  .toggle-knob {
    width: 20px;
    height: 20px;
    background: white;
    border-radius: 50%;
    position: absolute;
    top: 2px;
    left: 2px;
    transition: transform 0.2s;
  }

  .toggle.on .toggle-knob {
    transform: translateX(20px);
  }

  .error {
    color: var(--color-danger-hover);
    font-size: 0.85rem;
    padding: 0.5rem;
    text-align: center;
  }

  .spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid currentColor;
    border-top-color: transparent;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    vertical-align: middle;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
