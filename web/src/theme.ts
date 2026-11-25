import type { DensityMode, ThemeMode } from './types';

const THEME_KEY = 'cm-color-scheme';
const DENSITY_KEY = 'cm-density';

export function getSavedTheme(): ThemeMode {
  try {
    const val = localStorage.getItem(THEME_KEY);
    if (val === 'light' || val === 'dark') {
      return val;
    }
  } catch {
    // Ignore storage restriction.
  }
  return 'system';
}

export function getSystemResolvedScheme(): 'light' | 'dark' {
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return 'light';
}

export function applyTheme(mode: ThemeMode, persist = true): void {
  const root = document.documentElement;
  root.classList.add('theme-switching');

  const meta = document.querySelector('meta[name="color-scheme"]');
  if (meta) {
    meta.setAttribute('content', mode === 'system' ? 'light dark' : mode);
  }

  if (persist) {
    try {
      if (mode === 'system') {
        localStorage.removeItem(THEME_KEY);
      } else {
        localStorage.setItem(THEME_KEY, mode);
      }
    } catch {
      // Ignore storage restriction.
    }
  }

  if (typeof requestAnimationFrame === 'function') {
    requestAnimationFrame(() => root.classList.remove('theme-switching'));
  } else {
    root.classList.remove('theme-switching');
  }
}

export function getSavedDensity(): DensityMode {
  try {
    return localStorage.getItem(DENSITY_KEY) === 'compact' ? 'compact' : 'comfortable';
  } catch {
    return 'comfortable';
  }
}

export function applyDensity(mode: DensityMode, persist = true): void {
  document.documentElement.setAttribute('data-density', mode);
  if (persist) {
    try {
      localStorage.setItem(DENSITY_KEY, mode);
    } catch {
      // Ignore storage restriction.
    }
  }
}

export function initThemeListeners(onExternalChange: () => void): () => void {
  const onStorage = (ev: StorageEvent) => {
    if (ev.key === THEME_KEY) {
      applyTheme(getSavedTheme(), false);
      onExternalChange();
    } else if (ev.key === DENSITY_KEY) {
      applyDensity(getSavedDensity(), false);
      onExternalChange();
    }
  };
  window.addEventListener('storage', onStorage);

  let mq: MediaQueryList | null = null;
  const onMedia = () => onExternalChange();
  if (typeof window.matchMedia === 'function') {
    mq = window.matchMedia('(prefers-color-scheme: dark)');
    mq.addEventListener?.('change', onMedia);
  }

  return () => {
    window.removeEventListener('storage', onStorage);
    mq?.removeEventListener?.('change', onMedia);
  };
}
