import { CircleHelp, Moon, Sun } from "lucide-react";
import type { LocalShell, Theme } from "../types";
import LocalTerminalMenu from "./LocalTerminalMenu";
import AccountMenu from "./AccountMenu";
import { api, type AccountAction, type AccountStatus, type VaultStatus } from "../api";

type Props = {
  label: string;
  theme: Theme;
  onToggleTheme: () => void;
  onStartLocal: (shell: LocalShell, name: string) => void;
  account: AccountStatus;
  accountBusy: boolean;
  onAccountAction: (action: AccountAction) => void;
  vault: VaultStatus;
  onSyncSettings: () => void;
};

export default function Topbar({
  label,
  theme,
  onToggleTheme,
  onStartLocal,
  account,
  accountBusy,
  onAccountAction,
  vault,
  onSyncSettings,
}: Props) {
  const toggleLabel =
    theme === "dark" ? "Switch to light theme" : "Switch to dark theme";
  return (
    <header className="topbar">
      <div className="breadcrumb">
        <span>Workspace</span>
        <span className="crumb-slash">/</span>
        <strong>{label}</strong>
      </div>
      <div className="topbar-actions">
        <LocalTerminalMenu onStart={onStartLocal} />
        <button
          className="icon-button theme-toggle"
          title={toggleLabel}
          aria-label={toggleLabel}
          onClick={onToggleTheme}
        >
          {theme === "dark" ? <Sun size={18} /> : <Moon size={18} />}
        </button>
        <button
          className="icon-button"
          title="About this app"
          aria-label="About this app"
          onClick={() =>
            void api.openExternalLink("https://oxee.net/services/shell-anywhere")
          }
        >
          <CircleHelp size={18} />
        </button>
        <AccountMenu
          account={account}
          busy={accountBusy}
          onAction={onAccountAction}
          vault={vault}
          onSyncSettings={onSyncSettings}
        />
      </div>
    </header>
  );
}
