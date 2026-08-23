interface IUser {
  id: number;
  email: string;
  username?: string;
  password: string;
  scope: string;
  locale: string;
  perm: Permissions;
  lockPassword: boolean;
  singleClick: boolean;
  dateFormat: boolean;
  viewMode: ViewModeType;
  storageQuota?: string; // Formatted storage quota like "10M", "5G"
  sorting?: Sorting;
  theme: UserTheme;
  showHidden: boolean;
  passkeyEnabled?: boolean;
  hasPasskey?: boolean;
}

interface IPasskeyCredential {
  id: string;
  name: string;
  createdAt: number;
  lastUsedAt: number;
  enabled: boolean;
  credential?: any;
}

interface IPasskeyListResponse {
  credentials: IPasskeyCredential[];
  enabled: boolean;
}

type ViewModeType = "list" | "mosaic" | "mosaic gallery";

interface IUserForm {
  id?: number;
  email?: string;
  username?: string;
  password?: string;
  scope?: string;
  locale?: string;
  perm?: Permissions;
  lockPassword?: boolean;
  singleClick?: boolean;
  dateFormat?: boolean;
  storageQuota?: string | number;
  theme?: UserTheme;
  showHidden?: boolean;
}

interface Permissions {
  admin: boolean;
  copy: boolean;
  create: boolean;
  delete: boolean;
  download: boolean;
  execute: boolean;
  modify: boolean;
  move: boolean;
  rename: boolean;
  share: boolean;
  shell: boolean;
  upload: boolean;
}

interface Sorting {
  by: string;
  asc: boolean;
}

interface IRule {
  allow: boolean;
  path: string;
  regex: boolean;
  regexp: IRegexp;
}

interface IRegexp {
  raw: string;
}

type UserTheme = "auto" | "light" | "dark" | "";

interface IOtpSetupKey {
  setupKey: string;
}
