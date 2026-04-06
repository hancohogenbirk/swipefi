<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { connectWebSocket, disconnectWebSocket, loadInitialState, getPlayerState } from './lib/stores/player.svelte';
  import { api, type Device } from './lib/api/client';
  import FolderNav from './lib/components/FolderNav.svelte';
  import NowPlaying from './lib/components/NowPlaying.svelte';
  import Settings from './lib/components/Settings.svelte';
  import QueueView from './lib/components/QueueView.svelte';
  import BottomNav from './lib/components/BottomNav.svelte';
  import MiniPlayer from './lib/components/MiniPlayer.svelte';

  type AppPhase = 'loading' | 'choose-dir' | 'setup' | 'main';
  type Tab = 'folders' | 'player' | 'settings';

  let appPhase = $state<AppPhase>('loading');
  let activeTab = $state<Tab>('folders');
  let showQueue = $state(false);

  let devices = $state<Device[]>([]);
  let selectedDevice = $state('');
  let error = $state('');

  let playerState = $derived(getPlayerState());
  let scanProgress = $state({ scanning: false, scanned: 0, total: 0 });
  let scanPollTimer: ReturnType<typeof setInterval> | null = null;

  onMount(async () => {
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

      // If something is already playing, go straight to Now Playing
      if (playerState.state !== 'idle' && playerState.track) {
        // Auto-select the first device if available (renderer is already set server-side)
        if (devices.length > 0) {
          selectedDevice = devices[0].udn;
        }
        appPhase = 'main';
        activeTab = 'player';
        return;
      }

      // Otherwise show device selection
      appPhase = 'setup';
    } catch (e) {
      console.error('[swipefi] init error:', e);
      appPhase = 'choose-dir';
    }
  });

  onDestroy(() => {
    disconnectWebSocket();
    stopScanPolling();
  });

  function startScanPolling() {
    stopScanPolling();
    scanPollTimer = setInterval(async () => {
      try {
        scanProgress = await api.scanStatus();
        if (!scanProgress.scanning) {
          stopScanPolling();
        }
      } catch { /* ignore */ }
    }, 500);
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
      activeTab = 'folders';
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
    <div class="tab-content">
      <div class="tab-panel" class:hidden={activeTab !== 'folders'}>
        {#if scanProgress.scanning}
          <div class="scan-banner">
            <div class="scan-bar">
              <div class="scan-fill" style="width: {scanProgress.total ? Math.round((scanProgress.scanned / scanProgress.total) * 100) : 0}%"></div>
            </div>
            <span class="scan-banner-text">
              {#if scanProgress.total > 0}
                Scanning: {scanProgress.scanned} / {scanProgress.total}
              {:else}
                Scanning library...
              {/if}
            </span>
          </div>
        {/if}
        <FolderNav onNavigateToPlayer={() => activeTab = 'player'} />
      </div>

      <div class="tab-panel" class:hidden={activeTab !== 'player'}>
        {#if showQueue}
          <QueueView onBack={() => showQueue = false} />
        {:else}
          <NowPlaying onBack={() => activeTab = 'folders'} onOpenQueue={() => showQueue = true} />
        {/if}
      </div>

      <div class="tab-panel" class:hidden={activeTab !== 'settings'}>
        <Settings onDone={() => activeTab = 'folders'} />
      </div>
    </div>

    {#if activeTab !== 'player'}
      <MiniPlayer onClick={() => activeTab = 'player'} />
    {/if}
    <BottomNav {activeTab} onTabChange={(tab) => { activeTab = tab; if (tab !== 'player') showQueue = false; }} />
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
    color: #888;
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
    color: #888;
    font-size: 0.85rem;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  .scan-banner {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 1rem;
    background: #1a1a1a;
    border-bottom: 1px solid #222;
    flex-shrink: 0;
  }

  .scan-banner .scan-bar {
    flex: 1;
  }

  .scan-banner-text {
    font-size: 0.75rem;
    color: #888;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .error {
    color: #ff6b6b;
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
    background: #1a1a1a;
    border: 1px solid #333;
    color: #f0f0f0;
    padding: 1rem;
    border-radius: 12px;
    font-size: 1rem;
    cursor: pointer;
  }

  .device-btn:hover {
    border-color: #1db954;
    background: #1e1e1e;
  }

  .scan-btn {
    background: none;
    border: 1px solid #444;
    color: #888;
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

  .tab-panel {
    height: 100%;
    overflow: hidden;
  }

  .tab-panel.hidden {
    display: none;
  }
</style>
