import { apiRequest } from '../api';
import { iconEl } from '../icons';
import type { RoutingRule, SimulationResult } from '../types';
import { confirmAction, el, formatBytes, showToast, statusBadge } from '../ui';

export function renderRulesView(container: HTMLElement): () => void {
  const simCard = el('div', { class: 'card' });
  const rulesCard = el('div', { class: 'card' });
  const grid = el('div', { class: 'grid-2' }, rulesCard, simCard);
  container.replaceChildren(grid);

  let showAdvanced = false;
  let rules: RoutingRule[] = [];

  // Rule Dry-Run Simulator (supports drag-and-drop AND accessible file picker / manual fields)
  const simResultBox = el('div', { class: 'code-log', id: 'sim-output' }, 'Drop a file or enter metadata to preview exact rule & spillover.');
  const fileNameIn = el('input', { id: 'sim-name', type: 'text', class: 'input', value: 'lecture_clip.mp4', placeholder: 'e.g. textbook.pdf' }) as HTMLInputElement;
  const originIn = el('select', { id: 'sim-origin', class: 'select' }) as HTMLSelectElement;
  for (const orig of ['pc', 'recordings', 'phone', 'app']) {
    originIn.append(el('option', { value: orig }, orig));
  }
  const sizeIn = el('input', { id: 'sim-size', type: 'number', class: 'input', value: '104857600', min: '0' }) as HTMLInputElement;
  const runSimBtn = el('button', { type: 'button', class: 'btn-primary btn-sm' }, iconEl('zap', 14), 'Simulate Route');

  const runSimulation = async (fileName: string, originType: string, sizeBytes: number) => {
    try {
      const res = await apiRequest<SimulationResult>('/api/v1/rules/simulate', {
        method: 'POST',
        body: JSON.stringify({ fileName, originType, sizeBytes }),
      });
      simResultBox.textContent = [
        `File:       ${res.fileName} (${formatBytes(res.sizeBytes)}, origin=${res.originType})`,
        `Rule Hit:   #${res.matchedRuleId} ${res.matchedRuleName}`,
        `Target:     ${res.targetProvider} -> ${res.targetRemote}${res.targetPlaylist ? ` (playlist ${res.targetPlaylist})` : ''}`,
        `Spillover:  ${res.spilloverApplied ? `YES (${res.spilloverReason})` : 'No spillover needed'}`,
        `BW Window:  ${res.bandwidthCapKbps === 0 ? 'Unlimited (Night)' : `${res.bandwidthCapKbps} KB/s`}`,
      ].join('\n');
    } catch (err) {
      simResultBox.textContent = `Simulation error: ${(err as Error).message}`;
    }
  };

  runSimBtn.addEventListener('click', () => {
    void runSimulation(fileNameIn.value.trim() || 'sample.bin', originIn.value, Number(sizeIn.value) || 0);
  });

  const filePicker = el('input', { id: 'sim-file-picker', type: 'file', class: 'visually-hidden' }) as HTMLInputElement;
  const pickBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, 'Choose Local File');
  pickBtn.addEventListener('click', () => filePicker.click());
  filePicker.addEventListener('change', () => {
    const f = filePicker.files?.[0];
    if (f) {
      fileNameIn.value = f.name;
      sizeIn.value = String(f.size);
      void runSimulation(f.name, originIn.value, f.size);
    }
  });

  const dropZone = el(
    'div',
    { class: 'drop-zone', 'aria-label': 'Rule Dry-Run drop area' },
    el('strong', {}, 'Rule Dry-Run Simulator'),
    el('span', { class: 'vcell-sub' }, 'Drag & drop any file here (or use Choose Local File) to simulate priority ASC & 500 MiB spillover.'),
    filePicker,
    pickBtn,
  );
  dropZone.addEventListener('dragover', (e) => {
    e.preventDefault();
    dropZone.classList.add('drag-over');
  });
  dropZone.addEventListener('dragleave', () => dropZone.classList.remove('drag-over'));
  dropZone.addEventListener('drop', (e) => {
    e.preventDefault();
    dropZone.classList.remove('drag-over');
    const f = e.dataTransfer?.files?.[0];
    if (f) {
      fileNameIn.value = f.name;
      sizeIn.value = String(f.size);
      void runSimulation(f.name, originIn.value, f.size);
    }
  });

  simCard.append(
    el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Rule Dry-Run & Spillover Bench')),
    dropZone,
    el(
      'div',
      { class: 'toolbar', style: 'margin-top:12px;' },
      el('label', { for: 'sim-name' }, 'Name'),
      fileNameIn,
      el('label', { for: 'sim-origin' }, 'Origin'),
      originIn,
      el('label', { for: 'sim-size' }, 'Bytes'),
      sizeIn,
      runSimBtn,
    ),
    simResultBox,
  );

  // Priority Rules List + Progressive Disclosure Add Form
  const advToggle = el('button', { type: 'button', class: 'btn-secondary btn-sm', 'aria-expanded': 'false' }, 'Show Add Rule');
  const addForm = el('form', { class: 'toolbar', hidden: '' });
  const rName = el('input', { type: 'text', class: 'input', placeholder: 'Rule name', required: '' }) as HTMLInputElement;
  const rPrio = el('input', { type: 'number', class: 'input', value: '15', min: '1', max: '998', style: 'width:80px' }) as HTMLInputElement;
  const rGlob = el('input', { type: 'text', class: 'input', value: '*.pdf;*.epub', placeholder: '*.ext' }) as HTMLInputElement;
  const rTarget = el('select', { class: 'select' }) as HTMLSelectElement;
  rTarget.append(el('option', { value: 'gdrive|gdrive-1' }, 'GDrive #1'), el('option', { value: 'mega|mega-1' }, 'Mega #1'), el('option', { value: 'youtube|youtube-private' }, 'YouTube Private'));
  const rAddBtn = el('button', { type: 'submit', class: 'btn-primary btn-sm' }, 'Add Rule');
  addForm.append(rName, rPrio, rGlob, rTarget, rAddBtn);

  advToggle.addEventListener('click', () => {
    showAdvanced = !showAdvanced;
    addForm.hidden = !showAdvanced;
    advToggle.textContent = showAdvanced ? 'Hide Add Rule' : 'Show Add Rule';
    advToggle.setAttribute('aria-expanded', String(showAdvanced));
  });

  addForm.addEventListener('submit', (e) => {
    e.preventDefault();
    const [prov, rem] = rTarget.value.split('|');
    void apiRequest('/api/v1/rules', {
      method: 'POST',
      body: JSON.stringify({
        name: rName.value.trim(),
        priority: Number(rPrio.value) || 50,
        originMatch: '*',
        extGlob: rGlob.value.trim() || '*',
        targetProvider: prov,
        targetRemote: rem,
      }),
    })
      .then(() => {
        showToast('Routing rule created.');
        rName.value = '';
        return loadRules();
      })
      .catch((err: Error) => showToast(`Failed: ${err.message}`));
  });

  const tableWrap = el('div', {});
  rulesCard.append(
    el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Priority Routing Rules (First-Match-Wins)'), advToggle),
    addForm,
    tableWrap,
  );

  const loadRules = async () => {
    try {
      const res = await apiRequest<{ rules: RoutingRule[] }>('/api/v1/rules');
      rules = res.rules ?? [];
      const tbl = el('table', { class: 'data-table' });
      tbl.append(
        el('caption', { class: 'visually-hidden' }, 'Priority-ordered smart routing rules'),
        el('thead', {}, el('tr', {}, el('th', { scope: 'col' }, 'Priority'), el('th', { scope: 'col' }, 'Rule'), el('th', { scope: 'col' }, 'Match'), el('th', { scope: 'col' }, 'Target'), el('th', { scope: 'col' }, ''))),
      );
      const tbody = el('tbody', {});
      for (const r of rules) {
        const del = el('button', { type: 'button', class: 'btn-ghost btn-sm', 'aria-label': `Delete ${r.name}` }, iconEl('trash', 14));
        if (r.isFallback) del.disabled = true;
        del.addEventListener('click', async () => {
          if (await confirmAction('Delete Routing Rule?', `Delete "${r.name}"?`, 'Delete')) {
            await apiRequest(`/api/v1/rules?id=${r.id}`, { method: 'DELETE' });
            showToast('Rule deleted.');
            await loadRules();
          }
        });
        tbody.append(
          el(
            'tr',
            {},
            el('td', {}, String(r.priority)),
            el('td', {}, r.name),
            el('td', { class: 'vcell-sub' }, `${r.originMatch} / ${r.extGlob}`),
            el('td', {}, statusBadge('info', 'cloud', `${r.targetProvider} -> ${r.targetRemote}`)),
            el('td', {}, del),
          ),
        );
      }
      tbl.append(tbody);
      tableWrap.replaceChildren(tbl);
    } catch (err) {
      tableWrap.replaceChildren(el('p', { class: 'error-state' }, (err as Error).message));
    }
  };

  void loadRules();
  return () => {};
}
