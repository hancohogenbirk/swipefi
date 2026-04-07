<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { connectWebSocket, disconnectWebSocket, loadInitialState, getPlayerState } from './lib/stores/player.svelte';
  import { api, type Device } from './lib/api/client';
  import FolderNav from './lib/components/FolderNav.svelte';
  import NowPlaying from './lib/components/NowPlaying.svelte';
  import Settings from './lib/components/Settings.svelte';
  import QueueView from './lib/components/QueueView.svelte';
  import DeletedManager from './lib/components/DeletedManager.svelte';
  import BottomNav from './lib/components/BottomNav.svelte';
  import MiniPlayer from './lib/components/MiniPlayer.svelte';

  const SCAN_POLL_INTERVAL_MS = 500;
  const SESSION_KEY_TAB = 'swipefi-tab';

  type AppPhase = 'loading' | 'choose-dir' | 'setup' | 'main';
  type Tab = 'folders' | 'player' | 'settings';

  let appPhase = $state<AppPhase>('loading');
  let savedTab = (typeof sessionStorage !== 'undefined' ? sessionStorage.getItem(SESSION_KEY_TAB) : null) as Tab | null;
  let activeTab = $state<Tab>(savedTab || 'folders');
  let showQueue = $state(false);
  let showDeletedManager = $state(false);

  // Persist active tab across refreshes
  $effect(() => {
    sessionStorage.setItem(SESSION_KEY_TAB, activeTab);
  });

  let devices = $state<Device[]>([]);
  let selectedDevice = $state('');
  let error = $state('');

  let playerState = $derived(getPlayerState());
  let scanProgress = $state({ scanning: false, scanned: 0, total: 0, phase: '' });
  let scanPollTimer: ReturnType<typeof setInterval> | null = null;

  // --- History API for back button ---
  let folderHistory = $state<string[]>([]);
  let folderGoBackSignal = $state(0);
  let folderRefreshSignal = $state(0);

  function pushFolderHistory(path: string) {
    folderHistory = [...folderHistory, path];
    history.pushState({ type: 'folder', path }, '');
  }

  function pushQueueHistory() {
    history.pushState({ type: 'queue' }, '');
  }

  let showExitConfirm = $state(false);

  function handlePopState(e: PopStateEvent) {
    // Queue sub-view: go back to now playing
    if (showQueue) {
      showQueue = false;
      return;
    }

    // Deleted manager sub-view: go back to settings
    if (showDeletedManager) {
      showDeletedManager = false;
      return;
    }

    // Folder navigation: go back to parent folder
    if (activeTab === 'folders' && folderHistory.length > 0) {
      folderHistory = folderHistory.slice(0, -1);
      folderGoBackSignal++;
      return;
    }

    // At tab root — show exit confirmation
    // Push a state back so we don't actually leave
    history.pushState(null, '');
    showExitConfirm = true;
  }

  onMount(async () => {
    // Register history/back button handler immediately so it works on all paths
    window.addEventListener('popstate', handlePopState);
    history.pushState(null, '');

    try {
      const config = await api.config();
      connectWebSocket();

      if (!config.music_dir) {
        appPhase = 'choose-dir';
        return;
      }

      await loadInitialState();

      // Always check and poll scan progress
      scanProgress = await api.scanStatus();
      if (scanProgress.scanning) {
        startScanPolling();
      }

      // Discover devices
      devices = await api.devices();
      if (devices.length === 0) {
        try {
          devices = await api.scanDevices();
        } catch {
          // scan can be slow, don't block
        }
      }

      // Music dir is configured and device found — go to main app
      // Restore the last active tab from sessionStorage
      if (devices.length > 0) {
        selectedDevice = devices[0].udn;
        appPhase = 'main';
        // activeTab is already set from sessionStorage or defaults to 'folders'
      } else {
        appPhase = 'setup';
      }
    } catch (e) {
      console.error('[swipefi] init error:', e);
      appPhase = 'choose-dir';
    }
  });

  onDestroy(() => {
    disconnectWebSocket();
    stopScanPolling();
    window.removeEventListener('popstate', handlePopState);
  });

  async function pollScanOnce() {
    try {
      const prev = scanProgress.scanning;
      scanProgress = await api.scanStatus();
      if (!scanProgress.scanning) {
        stopScanPolling();
        // Refresh folders when scan finishes
        if (prev) {
          folderRefreshSignal++;
        }
      }
    } catch { /* ignore */ }
  }

  function startScanPolling() {
    stopScanPolling();
    // Set scanning immediately so UI reacts before the first poll returns
    scanProgress = { ...scanProgress, scanning: true };
    // Poll once immediately (no 500ms blind window), then every 500ms
    pollScanOnce();
    scanPollTimer = setInterval(pollScanOnce, SCAN_POLL_INTERVAL_MS);
  }

  function stopScanPolling() {
    if (scanPollTimer) {
      clearInterval(scanPollTimer);
      scanPollTimer = null;
    }
  }

  async function onMusicDirChosen() {
    appPhase = 'loading';
    startScanPolling();
    devices = await api.devices();
    if (devices.length === 0) {
      try { devices = await api.scanDevices(); } catch { /* */ }
    }

    if (devices.length === 0) {
      appPhase = 'setup';
    } else if (devices.length === 1) {
      await selectDevice(devices[0].udn);
    } else {
      appPhase = 'setup';
    }
  }

  async function selectDevice(udn: string) {
    try {
      await api.selectDevice(udn);
      selectedDevice = udn;
      // Check if scan is still running before going to folders
      scanProgress = await api.scanStatus();
      if (scanProgress.scanning) {
        startScanPolling();
      }
      appPhase = 'main';
      error = '';
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to select device';
    }
  }

  async function rescan() {
    error = '';
    try {
      devices = await api.scanDevices();
      if (devices.length === 0) {
        error = 'No DLNA renderers found. Is your device powered on?';
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Scan failed';
    }
  }
</script>

<div class="app">
  {#if appPhase === 'loading'}
    <div class="center-screen">
      <h1 class="logo">SwipeFi</h1>
      {#if scanProgress.scanning && scanProgress.total > 0}
        <div class="scan-progress">
          <div class="scan-bar">
            <div class="scan-fill" style="width: {Math.round((scanProgress.scanned / scanProgress.total) * 100)}%"></div>
          </div>
          <p class="scan-text">Scanning library: {scanProgress.scanned} / {scanProgress.total} files</p>
        </div>
      {:else if scanProgress.scanning}
        <p class="subtitle">Scanning library...</p>
      {:else}
        <p class="subtitle">Loading...</p>
      {/if}
    </div>

  {:else if appPhase === 'choose-dir'}
    <Settings onDone={onMusicDirChosen} />

  {:else if appPhase === 'setup'}
    <div class="center-screen">
      <h1 class="logo">SwipeFi</h1>

      {#if scanProgress.scanning && scanProgress.total > 0}
        <div class="scan-progress">
          <div class="scan-bar">
            <div class="scan-fill" style="width: {Math.round((scanProgress.scanned / scanProgress.total) * 100)}%"></div>
          </div>
          <p class="scan-text">Scanning: {scanProgress.scanned} / {scanProgress.total}</p>
        </div>
      {:else if scanProgress.scanning}
        <p class="subtitle">Scanning library...</p>
      {/if}

      <p class="subtitle">Select your audio renderer</p>

      {#if error}
        <p class="error">{error}</p>
      {/if}

      <div class="device-list">
        {#each devices as device}
          <button class="device-btn" onclick={() => selectDevice(device.udn)}>
            {device.name}
          </button>
        {/each}
      </div>

      <button class="scan-btn" onclick={rescan}>
        Scan for devices
      </button>
    </div>

  {:else}
    <!-- Main app with tabs -->
    {#if scanProgress.scanning}
      <div class="scan-banner">
        <div class="scan-bar">
          <div class="scan-fill" style="width: {scanProgress.phase === 'cleanup' ? 100 : scanProgress.total ? Math.round((scanProgress.scanned / scanProgress.total) * 100) : 0}%"></div>
        </div>
        <span class="scan-banner-text">
          {#if scanProgress.phase === 'cleanup'}
            Updating library...
          {:else if scanProgress.phase === 'counting'}
            Counting files...
          {:else if scanProgress.total > 0}
            Scanning: {scanProgress.scanned} / {scanProgress.total}
          {:else}
            Scanning library...
          {/if}
        </span>
      </div>
    {/if}
    <div class="tab-content" class:scanning={scanProgress.scanning}>
      <div class="tab-panel" class:hidden={activeTab !== 'folders'}>
        <FolderNav
          onNavigateToPlayer={() => activeTab = 'player'}
          onFolderNavigate={pushFolderHistory}
          goBackSignal={folderGoBackSignal}
          refreshSignal={folderRefreshSignal}
        />
      </div>

      <div class="tab-panel" class:hidden={activeTab !== 'player'}>
        {#if showQueue}
          <QueueView onBack={() => showQueue = false} />
        {:else}
          <NowPlaying onOpenQueue={() => { showQueue = true; pushQueueHistory(); }} />
        {/if}
      </div>

      <div class="tab-panel" class:hidden={activeTab !== 'settings'}>
        {#if showDeletedManager}
          <DeletedManager onBack={() => showDeletedManager = false} />
        {:else}
          <Settings onDone={() => { startScanPolling(); activeTab = 'folders'; }} onOpenDeleted={() => { showDeletedManager = true; history.pushState({ type: 'deleted' }, ''); }} onDisconnect={() => { appPhase = 'setup'; }} onSelectDevice={() => { appPhase = 'setup'; }} visible={activeTab === 'settings' && !showDeletedManager} scanning={scanProgress.scanning} />
        {/if}
      </div>
    </div>

    {#if activeTab !== 'player'}
      <MiniPlayer onClick={() => activeTab = 'player'} />
    {/if}
    <BottomNav {activeTab} onTabChange={(tab) => { activeTab = tab; showQueue = false; showDeletedManager = false; }} />

    {#if showExitConfirm}
      <div class="exit-overlay" role="button" tabindex="-1" onclick={() => showExitConfirm = false} onkeydown={(e) => { if (e.key === 'Escape') showExitConfirm = false; }}>
        <div class="exit-dialog" role="presentation" onclick={(e) => e.stopPropagation()}>
          <p>Leave SwipeFi?</p>
          <div class="exit-actions">
            <button class="exit-cancel" onclick={() => showExitConfirm = false}>Cancel</button>
            <button class="exit-leave" onclick={() => history.back()}>Leave</button>
          </div>
        </div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .app {
    height: 100dvh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .center-screen {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    padding: 2rem;
    gap: 1rem;
  }

  .logo {
    font-size: 2.5rem;
    font-weight: 800;
    background: linear-gradient(135deg, #1db954, #7cb3ff);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
  }

  .subtitle {
    color: var(--color-text-secondary);
    font-size: 1rem;
  }

  .scan-progress {
    width: 100%;
    max-width: 300px;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .scan-bar {
    height: 6px;
    background: #333;
    border-radius: 3px;
    overflow: hidden;
  }

  .scan-fill {
    height: 100%;
    background: linear-gradient(90deg, #1db954, #7cb3ff);
    border-radius: 3px;
    transition: width 0.3s ease;
  }

  .scan-text {
    color: var(--color-text-secondary);
    font-size: 0.85rem;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  .scan-banner {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
    background: var(--color-bg-card);
    border-bottom: 1px solid var(--color-bg-hover);
    flex-shrink: 0;
  }

  .scan-banner .scan-bar {
    flex: 1;
  }

  .scan-banner-text {
    font-size: 0.75rem;
    color: var(--color-text-secondary);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .error {
    color: var(--color-danger-hover);
    font-size: 0.9rem;
    text-align: center;
  }

  .device-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    width: 100%;
    max-width: 300px;
  }

  .device-btn {
    background: var(--color-bg-card);
    border: 1px solid #333;
    color: var(--color-text);
    padding: 1rem;
    border-radius: 12px;
    font-size: 1rem;
    cursor: pointer;
  }

  .device-btn:hover {
    border-color: var(--color-primary);
    background: #1e1e1e;
  }

  .scan-btn {
    background: none;
    border: 1px solid #444;
    color: var(--color-text-secondary);
    padding: 0.75rem 1.5rem;
    border-radius: 24px;
    cursor: pointer;
    font-size: 0.9rem;
    margin-top: 1rem;
  }

  .scan-btn:hover {
    border-color: #666;
    color: #aaa;
  }

  .tab-content {
    flex: 1;
    min-height: 0;
    position: relative;
  }

  .tab-content.scanning {
    opacity: 0.4;
    pointer-events: none;
  }

  .tab-panel {
    height: 100%;
    overflow: hidden;
  }

  .tab-panel.hidden {
    display: none;
  }

  .exit-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }

  .exit-dialog {
    background: #1e1e1e;
    border-radius: 16px;
    padding: 1.5rem 2rem;
    text-align: center;
    min-width: 250px;
  }

  .exit-dialog p {
    font-size: 1.1rem;
    margin: 0 0 1.25rem;
  }

  .exit-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: center;
  }

  .exit-cancel {
    background: #333;
    border: none;
    color: var(--color-text);
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
  }

  .exit-leave {
    background: var(--color-danger);
    border: none;
    color: white;
    padding: 0.6rem 1.5rem;
    border-radius: 24px;
    font-size: 0.95rem;
    cursor: pointer;
  }
</style>
