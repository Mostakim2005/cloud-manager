export type ViewId = 'explorer' | 'rules' | 'phone' | 'youtube' | 'backplane' | 'settings';
export type ThemeMode = 'system' | 'light' | 'dark';
export type DensityMode = 'comfortable' | 'compact';

export interface CatalogFile {
  id: number;
  relPath: string;
  name: string;
  localPath?: string;
  sizeBytes: number;
  sha256: string;
  provider: string;
  remoteId: string;
  originType: string;
  mimeType: string;
  mtimeUtc: number;
  parentRevision: string;
  status: string;
  updatedAt: number;
}

export interface Account {
  id: string;
  name: string;
  provider: string;
  rcloneRemote: string;
  hasherRemote: string;
  totalBytes: number;
  usedBytes: number;
  freeBytes: number;
  health: string;
  cooldownUntil: number;
}

export interface RoutingRule {
  id: number;
  priority: number;
  name: string;
  originMatch: string;
  extGlob: string;
  minSizeBytes: number;
  maxSizeBytes: number;
  targetProvider: string;
  targetRemote: string;
  targetPlaylist: string;
  spilloverPool: string;
  stopOnMatch: boolean;
  enabled: boolean;
  isFallback: boolean;
}

export interface SimulationResult {
  fileName: string;
  originType: string;
  sizeBytes: number;
  matchedRuleId: number;
  matchedRuleName: string;
  targetProvider: string;
  targetRemote: string;
  targetPlaylist?: string;
  spilloverApplied: boolean;
  spilloverReason?: string;
  bandwidthCapKbps: number;
}

export interface Job {
  id: number;
  kind: string;
  sourcePath: string;
  targetRemote: string;
  targetPlaylist?: string;
  status: string;
  priority: number;
  bytesTotal: number;
  bytesDone: number;
  lastError?: string;
  resumableUri?: string;
  updatedAt: number;
}

export interface PhoneSyncFolder {
  id: string;
  transport: string;
  label: string;
  localPath: string;
  remotePath: string;
  autoSync: boolean;
  ragNotify: boolean;
  lastSyncAt: number;
  status: string;
}

export interface LANQRStatus {
  enabled: boolean;
  url?: string;
  pin?: string;
  port: number;
  autoRoute: boolean;
  qrSvg?: string;
}

export interface YouTubeQuota {
  ptDate: string;
  unitsUsed: number;
  dailyCap: number;
  dailyCeiling: number;
  remaining: number;
  nextResetPt: string;
  canUploadNext: boolean;
}

export interface DuplicateGroup {
  sha256: string;
  count: number;
  wastedBytes: number;
  entries: CatalogFile[];
}

export interface Tombstone {
  id: number;
  fileId: number;
  remoteId: string;
  remotePath: string;
  detectedAt: number;
  resolution: string;
  purgeAfter: number;
}

export interface AppSnapshot {
  id: number;
  appSlug: string;
  snapshotType: string;
  kopiaSnapshotId?: string;
  sha256: string;
  relPath: string;
  conflictPath?: string;
  createdAt: number;
}

export interface Settings {
  schemaVersion: number;
  theme: ThemeMode;
  density: DensityMode;
  stagingDir: string;
  lanPort: number;
  lanEnabledDefault: boolean;
  autoRouteLan: boolean;
  bandwidthDayKbps: number;
  bandwidthNightKbps: number;
  bandwidthDayStartHour: number;
  bandwidthDayEndHour: number;
  ragManagerUrl: string;
  youtubePlaylistId: string;
  showAdvancedSettings: boolean;
}

export interface TelemetryEvent {
  ts: string;
  kind: string;
  component: string;
  message: string;
}

export interface TransferTick {
  jobId: number;
  label: string;
  bytesDone: number;
  bytesTotal: number;
  speedBps: number;
  percent: number;
  status: string;
}

export interface TelemetrySnapshot {
  ts: string;
  totalSpeedBps: number;
  activeJobs: TransferTick[];
  recentEvents: TelemetryEvent[];
  adapterStates: Record<string, boolean>;
}
