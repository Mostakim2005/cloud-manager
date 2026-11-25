import { Virtualizer, elementScroll, observeElementOffset, observeElementRect, type Rect } from '@tanstack/virtual-core';
import { apiRequest } from '../api';
import { iconEl } from '../icons';
import { getSavedDensity } from '../theme';
import type { CatalogFile, Job, TelemetrySnapshot } from '../types';
import { confirmAction, el, formatBytes, showQuickLookModal, showToast, statusBadge } from '../ui';

export function renderExplorerView(container: HTMLElement): () => void {
  let items: CatalogFile[] = [];
  let jobs: Job[] = [];
  let total = 0;
  let queryMs = 0;
  let loading = true;
  let errorMsg = '';
  let searchQuery = '';
  let providerFilter = 'all';
  let sortKey: 'name' | 'sizeBytes' | 'updatedAt' = 'updatedAt';
  let sortAsc = false;
  let selectedId: number | null = null;
  let unmountVirtual: (() => void) | null = null;
  let searchTimer: number | undefined;

  const progressCard = el('div', { class: 'card', id: 'live-progress-card' });
  const toolbar = el('div', { class: 'card' });
  const listCard = el('div', { class: 'card' });
  container.replaceChildren(progressCard, toolbar, listCard);

  const renderProgressCard = (snap?: TelemetrySnapshot) => {
    const activeTicks = snap?.activeJobs ?? [];
    const activeSQLJobs = jobs.filter((j) => j.status === 'RUNNING' || j.status === 'QUEUED' || j.status === 'PAUSED');
    const toggleBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('refresh', 14), 'Refresh Queue');
    toggleBtn.addEventListener('click', () => void fetchJobs());

    progressCard.replaceChildren(
      el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Live Transfer & Sync Progress'), toggleBtn),
    );

    if (activeTicks.length === 0 && activeSQLJobs.length === 0) {
      progressCard.append(el('p', { class: 'vcell-sub' }, 'Idle — 0 active transfers (daemon sleeping at ~0% CPU until triggered).'));
      return;
    }

    const stack = el('div', { class: 'account-stack' });
    for (const t of activeTicks) {
      const pct = Math.min(100, Math.round(t.percent || 0));
      stack.append(
        el(
          'div',
          { class: 'account-card' },
          el('div', { class: 'account-row' }, el('strong', {}, t.label), el('span', {}, `${pct}% · ${(t.speedBps / (1024 * 1024)).toFixed(1)} MB/s`)),
          el('div', { class: 'meter' }, el('div', { class: 'meter-fill', style: `width:${pct}%` })),
        ),
      );
    }
    for (const j of activeSQLJobs.slice(0, 6)) {
      if (activeTicks.some((t) => t.jobId === j.id)) continue;
      const pct = j.bytesTotal > 0 ? Math.min(100, Math.round((j.bytesDone / j.bytesTotal) * 100)) : 0;
      const act = j.status === 'PAUSED' ? 'resume' : 'pause';
      const btn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, act === 'pause' ? 'Pause' : 'Resume');
      btn.addEventListener('click', async () => {
        await apiRequest('/api/v1/jobs/action', { method: 'POST', body: JSON.stringify({ id: j.id, action: act }) });
        await fetchJobs();
      });
      stack.append(
        el(
          'div',
          { class: 'account-card' },
          el('div', { class: 'account-row' }, el('span', {}, `${j.kind}: ${j.sourcePath}`), el('span', {}, `${j.status} (${pct}%) `, btn)),
          el('div', { class: 'meter' }, el('div', { class: 'meter-fill', style: `width:${pct}%` })),
        ),
      );
    }
    progressCard.append(stack);
  };

  const fetchJobs = async () => {
    try {
      const res = await apiRequest<{ jobs: Job[] }>('/api/v1/jobs');
      jobs = res.jobs ?? [];
    } catch {
      jobs = [];
    }
    renderProgressCard();
  };

  const onTelemetry = (ev: Event) => {
    renderProgressCard((ev as CustomEvent<TelemetrySnapshot>).detail);
  };
  window.addEventListener('cm-telemetry', onTelemetry);
  renderProgressCard();
  void fetchJobs();

  const searchInput = el('input', {
    id: 'explorer-search',
    type: 'search',
    class: 'input',
    placeholder: 'Search catalog (FTS5 < 5 ms)…',
    'aria-label': 'Search catalog files',
  }) as HTMLInputElement;

  const resetBtn = el('button', { type: 'button', class: 'btn-ghost btn-sm' }, 'Reset filters');
  const activeCountLabel = el('span', { class: 'telemetry-stat', id: 'explorer-count' }, '');
  const chipRow = el('div', { class: 'chip-row' });

  const providers = [
    { id: 'all', label: 'All Pools' },
    { id: 'gdrive', label: 'Google Drive' },
    { id: 'mega', label: 'Mega Pool' },
    { id: 'youtube', label: 'YouTube Private' },
  ];

  const renderChips = () => {
    chipRow.replaceChildren();
    let activeCount = 0;
    if (providerFilter !== 'all') activeCount++;
    if (searchQuery.trim() !== '') activeCount++;

    for (const p of providers) {
      const btn = el(
        'button',
        {
          type: 'button',
          class: 'filter-chip',
          'aria-pressed': String(providerFilter === p.id),
        },
        p.label,
      );
      btn.addEventListener('click', () => {
        providerFilter = p.id;
        renderChips();
        void fetchCatalog();
      });
      chipRow.append(btn);
    }

    if (activeCount > 0) {
      resetBtn.textContent = `Reset filters (${activeCount})`;
      resetBtn.hidden = false;
    } else {
      resetBtn.hidden = true;
    }
    chipRow.append(resetBtn);
  };

  resetBtn.addEventListener('click', () => {
    providerFilter = 'all';
    searchQuery = '';
    searchInput.value = '';
    renderChips();
    void fetchCatalog();
  });

  searchInput.addEventListener('input', () => {
    searchQuery = searchInput.value;
    window.clearTimeout(searchTimer);
    searchTimer = window.setTimeout(() => {
      renderChips();
      void fetchCatalog();
    }, 120);
  });

  const searchLabel = el('label', { for: 'explorer-search', class: 'visually-hidden' }, 'Search catalog');
  const previewSelBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('eye', 14), 'Quick-Look (Space)');
  previewSelBtn.addEventListener('click', () => {
    const chosen = items.find((i) => i.id === selectedId) ?? items[0];
    if (chosen) {
      void openPreview(chosen);
    } else {
      showToast('Select a file row first to Quick-Look.');
    }
  });

  toolbar.append(
    el('div', { class: 'toolbar' }, searchLabel, searchInput, previewSelBtn, activeCountLabel),
    chipRow,
  );
  renderChips();

  const sortItems = () => {
    items.sort((a, b) => {
      const mul = sortAsc ? 1 : -1;
      if (sortKey === 'name') return a.name.localeCompare(b.name) * mul;
      if (sortKey === 'sizeBytes') return (a.sizeBytes - b.sizeBytes) * mul;
      return (a.updatedAt - b.updatedAt) * mul;
    });
  };

  const openPreview = async (file: CatalogFile) => {
    try {
      const data = await apiRequest<{ file: CatalogFile; previewable: boolean; rawUrl?: string; textSnippet?: string; reason?: string }>(
        `/api/v1/catalog/preview?id=${file.id}`,
      );
      const wrap = el('div', { class: 'stack' });
      wrap.append(
        el('p', {}, `${data.file.relPath} · ${formatBytes(data.file.sizeBytes)} · SHA-256: ${data.file.sha256 || 'pending'}`),
      );
      if (data.previewable && data.rawUrl) {
        if (data.file.mimeType.startsWith('image/')) {
          wrap.append(el('img', { src: data.rawUrl, alt: data.file.name, style: 'max-width:100%;max-height:60vh;border-radius:8px;' }));
        } else if (data.file.mimeType.startsWith('video/')) {
          const vid = el('video', { src: data.rawUrl, controls: '', style: 'max-width:100%;max-height:60vh;' });
          wrap.append(vid);
        } else if (data.textSnippet) {
          wrap.append(el('pre', { class: 'code-log' }, data.textSnippet));
        } else {
          wrap.append(el('p', {}, 'Binary file staged locally. Ready for transfer.'));
        }
      } else {
        wrap.append(el('p', { class: 'vcell-sub' }, data.reason ?? 'Cloud metadata entry.'));
      }
      showQuickLookModal(`Quick-Look: ${file.name}`, wrap);
    } catch (err) {
      showToast(`Preview failed: ${(err as Error).message}`);
    }
  };

  const requestSoftDelete = async (file: CatalogFile) => {
    const ok = await confirmAction(
      'Move to 30-Day Quarantine?',
      `Move "${file.name}" (${file.relPath}) to the 30-day soft-delete quarantine? You can restore it anytime from Backplane -> Tombstones.`,
      'Quarantine',
    );
    if (!ok) {
      return;
    }
    try {
      const res = await apiRequest<{ tombstone: { id: number } }>('/api/v1/catalog/delete', {
        method: 'POST',
        body: JSON.stringify({ id: file.id }),
      });
      showToast(`Moved "${file.name}" to 30-day quarantine.`, {
        label: 'Undo',
        onClick: () => {
          void apiRequest('/api/v1/tombstones/resolve', {
            method: 'POST',
            body: JSON.stringify({ id: res.tombstone.id, resolution: 'RESTORED' }),
          }).then(() => fetchCatalog());
        },
      });
      await fetchCatalog();
    } catch (err) {
      showToast(`Quarantine failed: ${(err as Error).message}`);
    }
  };

  const renderTable = () => {
    if (unmountVirtual) {
      unmountVirtual();
      unmountVirtual = null;
    }
    listCard.replaceChildren();
    activeCountLabel.textContent = `${total.toLocaleString()} items (${queryMs.toFixed(1)} ms)`;

    if (loading) {
      listCard.append(
        el('div', { class: 'skeleton-line', style: 'width: 75%' }),
        el('div', { class: 'skeleton-line', style: 'width: 90%' }),
        el('div', { class: 'skeleton-line', style: 'width: 60%' }),
      );
      return;
    }

    if (errorMsg) {
      const retry = el('button', { type: 'button', class: 'btn-primary btn-sm' }, 'Retry');
      retry.addEventListener('click', () => void fetchCatalog());
      listCard.append(el('div', { class: 'error-state' }, el('p', {}, errorMsg), retry));
      return;
    }

    if (items.length === 0) {
      const clearBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, 'Clear Search');
      clearBtn.addEventListener('click', () => {
        providerFilter = 'all';
        searchQuery = '';
        searchInput.value = '';
        renderChips();
        void fetchCatalog();
      });
      listCard.append(
        el(
          'div',
          { class: 'empty-state' },
          iconEl('folder', 28),
          el('p', {}, 'No catalog files match the active search or pool filter.'),
          clearBtn,
        ),
      );
      return;
    }

    sortItems();

    const viewport = el('div', { class: 'virtual-viewport', tabindex: '0', role: 'region', 'aria-label': 'Virtualized file catalog' });
    const head = el('div', { class: 'vtable-head' });

    const makeSortBtn = (label: string, key: 'name' | 'sizeBytes' | 'updatedAt') => {
      const indicator = sortKey === key ? (sortAsc ? ' ↑' : ' ↓') : '';
      const b = el('button', { type: 'button' }, `${label}${indicator}`);
      b.addEventListener('click', () => {
        if (sortKey === key) sortAsc = !sortAsc;
        else {
          sortKey = key;
          sortAsc = true;
        }
        renderTable();
      });
      return b;
    };

    head.append(
      makeSortBtn('File / Path', 'name'),
      el('div', { class: 'vcell' }, 'Pool'),
      makeSortBtn('Size', 'sizeBytes'),
      el('div', { class: 'vcell' }, 'Sync State'),
      el('div', { class: 'vcell' }, 'Actions'),
    );

    const body = el('div', { class: 'vtable-body' });
    viewport.append(head, body);
    listCard.append(viewport);

    const rowHeight = getSavedDensity() === 'compact' ? 42 : 56;
    const safeObserveRect = (instance: Virtualizer<HTMLElement, HTMLElement>, cb: (rect: Rect) => void) => {
      if (typeof ResizeObserver === 'undefined') {
        cb({ width: viewport.clientWidth || 800, height: viewport.clientHeight || 420 });
        return () => {};
      }
      return observeElementRect(instance, cb);
    };

    const renderRows = (virtualizer: Virtualizer<HTMLElement, HTMLElement>) => {
      body.style.height = `${virtualizer.getTotalSize()}px`;
      const vItems = virtualizer.getVirtualItems();
      const nodes: HTMLElement[] = [];

      for (const vItem of vItems) {
        const file = items[vItem.index];
        if (!file) continue;
        const row = el('div', {
          class: 'vtable-row',
          tabindex: '0',
          'data-index': String(vItem.index),
          'data-selected': String(selectedId === file.id),
          style: `height:${vItem.size}px;transform:translateY(${vItem.start}px);`,
        });
        row.addEventListener('click', () => {
          selectedId = file.id;
          renderRows(virtualizer);
        });
        row.addEventListener('keydown', (ev) => {
          if (ev.key === ' ') {
            ev.preventDefault();
            void openPreview(file);
          } else if (ev.key === 'Enter') {
            selectedId = file.id;
            void openPreview(file);
          }
        });

        const badge =
          file.status === 'SYNCED'
            ? statusBadge('success', 'check', 'Synced')
            : file.status === 'STAGED'
              ? statusBadge('info', 'activity', 'Staged')
              : statusBadge('warning', 'warn', file.status);

        const previewBtn = el('button', { type: 'button', class: 'btn-ghost btn-sm', 'aria-label': `Preview ${file.name}` }, iconEl('eye', 14));
        previewBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          void openPreview(file);
        });
        const delBtn = el('button', { type: 'button', class: 'btn-ghost btn-sm', 'aria-label': `Quarantine ${file.name}` }, iconEl('trash', 14));
        delBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          void requestSoftDelete(file);
        });

        row.append(
          el('div', { class: 'vcell' }, el('div', {}, file.name), el('div', { class: 'vcell-sub' }, file.relPath)),
          el('div', { class: 'vcell' }, `${file.provider} (${file.remoteId})`),
          el('div', { class: 'vcell' }, formatBytes(file.sizeBytes)),
          el('div', { class: 'vcell' }, badge),
          el('div', { class: 'vcell' }, previewBtn, delBtn),
        );
        nodes.push(row);
      }
      body.replaceChildren(...nodes);
    };

    const virtualizer = new Virtualizer<HTMLElement, HTMLElement>({
      count: items.length,
      getScrollElement: () => viewport,
      estimateSize: () => rowHeight,
      overscan: 6,
      scrollToFn: elementScroll,
      observeElementRect: safeObserveRect,
      observeElementOffset,
      onChange: (inst) => renderRows(inst),
    });

    const cleanupMount = virtualizer._didMount();
    virtualizer._willUpdate();
    renderRows(virtualizer);
    unmountVirtual = cleanupMount;
  };

  const fetchCatalog = async () => {
    loading = true;
    errorMsg = '';
    renderTable();
    try {
      const params = new URLSearchParams({
        q: searchQuery,
        provider: providerFilter,
        limit: '500',
      });
      const res = await apiRequest<{ items: CatalogFile[]; total: number; queryMs: number }>(`/api/v1/catalog?${params.toString()}`);
      items = res.items ?? [];
      total = res.total ?? 0;
      queryMs = res.queryMs ?? 0;
    } catch (err) {
      errorMsg = `Failed to query catalog: ${(err as Error).message}`;
    } finally {
      loading = false;
      renderTable();
    }
  };

  const onSpacePreview = (ev: KeyboardEvent) => {
    if (ev.key === ' ' && (ev.target as HTMLElement)?.tagName !== 'INPUT' && (ev.target as HTMLElement)?.tagName !== 'BUTTON') {
      const chosen = items.find((i) => i.id === selectedId) ?? items[0];
      if (chosen) {
        ev.preventDefault();
        void openPreview(chosen);
      }
    }
  };
  document.addEventListener('keydown', onSpacePreview);
  void fetchCatalog();

  return () => {
    window.clearTimeout(searchTimer);
    window.removeEventListener('cm-telemetry', onTelemetry);
    document.removeEventListener('keydown', onSpacePreview);
    if (unmountVirtual) unmountVirtual();
  };
}
