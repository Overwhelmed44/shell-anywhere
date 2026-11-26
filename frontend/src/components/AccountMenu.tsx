import { LoaderCircle, LogIn, Settings2, UserRound } from "lucide-react";
import type { AccountAction, AccountStatus } from "../api";

type Props = {
  account: AccountStatus;
  busy: boolean;
  onAction: (action: AccountAction) => void;
  onSettings: () => void;
};

export default function AccountMenu({ account, busy, onAction, onSettings }: Props) {
  if (account.user_id) {
    return (
      <button
        type="button"
        className="account-trigger"
        title="Open settings"
        aria-label={`Open settings for ${account.username}`}
        onClick={onSettings}
      >
        <UserRound size={16} aria-hidden="true" />
        <span>{account.username}</span>
        <Settings2 size={14} aria-hidden="true" />
      </button>
    );
  }

  return (
    <div className="account-actions">
      <button type="button" className="settings-trigger" onClick={onSettings}>
        <Settings2 size={16} aria-hidden="true" /> Settings
      </button>
      <button
        type="button"
        className="account-sign-in"
        disabled={busy || account.pending}
        aria-busy={busy || account.pending}
        onClick={() => onAction("login")}
      >
        {busy || account.pending ? (
          <LoaderCircle size={16} className="account-spinner" aria-hidden="true" />
        ) : (
          <LogIn size={16} aria-hidden="true" />
        )}
        {account.pending ? "Signing in:" : "Sign in"}
      </button>
    </div>
  );
}
