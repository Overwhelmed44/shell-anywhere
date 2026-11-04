import { useEffect, useId, useRef, useState } from "react";
import { ChevronDown, Cloud, LoaderCircle, LogIn, LogOut, UserRound } from "lucide-react";
import type { AccountAction, AccountStatus, VaultStatus } from "../api";

type Props = {
  account: AccountStatus;
  busy: boolean;
  onAction: (action: AccountAction) => void;
  vault: VaultStatus;
  onSyncSettings: () => void;
};

export default function AccountMenu({ account, busy, onAction, vault, onSyncSettings }: Props) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const firstItem = useRef<HTMLButtonElement>(null);
  const menuId = useId();
  const signedIn = Boolean(account.user_id);
  const menuVisible = signedIn && open;

  useEffect(() => {
    if (!menuVisible) return;

    firstItem.current?.focus();

    const onPointerDown = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setOpen(false);
        trigger.current?.focus();
      } else if (event.key === "Tab") {
        setOpen(false);
      } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const items = Array.from(container.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not(:disabled)') || []);
        const index = items.indexOf(document.activeElement as HTMLButtonElement);
        const direction = event.key === "ArrowDown" ? 1 : -1;
        items[(index + direction + items.length) % items.length]?.focus();
      }
    };

    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);

    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuVisible]);

  if (!signedIn) {
    return (
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
        {account.pending ? "Signing in…" : "Sign in"}
      </button>
    );
  }

  return (
    <div className="account-menu" ref={container}>
      <button
        ref={trigger}
        type="button"
        className="account-trigger"
        title={account.username}
        disabled={busy}
        aria-haspopup="menu"
        aria-expanded={menuVisible}
        aria-controls={menuVisible ? menuId : undefined}
        onClick={() => setOpen((value) => !value)}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        <UserRound size={16} aria-hidden="true" />
        <span>{account.username}</span>
        <ChevronDown size={14} aria-hidden="true" />
      </button>

      {menuVisible && (
        <div className="account-options" id={menuId} role="menu" aria-label="Oxee account">
          <button
            ref={firstItem}
            type="button"
            role="menuitem"
            className="account-sync-item"
            onClick={() => {
              setOpen(false);
              onSyncSettings();
            }}
          >
            <Cloud size={16} aria-hidden="true" />
            <span>Host syncing<small>{vault.locked ? "Enter vault password" : vault.error ? "Needs attention" : vault.busy ? "Syncing…" : vault.enabled ? vault.protected ? "Encrypted" : "Enabled" : "Set up sync"}</small></span>
          </button>
          <button
            type="button"
            role="menuitem"
            disabled={busy}
            onClick={() => {
              setOpen(false);
              trigger.current?.focus();
              onAction("logout");
            }}
          >
            <LogOut size={16} aria-hidden="true" /> Sign out
          </button>
        </div>
      )}
    </div>
  );
}
