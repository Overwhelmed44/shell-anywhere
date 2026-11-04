import type { LocalShell } from "./types";

export type LocalShellOption = { id: LocalShell; name: string };

export type Host = {
  id: string;
  name: string;
  address: string;
  port: number;
  username: string;
  authMethod: "password" | "key";
  keyPath: string;
  groupId: string;
  identityId: string;
  useGroupIdentity: boolean;
  workspaceId: string;
  favorite: boolean;
  savedPassword: boolean;
};

export type Group = { id: string; name: string; defaultIdentityId: string };

export type Identity = {
  id: string;
  name: string;
  username: string;
  authMethod: "password" | "key";
  keyPath: string;
  savedPassword: boolean;
};

export type WorkspaceView = {
  id: string;
  name: string;
  hosts: Host[];
  groups: Group[];
  identities: Identity[];
};

export type ConnectResult = {
  status: "connected" | "host_key_unknown" | "passphrase_required";
  sessionId?: string;
  fingerprint?: string;
  keyType?: string;
};

export type FileEntry = {
  name: string;
  path: string;
  directory: boolean;
  symlink: boolean;
  size: number;
  modified: number;
};

export type FileListing = {
  path: string;
  parent: string;
  entries: FileEntry[];
};

export type FileTransferProgress = {
  sessionId: string;
  id: string;
  direction: "upload" | "download" | "copy";
  phase: string;
  current: string;
  copied: number;
  total: number;
  files: number;
  completed: number;
};

export type FileTransferResult = { status: string; conflicts: string[] };

export type AccountStatus = {
  user_id: string;
  username: string;
  pending: boolean;
};

export type AccountAction = "login" | "cancel" | "logout";

export type UpdateStatus = {
  currentVersion: string;
  latestVersion: string;
  state:
    | "checking"
    | "up_to_date"
    | "update_available"
    | "no_release"
    | "unavailable";
};

export type VaultStatus = {
  enabled: boolean;
  protected: boolean;
  locked: boolean;
  busy: boolean;
  needsChoice: boolean;
  localHosts: number;
  cloudHosts: number;
  lastSynced: string;
  error: string;
};

type Backend = {
  ConnectSFTP(
    workspaceId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint: string,
  ): Promise<ConnectResult>;
  ReconnectSFTP(
    sessionId: string,
    workspaceId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint: string,
  ): Promise<ConnectResult>;
  ListLocalFiles(path: string): Promise<FileListing>;
  ListSFTPFiles(sessionId: string, path: string): Promise<FileListing>;
  ChooseLocalFolder(): Promise<string | null>;
  ChooseUploadFiles(): Promise<string[] | null>;
  CreateFileFolder(
    sessionId: string,
    side: string,
    parent: string,
    name: string,
  ): Promise<void>;
  TransferSFTP(
    sessionId: string,
    id: string,
    direction: string,
    sources: string[],
    target: string,
    overwrite: boolean,
    peerSessionId: string,
  ): Promise<FileTransferResult>;
  CancelSFTPTransfer(sessionId: string, id: string): Promise<void>;
  GetAppVersion(): Promise<string>;
  GetUpdateStatus(): Promise<UpdateStatus>;
  GetAccount(): Promise<AccountStatus>;
  BeginLogin(): Promise<void>;
  CancelLogin(): Promise<void>;
  Logout(): Promise<void>;
  GetVaultStatus(): Promise<VaultStatus>;
  ConfigureVaultSync(password: string, direction: string): Promise<VaultStatus>;
  SyncVault(): Promise<VaultStatus>;
  PauseVaultSync(): Promise<VaultStatus>;
  ListHosts(): Promise<Host[]>;
  GetWorkspace(): Promise<WorkspaceView>;
  ListGroups(): Promise<Group[]>;
  ListIdentities(): Promise<Identity[]>;
  CreateGroup(name: string): Promise<Group>;
  SaveGroup(group: Group): Promise<Group>;
  SaveIdentity(
    identity: Identity,
    password: string,
    passphrase: string,
  ): Promise<Identity>;
  DeleteIdentity(id: string): Promise<void>;
  ConnectWorkspaceHost(
    workspaceId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint: string,
  ): Promise<ConnectResult>;
  ReconnectWorkspaceHost(
    sessionId: string,
    workspaceId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint: string,
  ): Promise<ConnectResult>;
  DeleteGroup(name: string): Promise<void>;
  SaveHost(host: Host, password: string): Promise<Host>;
  DeleteHost(id: string): Promise<void>;
  ChooseKey(): Promise<string>;
  ConnectHost(
    id: string,
    password: string,
    passphrase: string,
    fingerprint: string,
  ): Promise<ConnectResult>;
  ReconnectHost(
    sessionId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint: string,
  ): Promise<ConnectResult>;
  GetLocalShells(): Promise<LocalShellOption[]>;
  StartLocalShell(shell: LocalShell): Promise<string>;
  WriteSession(id: string, data: string): Promise<void>;
  ResizeSession(id: string, rows: number, columns: number): Promise<void>;
  DisconnectSession(id: string): Promise<void>;
};

type Runtime = {
  EventsOn(name: string, callback: (data: unknown) => void): () => void;
};

declare global {
  interface Window {
    go?: { main?: { App?: Backend } };
    runtime?: Runtime;
  }
}

export const isDesktop = Boolean(window.go?.main?.App);

function backend(): Backend {
  const app = window.go?.main?.App;
  if (!app) throw new Error("Open the desktop app to use SSH connections.");
  return app;
}

export const api = {
  connectSFTP: (
    workspaceId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint = "",
  ) =>
    backend().ConnectSFTP(
      workspaceId,
      hostId,
      password,
      passphrase,
      fingerprint,
    ),
  reconnectSFTP: (
    sessionId: string,
    workspaceId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint = "",
  ) =>
    backend().ReconnectSFTP(
      sessionId,
      workspaceId,
      hostId,
      password,
      passphrase,
      fingerprint,
    ),
  listLocalFiles: (path: string) => backend().ListLocalFiles(path),
  listSFTPFiles: (sessionId: string, path: string) =>
    backend().ListSFTPFiles(sessionId, path),
  chooseLocalFolder: async () => (await backend().ChooseLocalFolder()) ?? "",
  chooseUploadFiles: async () => (await backend().ChooseUploadFiles()) ?? [],
  createFileFolder: (
    sessionId: string,
    side: string,
    parent: string,
    name: string,
  ) => backend().CreateFileFolder(sessionId, side, parent, name),
  transferSFTP: (
    sessionId: string,
    id: string,
    direction: string,
    sources: string[],
    target: string,
    overwrite: boolean,
    peerSessionId = "",
  ) =>
    backend().TransferSFTP(
      sessionId,
      id,
      direction,
      sources,
      target,
      overwrite,
      peerSessionId,
    ),
  cancelSFTPTransfer: (sessionId: string, id: string) =>
    backend().CancelSFTPTransfer(sessionId, id),
  getAppVersion: () => backend().GetAppVersion(),
  getUpdateStatus: () => backend().GetUpdateStatus(),
  getAccount: () => backend().GetAccount(),
  beginLogin: () => backend().BeginLogin(),
  cancelLogin: () => backend().CancelLogin(),
  logout: () => backend().Logout(),
  getVaultStatus: () => backend().GetVaultStatus(),
  configureVaultSync: (password: string, direction = "") =>
    backend().ConfigureVaultSync(password, direction),
  syncVault: () => backend().SyncVault(),
  pauseVaultSync: () => backend().PauseVaultSync(),
  listHosts: () => backend().ListHosts(),
  listGroups: () => backend().ListGroups(),
  getWorkspace: () => backend().GetWorkspace(),
  saveGroup: (group: Group) => backend().SaveGroup(group),
  saveIdentity: (identity: Identity, password: string, passphrase: string) =>
    backend().SaveIdentity(identity, password, passphrase),
  deleteIdentity: (id: string) => backend().DeleteIdentity(id),
  createGroup: (name: string) => backend().CreateGroup(name),
  deleteGroup: (name: string) => backend().DeleteGroup(name),
  saveHost: (host: Host, password = "") => backend().SaveHost(host, password),
  deleteHost: (id: string) => backend().DeleteHost(id),
  chooseKey: () => backend().ChooseKey(),
  connectHost: (
    id: string,
    password: string,
    passphrase: string,
    fingerprint = "",
    workspaceId = "personal",
  ) =>
    backend().ConnectWorkspaceHost(
      workspaceId,
      id,
      password,
      passphrase,
      fingerprint,
    ),
  reconnectHost: (
    sessionId: string,
    hostId: string,
    password: string,
    passphrase: string,
    fingerprint = "",
    workspaceId = "personal",
  ) =>
    backend().ReconnectWorkspaceHost(
      sessionId,
      workspaceId,
      hostId,
      password,
      passphrase,
      fingerprint,
    ),
  getLocalShells: async () => backend().GetLocalShells(),
  startLocalShell: (shell: LocalShell) => backend().StartLocalShell(shell),
  writeSession: (id: string, data: string) => backend().WriteSession(id, data),
  resizeSession: (id: string, rows: number, columns: number) =>
    backend().ResizeSession(id, rows, columns),
  disconnectSession: (id: string) => backend().DisconnectSession(id),
};

export function onEvent(
  name: string,
  callback: (data: unknown) => void,
): () => void {
  return window.runtime?.EventsOn(name, callback) ?? (() => {});
}
