import { ArrowRight, Copy, ShieldCheck, X } from "lucide-react";
import type { AuthPrompt } from "../types";

function ConnectDialog({
  prompt,
  busy,
  onChange,
  onConnect,
  onClose,
}: {
  prompt: AuthPrompt;
  busy: boolean;
  onChange: (prompt: AuthPrompt) => void;
  onConnect: () => void;
  onClose: () => void;
}) {
  const confirmingKey = Boolean(prompt.fingerprint);
  return (
    <div
      className="modal-backdrop p-5"
      onMouseDown={(event) => {
        if (!busy && event.target === event.currentTarget) onClose();
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
          <button className="icon-button" onClick={onClose} aria-label="Close" disabled={busy}>
            <X size={18} />
          </button>
        </div>
        <h2 id="connect-dialog-title">
          {prompt.keyMismatch
            ? "Server key has changed"
            : confirmingKey
              ? "Verify this server"
              : `${prompt.reconnectSessionId ? "Reconnect to" : "Connect to"} ${prompt.host.name}${prompt.mode === "sftp" ? " over SFTP" : ""}`}
        </h2>
        <p className="modal-intro">
          {prompt.keyMismatch
            ? `The key from ${prompt.host.username}@${prompt.host.address}:${prompt.host.port} does not match its saved key. This can happen after a server reinstall or if the connection is being intercepted. Verify the new fingerprint before overwriting the saved key`
            : confirmingKey
              ? "This server is new to Shell Anywhere. Check its fingerprint against a trusted source before continuing"
              : `${prompt.host.username}@${prompt.host.address}:${prompt.host.port}`}
        </p>
        {prompt.keyMismatch && [...new Set(prompt.previousFingerprints)].map((fingerprint) => (
          <div className="fingerprint" key={fingerprint}>
            <span>Saved fingerprint</span>
            <code>{fingerprint}</code>
          </div>
        ))}
        {confirmingKey ? (
          <div className="fingerprint">
            <span>{prompt.keyMismatch ? "New " : ""}{prompt.keyType} fingerprint</span>
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
        <div className="modal-actions">
          <span className="spacer" />
          <button className="button secondary" onClick={onClose} disabled={busy} autoFocus={prompt.keyMismatch}>
            {prompt.keyMismatch ? "Abort connection" : "Cancel"}
          </button>
          <button
            className="button primary"
            disabled={busy}
            onClick={onConnect}
          >
            {busy
              ? "Connecting…"
              : prompt.keyMismatch
                ? "Overwrite key & connect"
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
