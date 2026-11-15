import { useEffect, useRef, useState, type FormEvent } from "react";
import { Cloud, LockKeyhole, RefreshCw, X } from "lucide-react";
import { api, type VaultStatus } from "../api";
import { errorText } from "../utils";

type Props = {
  status: VaultStatus;
  onChange: (status: VaultStatus) => void;
  onClose: () => void;
};

export default function VaultDialog({ status, onChange, onClose }: Props) {
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const dialog = useRef<HTMLDivElement>(null);
  const working = busy || status.busy;
  const latest = useRef({ working, onClose });

  useEffect(() => {
    latest.current = { working, onClose };
  }, [working, onClose]);

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const element = dialog.current;
    element?.querySelector<HTMLElement>("input, button")?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !latest.current.working) {
        event.preventDefault();
        latest.current.onClose();
      }

      if (event.key === "Tab" && element) {
        const items = Array.from(element.querySelectorAll<HTMLElement>("button:not(:disabled), input:not(:disabled)"));
        const first = items[0];
        const last = items[items.length - 1];

        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      previous?.focus();
    };
  }, []);

  const enable = async (direction = "") => {
    if (working) return;

    if (status.locked && !password) {
      setError("Enter your vault password to unlock syncing");
      return;
    }

    if (!status.locked && !status.protected && password && password !== confirmation) {
      setError("The passwords do not match");
      return;
    }

    setBusy(true);
    setError("");

    try {
      const next = await api.configureVaultSync(password, direction);
      onChange(next);

      if (next.enabled && !next.needsChoice) {
        setPassword("");
        setConfirmation("");
      }
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      setBusy(false);
    }
  };

  const action = async (pause: boolean) => {
    setBusy(true);
    setError("");

    try {
      onChange(await (pause ? api.pauseVaultSync() : api.syncVault()));
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      setBusy(false);
    }
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    void enable();
  };

  return (
    <div className="modal-backdrop p-5" onMouseDown={(event) => {
      if (event.target === event.currentTarget && !working) onClose();
    }}>
      <div ref={dialog} className="modal vault-modal" role="dialog" aria-modal="true" aria-labelledby="vault-dialog-title">
        <div className="modal-heading">
          <div>
            <div className="eyebrow">Your Oxee account</div>
            <h2 id="vault-dialog-title">{status.locked ? "Unlock host syncing" : "Host syncing"}</h2>
          </div>
          <button type="button" className="icon-button" onClick={onClose} disabled={working} aria-label="Close sync settings">
            <X size={18} />
          </button>
        </div>

        {status.needsChoice ? (
          <div className="vault-content">
            <p className="modal-intro">This device and the cloud have different hosts. Choose which list to use for the first sync</p>
            <button type="button" className="vault-choice" disabled={working} onClick={() => void enable("download")}>
              <Cloud size={20} />
              <span><strong>Use cloud hosts</strong><small>{status.cloudHosts} hosts · replaces the host list on this device</small></span>
            </button>
            <button type="button" className="vault-choice" disabled={working} onClick={() => void enable("upload")}>
              <RefreshCw size={20} />
              <span><strong>Use this device’s hosts</strong><small>{status.localHosts} hosts · replaces the cloud host list</small></span>
            </button>
          </div>
        ) : status.enabled && !status.locked ? (
          <div className="vault-content">
            <p className="modal-intro">Your hosts, groups, passwords, private keys, and trusted servers sync automatically across your devices</p>
            <div className="vault-protection">
              {status.protected ? <LockKeyhole size={20} /> : <Cloud size={20} />}
              <div>
                <strong>{status.protected ? "Your connection vault is encrypted" : "Syncing with your Oxee account"}</strong>
                <p>{status.protected
                  ? "Enter your vault password on another device to restore your connections"
                  : "Sign in on another device to restore your connections without a vault password"}</p>
              </div>
            </div>
            {status.lastSynced && <p className="vault-note">Last synced {new Date(status.lastSynced).toLocaleString()}</p>}
            <div className="modal-actions">
              <button type="button" className="button secondary" disabled={working} onClick={() => void action(true)}>Pause syncing</button>
              <span className="spacer" />
              <button type="button" className="button primary" disabled={working} onClick={() => void action(false)}>
                {working ? "Syncing…" : "Sync now"}
              </button>
            </div>
          </div>
        ) : (
          <form className="vault-content" onSubmit={submit}>
            <p className="modal-intro">
              {status.locked
                ? "This cloud vault uses a password. Enter it to sync here, or keep using your local hosts without syncing"
                : "Sync your complete SSH connections across devices with your Oxee account"}
            </p>
            <div className="vault-protection">
              {status.protected || password ? <LockKeyhole size={20} /> : <Cloud size={20} />}
              <p>{status.locked
                ? "Your password unlocks your hosts, passwords, and private keys"
                : status.protected
                  ? "This device remembers your vault encryption key"
                  : "Add an optional vault password for end-to-end encryption"}</p>
            </div>
            <label className="field">
              <span>Vault password{!status.protected && !status.locked && <span className="optional">Optional</span>}</span>
              <input type="password" autoComplete={status.locked || status.protected ? "current-password" : "new-password"} value={password} onChange={(event) => setPassword(event.target.value)} placeholder={status.protected && !status.locked ? "Leave blank to use this device's saved vault key" : status.locked ? "Enter your vault password" : "Leave blank to sync without a password"} required={status.locked} disabled={working} />
            </label>
            {password && !status.locked && !status.protected && <label className="field">
              Confirm vault password
              <input type="password" autoComplete="new-password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={working} />
            </label>}
            <p className="vault-note">{status.protected || password
              ? "You'll need your vault password when setting up syncing on a new device"
              : "Without a vault password, access to your synced connections is managed by your Oxee account"}</p>
            <div className="modal-actions">
              <button type="button" className="button secondary" disabled={working} onClick={onClose}>Not now</button>
              <span className="spacer" />
              <button type="submit" className="button primary" disabled={working}>{working ? "Connecting…" : status.locked ? "Unlock syncing" : "Enable syncing"}</button>
            </div>
          </form>
        )}

        {(error || status.error) && <div className="form-error" role="alert">{error || status.error}</div>}
        <p className="vault-note vault-local-note">Sync includes SSH passwords, private keys, and saved key passphrases</p>
      </div>
    </div>
  );
}
