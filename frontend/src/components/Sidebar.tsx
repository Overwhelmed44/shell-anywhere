import {
  Folder,
  Layers3,
  Plus,
  ShieldCheck,
  Star,
  LogIn,
  UserRound,
  KeyRound,
} from "lucide-react";
import type {
  Host,
  AccountAction,
  AccountStatus,
  UpdateStatus,
  Group,
} from "../api";

type Props = {
  update: UpdateStatus;
  account: AccountStatus;
  accountBusy: boolean;
  onAccountAction: (action: AccountAction) => void;
  hosts: Host[];
  groups: Group[];
  onIdentities: () => void;
  selectedGroup: string;
  onSelectGroup: (group: string) => void;
  onNewGroup: () => void;
};

export default function Sidebar({
  update,
  account,
  accountBusy,
  onAccountAction,
  hosts,
  groups,
  selectedGroup,
  onSelectGroup,
  onNewGroup,
  onIdentities,
}: Props) {
  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-mark">
          <img src="/favicon.svg" alt="" />
        </div>
        <div>
          <strong>
            Shell <span>Anywhere</span>
          </strong>
          <small>By Oxee</small>
        </div>
      </div>
      <div className="workspace-label">
        <span>Personal workspace</span>
      </div>
      <nav className="side-nav" aria-label="Host groups">
        <button
          className={selectedGroup === "All hosts" ? "active" : ""}
          onClick={() => onSelectGroup("All hosts")}
        >
          <Layers3 size={17} /> All hosts{" "}
          <span className="nav-count">{hosts.length}</span>
        </button>
        <button
          className={selectedGroup === "Favorites" ? "active" : ""}
          onClick={() => onSelectGroup("Favorites")}
        >
          <Star size={17} /> Favorites{" "}
          <span className="nav-count">
            {hosts.filter((host) => host.favorite).length}
          </span>
        </button>
        <button onClick={onIdentities}>
          <KeyRound size={17} /> Identities
        </button>
      </nav>
      <div className="side-section-heading">
        <div className="side-section-label">GROUPS</div>
        <button
          type="button"
          onClick={onNewGroup}
          aria-label="New group"
          title="New group"
        >
          <Plus size={16} />
        </button>
      </div>
      {groups.length > 0 && (
        <nav className="side-nav group-nav">
          {groups.map((group) => (
            <button
              key={group.id}
              className={selectedGroup === group.id ? "active" : ""}
              onClick={() => onSelectGroup(group.id)}
            >
              <Folder size={16} /> {group.name}{" "}
              <span className="nav-count">
                {hosts.filter((host) => host.groupId === group.id).length}
              </span>
            </button>
          ))}
        </nav>
      )}
      <div className="sidebar-bottom">
        {!account.user_id && (
          <div className="oxee-account" aria-busy={accountBusy}>
            <div className="oxee-account-identity">
              <UserRound size={18} aria-hidden="true" />
              <div>
                <strong>Your Oxee account</strong>
                <small role="status">
                  {account.pending
                    ? "Continue in your browser"
                    : "Sign in to sync your hosts across devices"}
                </small>
              </div>
            </div>

            {account.pending ? (
              <button
                type="button"
                disabled={accountBusy}
                onClick={() => onAccountAction("cancel")}
              >
                Cancel sign-in
              </button>
            ) : (
              <button
                type="button"
                disabled={accountBusy}
                onClick={() => onAccountAction("login")}
              >
                <LogIn size={15} aria-hidden="true" />
                {accountBusy ? "Please wait…" : "Sign in with Oxee"}
              </button>
            )}
          </div>
        )}
        <div className="security-card p-4">
          <div className="sidebar-pbdesign">
            <div className="security-icon">
              <ShieldCheck size={17} />
            </div>
            <strong>Private by design</strong>
          </div>
          <p>
            Encrypted syncing brings your hosts, passwords, and private keys to
            your other devices
          </p>
        </div>
        <div
          className="sidebar-footer"
          data-update-state={update.state}
          role="status"
          title={
            update.latestVersion
              ? `Latest release: v${update.latestVersion}`
              : undefined
          }
        >
          <span className="status-dot" aria-hidden="true" />
          <span className="update-label">
            {update.state === "update_available"
              ? "New version available"
              : update.state === "up_to_date"
                ? "Up to date"
                : update.state === "checking"
                  ? "Checking for updates…"
                  : update.state === "no_release"
                    ? "No releases yet"
                    : "Couldn’t check updates"}
          </span>
          <span className="app-version">
            {update.currentVersion ? `v${update.currentVersion}` : ""}
          </span>
        </div>
      </div>
    </aside>
  );
}
