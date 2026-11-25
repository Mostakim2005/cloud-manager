import type { TelemetrySnapshot } from './types';

function readSessionToken(): string {
  const meta = document.querySelector('meta[name="cm-session"]');
  return meta?.getAttribute('content') ?? '';
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const tok = readSessionToken();
  if (tok) {
    headers.set('Authorization', `Bearer ${tok}`);
  }
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const resp = await fetch(path, { ...init, headers });
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const payload = (await resp.json()) as { error?: string };
      if (payload.error) {
        msg = payload.error;
      }
    } catch {
      // Fallback to HTTP status.
    }
    throw new Error(msg);
  }
  return (await resp.json()) as T;
}

export function subscribeTelemetry(
  onSnapshot: (snap: TelemetrySnapshot) => void,
  onStatus: (online: boolean) => void,
): () => void {
  const ctrl = new AbortController();
  let stopped = false;

  const connect = async () => {
    while (!stopped) {
      try {
        const headers: Record<string, string> = {};
        const tok = readSessionToken();
        if (tok) {
          headers['Authorization'] = `Bearer ${tok}`;
        }
        const resp = await fetch('/api/v1/events', { headers, signal: ctrl.signal });
        if (!resp.ok || !resp.body) {
          onStatus(false);
          await sleep(2500);
          continue;
        }
        onStatus(true);
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (!stopped) {
          const { done, value } = await reader.read();
          if (done) {
            break;
          }
          buffer += decoder.decode(value, { stream: true });
          const frames = buffer.split('\n\n');
          buffer = frames.pop() ?? '';
          for (const frame of frames) {
            const dataLine = frame.split('\n').find((l) => l.startsWith('data: '));
            if (!dataLine) {
              continue;
            }
            try {
              onSnapshot(JSON.parse(dataLine.slice(6)) as TelemetrySnapshot);
            } catch {
              // Skip malformed partial frame.
            }
          }
        }
      } catch {
        if (!stopped) {
          onStatus(false);
          await sleep(2500);
        }
      }
    }
  };

  void connect();
  return () => {
    stopped = true;
    ctrl.abort();
  };
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
