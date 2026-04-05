<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { connectWebSocket, disconnectWebSocket, loadInitialState, getPlayerState } from './lib/stores/player.svelte';
  import { api, type Device } from './lib/api/client';
  import FolderNav from './lib/components/FolderNav.svelte';
  import NowPlaying from './lib/components/NowPlaying.svelte';
  import Settings from './lib/components/Settings.svelte';
  import QueueView from './lib/components/QueueView.svelte';

  type View = 'loading' | 'choose-dir' | 'setup' | 'folders' | 'player' | 'queue' | 'settings';

  let previousView = $state<View>('folders');

  let view = $state<View>('loading');
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
        view = 'choose-dir';
        return;
      }

      await loadInitialState();

      // Check if a scan is running
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
        view = 'player';
        return;
      }

      // Otherwise show device selection
      view = 'setup';
    } catch (e) {
      console.error('[swipefi] init error:', e);
      view = 'choose-dir';
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
    view = 'loading';
    startScanPolling();
    devices = await api.devices();
    if (devices.length === 0) {
      try { devices = await api.scanDevices(); } catch { /* */ }
    }

    if (devices.length === 0) {
      view = 'setup';
    } else if (devices.length === 1) {
      await selectDevice(devices[0].udn);
    } else {
      view = 'setup';
    }
  }

  async function selectDevice(udn: string) {
    try {
      await api.selectDevice(udn);
      selectedDevice = udn;
      view = 'folders';
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

  function navigateToPlayer() {
    view = 'player';
  }

  function navigateToFolders() {
    view = 'folders';
  }

  function navigateToHome() {
    view = 'setup';
  }

  function openSettings() {
    previousView = view as View;
    view = 'settings';
  }

  function closeSettings() {
    // After settings change, always go to folders (music dir may have changed)
    view = 'folders';
  }
</script>

<div class="app">
  {#if view === 'loading'}
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

  {:else if view === 'choose-dir'}
    <Settings onDone={onMusicDirChosen} />

  {:else if view === 'setup'}
    <div class="center-screen">
      <h1 class="logo">SwipeFi</h1>
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

  {:else if view === 'settings'}
    <Settings onDone={closeSettings} />

  {:else if view === 'folders'}
    <FolderNav onNavigateToPlayer={navigateToPlayer} onOpenSettings={openSettings} onNavigateHome={navigateToHome} />

    {#if playerState.state !== 'idle' && playerState.track}
      <button class="mini-player" onclick={navigateToPlayer}>
        <div class="mini-info">
          <span class="mini-title">{playerState.track.title}</span>
          <span class="mini-artist">{playerState.track.artist || 'Unknown'}</span>
        </div>
        <span class="mini-state">{playerState.state === 'playing' ? '▶' : '⏸'}</span>
      </button>
    {/if}

  {:else if view === 'player'}
    <NowPlaying onBack={navigateToFolders} onOpenQueue={() => view = 'queue'} />

  {:else if view === 'queue'}
    <QueueView onBack={() => view = 'player'} />
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

  .mini-player {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: #1a1a1a;
    border: none;
    border-top: 1px solid #333;
    color: #f0f0f0;
    padding: 0.75rem 1rem;
    cursor: pointer;
    width: 100%;
    text-align: left;
  }

  .mini-player:hover {
    background: #222;
  }

  .mini-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
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
    color: #888;
  }

  .mini-state {
    font-size: 1.2rem;
    flex-shrink: 0;
  }
</style>
