interface ISettings {
  signup: boolean;
  createUserDir?: boolean; // Not editable in UI, kept for backward compatibility
  hideLoginButton: boolean;
  minimumPasswordLength: number;
  defaults: SettingsDefaults;
  branding: SettingsBranding;
  tus: SettingsTus;
  storageType?: string;
  gcInterval?: string;
  availableStorageTypes?: StorageTypeInfo[];
}

interface StorageTypeInfo {
  name: string;
  displayName: string;
  description: string;
}

interface GCHistory {
  time: string;
  duration: string;
  deletedSize: number;
  deletedCount: number;
  missingCount: number;
  status: "done" | "failed" | "running";
  error?: string;
  dryRun: boolean;
}

interface MigrationStatus {
  running: boolean;
  sourceType: string;
  targetType: string;
  total: number;
  migrated: number;
  failed: number;
  startedAt: string;
  finishedAt?: string;
  status: "idle" | "running" | "done" | "done_with_errors" | "failed";
  error?: string;
}

interface PreflightResult {
  ok: boolean;
  missingFiles?: { userId: number; filePath: string; reason: string }[];
  pluginErrors?: string[];
  totalFiles: number;
}

interface SettingsDefaults {
  scope: string;
  locale: string;
  viewMode: ViewModeType;
  singleClick: boolean;
  sorting: Sorting;
  perm: Permissions;
  dateFormat: boolean;
  aceEditorTheme: string;
}

interface SettingsBranding {
  name: string;
  disableExternal: boolean;
  disableUsedPercentage: boolean;
  files: string;
  theme: UserTheme;
  color: string;
}

interface SettingsTus {
  chunkSize: number;
  retryCount: number;
}
