import { ArrowRight, Copy, LockKeyhole, ShieldCheck, X } from "lucide-react";
import type { AuthPrompt } from "../types";

function ConnectDialog({
  prompt,
  busy,
  error,
  onChange,
  onConnect,
  onClose,
}: {
  prompt: AuthPrompt;
  busy: boolean;
  error: string;
  onChange: (prompt: AuthPrompt) => void;
  onConnect: () => void;
  onClose: () => void;
}) {
  const confirmingKey = Boolean(prompt.fingerprint);
  return (
    <div
      className="modal-backdrop p-5"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        className="modal connect-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="connect-dialog-title"
      >
        <div className="modal-heading">
          <div className="modal-icon">
            <ShieldCheck size={22} />
          </div>
          <button className="icon-button" onClick={onClose} aria-label="Close">
            <X size={18} />
          </button>
        </div>
        <h2 id="connect-dialog-title">
          {confirmingKey
            ? "Verify this server"
            : `${prompt.reconnectSessionId ? "Reconnect to" : "Connect to"} ${prompt.host.name}${prompt.mode === "sftp" ? " over SFTP" : ""}`}
        </h2>
        <p className="modal-intro">
          {confirmingKey
            ? "This server is new to Shell Anywhere. Check its fingerprint against a trusted source before continuing"
            : `${prompt.host.username}@${prompt.host.address}:${prompt.host.port}`}
        </p>
        {confirmingKey ? (
          <div className="fingerprint">
            <span>{prompt.keyType} fingerprint</span>
            <code>{prompt.fingerprint}</code>
            <button
              type="button"
              className="copy-button"
              onClick={() => {
                void navigator.clipboard.writeText(prompt.fingerprint ?? "");
              }}
              aria-label="Copy fingerprint"
            >
              <Copy size={15} />
            </button>
          </div>
        ) : prompt.host.authMethod === "password" ? (
          <label className="field">
            {prompt.host.savedPassword
              ? "Password override (optional)"
              : "Password"}
            <input
              type="password"
              autoFocus={!prompt.host.savedPassword}
              value={prompt.password}
              onChange={(event) =>
                onChange({ ...prompt, password: event.target.value })
              }
              onKeyDown={(event) => {
                if (event.key === "Enter") onConnect();
              }}
              placeholder={
                prompt.host.savedPassword
                  ? "Leave blank to use saved password"
                  : "Enter SSH password"
              }
            />
          </label>
        ) : (
          <label className="field">
            <span>
              Key passphrase <span className="optional">if needed</span>
            </span>
            <input
              type="password"
              autoFocus
              value={prompt.passphrase}
              onChange={(event) =>
                onChange({ ...prompt, passphrase: event.target.value })
              }
              onKeyDown={(event) => {
                if (event.key === "Enter") onConnect();
              }}
              placeholder="Leave empty if unencrypted"
            />
          </label>
        )}
        <div className="privacy-note">
          <LockKeyhole size={14} />{" "}
          {prompt.host.useGroupIdentity && !prompt.host.savedPassword
            ? "Use this password for this connection, or set a group identity to reuse saved credentials"
            : prompt.host.authMethod === "password"
              ? "Your password is saved after connecting and included in encrypted syncing"
              : "Your key passphrase is saved after connecting and included in encrypted syncing"}
        </div>
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions">
          <span className="spacer" />
          <button className="button secondary" onClick={onClose}>
            Cancel
          </button>
          <button
            className="button primary"
            disabled={busy}
            onClick={onConnect}
          >
            {busy
              ? "Connecting…"
              : confirmingKey
                ? "Trust & connect"
                : prompt.reconnectSessionId
                  ? "Reconnect"
                  : "Connect"}{" "}
            <ArrowRight size={15} />
          </button>
        </div>
      </div>
    </div>
  );
}

export default ConnectDialog;
