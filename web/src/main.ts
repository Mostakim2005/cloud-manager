import './styles/tokens.css';
import './styles/app.css';
import { apiRequest, subscribeTelemetry } from './api';
import { iconEl, type IconName } from './icons';
import { applyDensity, applyTheme, getSavedDensity, getSavedTheme, initThemeListeners } from './theme';
import type { Account, TelemetrySnapshot, ViewId } from './types';
import { el, formatBytes, openModalDialog, showToast } from './ui';
import { renderBackplaneView } from './views/backplane';
import { renderExplorerView } from './views/explorer';
import { renderPhoneView } from './views/phone';
import { renderRulesView } from './views/rules';
import { renderSettingsView } from './views/settings';
import { renderYouTubeView } from './views/youtube';

interface NavAction {
  id: ViewId;
  label: string;
  icon: IconName;
  shortcut: string;
}

const NAV_ITEMS: NavAction[] = [
  { id: 'explorer', label: 'Virtual File Explorer', icon: 'folder', shortcut: 'Ctrl+1' },
  { id: 'rules', label: 'Smart Routing & Simulator', icon: 'zap', shortcut: 'Ctrl+2' },
  { id: 'phone', label: 'Phone Delta Sync (Wi-Fi/ADB)', icon: 'smartphone', shortcut: 'Ctrl+3' },
  { id: 'youtube', label: 'YouTube Private Pipeline', icon: 'youtube', shortcut: 'Ctrl+4' },
  { id: 'backplane', label: '.EXE Backplane & Duplicates', icon: 'terminal', shortcut: 'Ctrl+5' },
  { id: 'settings', label: 'Settings & Diagnostics', icon: 'settings', shortcut: 'Ctrl+6' },
];

export function bootstrapApp(): () => void {
  applyTheme(getSavedTheme(), false);
  applyDensity(getSavedDensity(), false);

  const brandIcon = document.getElementById('brand-icon');
  brandIcon?.replaceChildren(iconEl('cloud', 20));
  document.getElementById('btn-drawer-toggle')?.replaceChildren(iconEl('menu', 18));
  document.getElementById('btn-open-shortcuts')?.replaceChildren(iconEl('help', 18));
  document.getElementById('throughput-icon')?.replaceChildren(iconEl('activity', 14));
  document.getElementById('daemon-pill-icon')?.replaceChildren(iconEl('check', 14));

  let activeView: ViewId = parseInitialView();
  let cleanupView: (() => void) | null = null;
  let latestTelemetry: TelemetrySnapshot | null = null;

  const navContainer = document.getElementById('primary-nav');
  const viewHeading = document.getElementById('view-heading');
  const viewRoot = document.getElementById('view-root');
  const sidebar = document.getElementById('sidebar-drawer');
  const drawerBtn = document.getElementById('btn-drawer-toggle');

  const syncDensityBtn = () => {
    const d = getSavedDensity();
    const label = document.getElementById('density-label');
    const btn = document.getElementById('btn-density-toggle');
    if (label) label.textContent = d === 'compact' ? 'Compact' : 'Comfortable';
    btn?.setAttribute('aria-pressed', String(d === 'compact'));
  };
  syncDensityBtn();

  document.getElementById('btn-density-toggle')?.addEventListener('click', () => {
    const next = getSavedDensity() === 'compact' ? 'comfortable' : 'compact';
    applyDensity(next);
    syncDensityBtn();
    switchView(activeView);
  });

  drawerBtn?.addEventListener('click', () => {
    const open = sidebar?.classList.toggle('drawer-open') ?? false;
    drawerBtn.setAttribute('aria-expanded', String(open));
  });

  const renderSidebarAccounts = async () => {
    const stack = document.getElementById('sidebar-accounts');
    if (!stack) return;
    try {
      const res = await apiRequest<{ accounts: Account[]; stats?: { totalFiles: number; activeJobs: number } }>('/api/v1/status');
      stack.replaceChildren();
      for (const a of res.accounts ?? []) {
        const pct = a.totalBytes > 0 ? Math.min(100, Math.round((a.usedBytes / a.totalBytes) * 100)) : 0;
        const label = a.provider === 'youtube' ? 'Private Quota' : `${formatBytes(a.usedBytes)} / ${formatBytes(a.totalBytes)}`;
        stack.append(
          el(
            'div',
            { class: 'account-card' },
            el('div', { class: 'account-row' }, el('span', {}, a.name), el('strong', {}, label)),
            el('div', { class: 'meter' }, el('div', { class: `meter-fill${pct > 85 ? ' warn' : ''}`, style: `width:${pct}%` })),
          ),
        );
      }
      if (res.stats) {
        const cat = document.getElementById('catalog-stat');
        const q = document.getElementById('queue-stat');
        if (cat) cat.textContent = `${res.stats.totalFiles.toLocaleString()} indexed`;
        if (q) q.textContent = `${res.stats.activeJobs} active jobs`;
      }
    } catch {
      // Daemon offline; leave last state or placeholder.
    }
  };

  document.getElementById('btn-refresh-status')?.addEventListener('click', () => {
    void renderSidebarAccounts();
    showToast('Storage pool status refreshed.');
  });

  const switchView = (id: ViewId) => {
    activeView = id;
    if (cleanupView) {
      cleanupView();
      cleanupView = null;
    }
    const item = NAV_ITEMS.find((n) => n.id === id) ?? NAV_ITEMS[0];
    if (viewHeading) viewHeading.textContent = item.label;
    document.title = `${item.label} | Cloud Manager`;

    if (navContainer) {
      for (const btn of navContainer.querySelectorAll('.nav-btn')) {
        btn.setAttribute('aria-current', btn.getAttribute('data-view') === id ? 'page' : 'false');
      }
    }
    sidebar?.classList.remove('drawer-open');
    drawerBtn?.setAttribute('aria-expanded', 'false');

    if (!viewRoot) return;
    switch (id) {
      case 'explorer':
        cleanupView = renderExplorerView(viewRoot);
        break;
      case 'rules':
        cleanupView = renderRulesView(viewRoot);
        break;
      case 'phone':
        cleanupView = renderPhoneView(viewRoot);
        break;
      case 'youtube':
        cleanupView = renderYouTubeView(viewRoot);
        break;
      case 'backplane':
        cleanupView = renderBackplaneView(viewRoot, () => latestTelemetry);
        break;
      case 'settings':
        cleanupView = renderSettingsView(viewRoot, syncDensityBtn);
        break;
    }
  };

  if (navContainer) {
    navContainer.replaceChildren();
    for (const item of NAV_ITEMS) {
      const btn = el(
        'button',
        {
          type: 'button',
          class: 'nav-btn',
          'data-view': item.id,
          'aria-current': item.id === activeView ? 'page' : 'false',
        },
        iconEl(item.icon, 18),
        el('span', {}, item.label),
      );
      btn.addEventListener('click', () => switchView(item.id));
      navContainer.append(btn);
    }
  }

  // Command Quick-Picker (Ctrl+K)
  const cmdDialog = document.getElementById('command-dialog') as HTMLDialogElement | null;
  const cmdSearch = document.getElementById('cmd-search') as HTMLInputElement | null;
  const cmdList = document.getElementById('cmd-list');
  const openCommandPalette = () => {
    if (!cmdDialog || !cmdSearch || !cmdList) return;
    const renderCmds = (q: string) => {
      const lq = q.trim().toLowerCase();
      const filtered = NAV_ITEMS.filter((i) => i.label.toLowerCase().includes(lq) || i.id.includes(lq));
      cmdList.replaceChildren();
      filtered.forEach((item, idx) => {
        const btn = el(
          'button',
          { type: 'button', class: 'cmd-item-btn', role: 'option', 'aria-selected': String(idx === 0) },
          el('span', {}, item.label),
          el('kbd', {}, item.shortcut),
        );
        btn.addEventListener('click', () => {
          cmdDialog.close();
          switchView(item.id);
        });
        cmdList.append(el('li', {}, btn));
      });
    };
    cmdSearch.value = '';
    renderCmds('');
    cmdSearch.oninput = () => renderCmds(cmdSearch.value);
    openModalDialog(cmdDialog);
    cmdSearch.focus();
  };
  document.getElementById('btn-open-palette')?.addEventListener('click', openCommandPalette);

  const openShortcuts = () => {
    const dlg = document.getElementById('shortcuts-dialog') as HTMLDialogElement | null;
    if (!dlg) return;
    openModalDialog(dlg);
  };
  document.getElementById('btn-open-shortcuts')?.addEventListener('click', openShortcuts);

  const onGlobalKey = (ev: KeyboardEvent) => {
    if (ev.ctrlKey && ev.key.toLowerCase() === 'k') {
      ev.preventDefault();
      openCommandPalette();
      return;
    }
    if (ev.ctrlKey && ev.key >= '1' && ev.key <= '6') {
      ev.preventDefault();
      const idx = Number(ev.key) - 1;
      if (NAV_ITEMS[idx]) switchView(NAV_ITEMS[idx].id);
      return;
    }
    const targetTag = (ev.target as HTMLElement)?.tagName;
    if (targetTag === 'INPUT' || targetTag === 'TEXTAREA' || targetTag === 'SELECT') return;
    if (ev.key === '?') {
      ev.preventDefault();
      openShortcuts();
    } else if (ev.key === '/' || (ev.ctrlKey && ev.key.toLowerCase() === 'f')) {
      const search = document.getElementById('explorer-search') as HTMLInputElement | null;
      if (search) {
        ev.preventDefault();
        search.focus();
      }
    }
  };
  document.addEventListener('keydown', onGlobalKey);

  const stopTheme = initThemeListeners(() => {
    if (activeView === 'settings') switchView('settings');
  });

  const stopSSE = subscribeTelemetry(
    (snap) => {
      latestTelemetry = snap;
      const mbps = (snap.totalSpeedBps / (1024 * 1024)).toFixed(1);
      const tEl = document.getElementById('throughput-text');
      const qEl = document.getElementById('queue-stat');
      if (tEl) tEl.textContent = `${mbps} MB/s`;
      if (qEl) qEl.textContent = `${snap.activeJobs.length} active jobs`;
    },
    (online) => {
      const pill = document.getElementById('daemon-pill');
      const txt = document.getElementById('daemon-pill-text');
      if (pill && txt) {
        pill.className = `status-badge ${online ? 'status-success' : 'status-warning'}`;
        txt.textContent = online ? 'Daemon Online' : 'Offline';
      }
    },
  );

  switchView(activeView);
  void renderSidebarAccounts();

  return () => {
    document.removeEventListener('keydown', onGlobalKey);
    stopTheme();
    stopSSE();
    if (cleanupView) cleanupView();
  };
}

function parseInitialView(): ViewId {
  const params = new URLSearchParams(window.location.search);
  const v = params.get('view') as ViewId | null;
  if (v && NAV_ITEMS.some((n) => n.id === v)) return v;
  return 'explorer';
}

if (typeof document !== 'undefined' && document.getElementById('view-root')) {
  bootstrapApp();
}
