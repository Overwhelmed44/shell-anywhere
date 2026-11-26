import { CircleHelp } from "lucide-react";
import type { LocalShell } from "../types";
import LocalTerminalMenu from "./LocalTerminalMenu";
import AccountMenu from "./AccountMenu";
import { api, type AccountAction, type AccountStatus } from "../api";

type Props = {
  label: string;
  onStartLocal: (shell: LocalShell, name: string) => void;
  account: AccountStatus;
  accountBusy: boolean;
  onAccountAction: (action: AccountAction) => void;
  onSettings: () => void;
};

export default function Topbar({
  label,
  onStartLocal,
  account,
  accountBusy,
  onAccountAction,
  onSettings,
}: Props) {
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
          onSettings={onSettings}
        />
      </div>
    </header>
  );
}
