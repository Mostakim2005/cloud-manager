import { apiRequest } from '../api';
import { iconEl } from '../icons';
import { applyDensity, applyTheme, getSavedDensity, getSavedTheme, getSystemResolvedScheme } from '../theme';
import type { DensityMode, Settings, ThemeMode } from '../types';
import { confirmAction, el, showToast } from '../ui';

export function renderSettingsView(container: HTMLElement, onThemeOrDensitySync: () => void): () => void {
  const card = el('div', { class: 'card' });
  container.replaceChildren(card);

  const load = async () => {
    try {
      const s = await apiRequest<Settings>('/api/v1/settings');
      card.replaceChildren();

      const currentTheme = getSavedTheme();
      const sysMode = getSystemResolvedScheme();
      const themeSelect = el('select', { id: 'set-theme', class: 'select' }) as HTMLSelectElement;
      themeSelect.append(
        el('option', { value: 'system' }, `System (Currently ${sysMode})`),
        el('option', { value: 'light' }, 'Light'),
        el('option', { value: 'dark' }, 'Dark'),
      );
      themeSelect.value = currentTheme;
      themeSelect.addEventListener('change', () => {
        const next = themeSelect.value as ThemeMode;
        applyTheme(next);
        onThemeOrDensitySync();
        showToast(`Theme set to ${next}.`);
      });

      const densitySelect = el('select', { id: 'set-density', class: 'select' }) as HTMLSelectElement;
      densitySelect.append(el('option', { value: 'comfortable' }, 'Comfortable'), el('option', { value: 'compact' }, 'Compact (-25% padding)'));
      densitySelect.value = getSavedDensity();
      densitySelect.addEventListener('change', () => {
        const next = densitySelect.value as DensityMode;
        applyDensity(next);
        onThemeOrDensitySync();
        showToast(`Density set to ${next}.`);
      });

      const stagingIn = el('input', { id: 'set-staging', type: 'text', class: 'input', value: s.stagingDir }) as HTMLInputElement;
      const bwDayIn = el('input', { id: 'set-bw-day', type: 'number', class: 'input', value: String(s.bandwidthDayKbps), min: '0' }) as HTMLInputElement;
      const ragIn = el('input', { id: 'set-rag', type: 'url', class: 'input', value: s.ragManagerUrl }) as HTMLInputElement;

      const saveBtn = el('button', { type: 'button', class: 'btn-primary btn-sm' }, 'Save Preferences');
      saveBtn.addEventListener('click', async () => {
        await apiRequest('/api/v1/settings', {
          method: 'PUT',
          body: JSON.stringify({
            ...s,
            theme: themeSelect.value,
            density: densitySelect.value,
            stagingDir: stagingIn.value.trim(),
            bandwidthDayKbps: Number(bwDayIn.value) || 0,
            ragManagerUrl: ragIn.value.trim(),
          }),
        });
        showToast('Preferences saved atomically (UTF-8 without BOM).');
      });

      let advVisible = false;
      const advToggle = el('button', { type: 'button', class: 'btn-secondary btn-sm', 'aria-expanded': 'false' }, 'Show Advanced & Diagnostics');
      const advPanel = el('div', { hidden: '', style: 'margin-top:12px;' });
      const diagBox = el('pre', { class: 'code-log' }, 'Click "Copy Diagnostic Bundle" to inspect redacted environment status.');

      const copyDiagBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, iconEl('copy', 14), 'Copy Diagnostic Bundle');
      copyDiagBtn.addEventListener('click', async () => {
        const rep = await apiRequest<any>('/api/v1/doctor');
        const txt = JSON.stringify(rep, null, 2);
        diagBox.textContent = txt;
        await navigator.clipboard?.writeText(txt).catch(() => {});
        showToast('Redacted diagnostic bundle loaded (copied if clipboard permitted).');
      });

      const exportBtn = el('button', { type: 'button', class: 'btn-secondary btn-sm' }, 'Export Redacted Config JSON');
      exportBtn.addEventListener('click', async () => {
        const data = await apiRequest<any>('/api/v1/settings/export');
        const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
        const a = el('a', { href: URL.createObjectURL(blob), download: 'cloud-manager-config-redacted.json' });
        a.click();
      });

      const resetBtn = el('button', { type: 'button', class: 'btn-danger btn-sm' }, 'Reset to Defaults');
      resetBtn.addEventListener('click', async () => {
        if (await confirmAction('Reset All Settings?', 'Restore Cloud Manager settings to clean offline-first defaults?', 'Reset')) {
          await apiRequest('/api/v1/settings/reset', { method: 'POST' });
          applyTheme('system');
          applyDensity('comfortable');
          onThemeOrDensitySync();
          showToast('Settings reset to defaults.');
          await load();
        }
      });

      advPanel.append(el('div', { class: 'toolbar' }, copyDiagBtn, exportBtn, resetBtn), diagBox);
      advToggle.addEventListener('click', () => {
        advVisible = !advVisible;
        advPanel.hidden = !advVisible;
        advToggle.textContent = advVisible ? 'Hide Advanced & Diagnostics' : 'Show Advanced & Diagnostics';
        advToggle.setAttribute('aria-expanded', String(advVisible));
      });

      card.append(
        el('div', { class: 'card-header' }, el('h2', { class: 'card-title' }, 'Appearance, Storage & Diagnostics'), saveBtn),
        el(
          'div',
          { class: 'account-stack' },
          el('div', { class: 'toolbar' }, el('label', { for: 'set-theme' }, 'Theme'), themeSelect, el('label', { for: 'set-density' }, 'Density'), densitySelect),
          el('div', { class: 'toolbar' }, el('label', { for: 'set-staging' }, 'Staging Directory'), stagingIn),
          el('div', { class: 'toolbar' }, el('label', { for: 'set-bw-day' }, 'Daytime Bandwidth Cap (KB/s)'), bwDayIn, el('label', { for: 'set-rag' }, 'RAG Manager Webhook'), ragIn),
        ),
        el('div', { style: 'margin-top:16px;' }, advToggle, advPanel),
      );
    } catch (err) {
      card.replaceChildren(el('div', { class: 'error-state' }, (err as Error).message));
    }
  };

  void load();
  return () => {};
}
