<script lang="ts">
  import { FolderOpen, Disc3, Settings } from 'lucide-svelte';

  type Tab = 'folders' | 'player' | 'settings';

  let { activeTab, onTabChange }: {
    activeTab: Tab;
    onTabChange: (tab: Tab) => void;
  } = $props();
</script>

<nav class="bottom-nav">
  <button
    class="nav-tab"
    class:active={activeTab === 'folders'}
    onclick={() => onTabChange('folders')}
    aria-label="Folders"
    style="--tab-color: #1db954"
  >
    <FolderOpen size={22} />
    <span class="nav-label">Folders</span>
  </button>

  <button
    class="nav-tab"
    class:active={activeTab === 'player'}
    onclick={() => onTabChange('player')}
    aria-label="Now Playing"
    style="--tab-color: #4ec484"
  >
    <Disc3 size={22} />
    <span class="nav-label">Now Playing</span>
  </button>

  <button
    class="nav-tab"
    class:active={activeTab === 'settings'}
    onclick={() => onTabChange('settings')}
    aria-label="Settings"
    style="--tab-color: #7cb3ff"
  >
    <Settings size={22} />
    <span class="nav-label">Settings</span>
  </button>
</nav>

<style>
  .bottom-nav {
    display: flex;
    justify-content: space-around;
    align-items: center;
    padding: 0.4rem 0 0.5rem;
    background: #0d0d0d;
    border-top: 1px solid #222;
    flex-shrink: 0;
  }

  .nav-tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
    background: none;
    border: none;
    color: #555;
    cursor: pointer;
    padding: 0.25rem 1rem;
    border-radius: 8px;
    font-size: 0.65rem;
    transition: color 0.15s;
  }

  .nav-tab.active {
    color: var(--tab-color);
  }

  .nav-tab:hover:not(.active) {
    color: #888;
  }

  .nav-label {
    font-weight: 500;
  }
</style>
