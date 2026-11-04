import { useEffect, useRef, useState, type FormEvent } from "react";
import { KeyRound, LockKeyhole, Pencil, Plus, Trash2 } from "lucide-react";
import { api, type Group, type Host, type Identity } from "../api";
import { errorText } from "../utils";
import Dropdown from "./Dropdown";
import EditorSidebar from "./EditorSidebar";

type Props = {
  identities: Identity[];
  hosts: Host[];
  groups: Group[];
  onChanged: () => Promise<void>;
  onClose: () => void;
};

const emptyIdentity: Identity = {
  id: "",
  name: "",
  username: "root",
  authMethod: "password",
  keyPath: "",
  savedPassword: false,
};

export default function IdentitiesDialog({
  identities,
  hosts,
  groups,
  onChanged,
  onClose,
}: Props) {
  const [draft, setDraft] = useState<Identity | null>(null);
  const [password, setPassword] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const editing = draft !== null;

  useEffect(() => {
    const element = container.current;
    (
      element?.querySelector<HTMLInputElement>("input") ??
      element?.querySelector<HTMLButtonElement>("button")
    )?.focus();
  }, [editing]);

  const edit = (identity: Identity | null) => {
    setDraft(identity ? { ...identity } : null);
    setConfirmDelete(false);
    setPassword("");
    setPassphrase("");
    setError("");
  };

  const update = <K extends keyof Identity>(key: K, value: Identity[K]) => {
    setDraft((old) => (old ? { ...old, [key]: value } : old));
  };

  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (!draft || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.saveIdentity(draft, password, passphrase);
      await onChanged();
      edit(null);
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!draft || busy) return;
    setBusy(true);
    setError("");
    try {
      await api.deleteIdentity(draft.id);
      await onChanged();
      edit(null);
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      setBusy(false);
    }
  };

  const defaultGroups = draft
    ? groups.filter((group) => group.defaultIdentityId === draft.id)
    : [];
  const dependentHosts = draft
    ? hosts.filter(
        (host) =>
          host.identityId === draft.id ||
          (host.useGroupIdentity &&
            defaultGroups.some((group) => group.id === host.groupId)),
      )
    : [];
  const saved = identities.find((identity) => identity.id === draft?.id);

  return (
    <EditorSidebar
      title={
        draft ? (draft.id ? "Edit identity" : "New identity") : "Identities"
      }
      eyebrow="Personal workspace"
      busy={busy}
      onClose={onClose}
    >
      <div ref={container} className="identity-editor-content">
        {draft ? (
          <form onSubmit={save}>
            <p className="modal-intro">
              Reuse this username and password or key across your hosts
            </p>
            <div className="form-grid">
              <label className="field">
                Identity name
                <input
                  autoFocus
                  value={draft.name}
                  onChange={(event) => update("name", event.target.value)}
                  placeholder="Production deploy"
                  required
                  disabled={busy}
                />
              </label>
              <label className="field">
                Username
                <input
                  value={draft.username}
                  onChange={(event) => update("username", event.target.value)}
                  required
                  disabled={busy}
                />
              </label>
            </div>
            <Dropdown
              label="Authentication"
              className="auth-field"
              value={draft.authMethod}
              onChange={(value) =>
                update("authMethod", value as Identity["authMethod"])
              }
              disabled={busy}
              icon={
                draft.authMethod === "key" ? (
                  <KeyRound size={16} />
                ) : (
                  <LockKeyhole size={16} />
                )
              }
              options={[
                { value: "password", label: "Password" },
                { value: "key", label: "SSH key" },
              ]}
            />
            {draft.authMethod === "password" ? (
              <label className="field connection-auth-type">
                Password
                <input
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  placeholder={
                    saved?.savedPassword
                      ? "Leave blank to keep saved password"
                      : "Enter SSH password"
                  }
                  required={!saved?.savedPassword}
                  disabled={busy}
                />
              </label>
            ) : (
              <div className="identity-key-fields">
                <label className="field">
                  Private key file
                  <div className="file-input">
                    <input
                      value={draft.keyPath}
                      onChange={(event) =>
                        update("keyPath", event.target.value)
                      }
                      required
                      disabled={busy}
                    />
                    <button
                      type="button"
                      disabled={busy}
                      onClick={async () => {
                        try {
                          const path = await api.chooseKey();
                          if (path) update("keyPath", path);
                        } catch (reason) {
                          setError(errorText(reason));
                        }
                      }}
                    >
                      Browse
                    </button>
                  </div>
                </label>
                <label className="field">
                  Key passphrase <span className="optional">optional</span>
                  <input
                    type="password"
                    autoComplete="new-password"
                    value={passphrase}
                    onChange={(event) => setPassphrase(event.target.value)}
                    placeholder="Leave blank to keep a saved passphrase"
                    disabled={busy}
                  />
                </label>
              </div>
            )}
            <p className="vault-note identity-note">
              Changes apply to future connections from every host using this
              identity
            </p>
            {draft.authMethod === "password" && saved?.savedPassword && (
              <p className="vault-note saved-password-note">
                A password is already saved. Leave the password field blank to
                keep it, or enter a new one to replace it
              </p>
            )}
            {confirmDelete && (
              <div className="identity-delete-confirm" role="alert">
                <strong>Delete “{draft.name}”?</strong>
                <p>
                  {dependentHosts.length
                    ? `${dependentHosts.length} ${dependentHosts.length === 1 ? "host will" : "hosts will"} keep the credentials separately`
                    : "This removes the saved identity"}
                  {defaultGroups.length
                    ? `. The default will be cleared from ${defaultGroups.length} ${defaultGroups.length === 1 ? "group" : "groups"}`
                    : ""}
                </p>
                <div className="identity-delete-actions">
                  <button
                    type="button"
                    className="button secondary"
                    disabled={busy}
                    onClick={() => setConfirmDelete(false)}
                  >
                    Cancel deletion
                  </button>
                  <button
                    type="button"
                    className="button danger"
                    disabled={busy}
                    onClick={() => void remove()}
                  >
                    {busy ? "Deleting…" : "Delete identity"}
                  </button>
                </div>
              </div>
            )}
            {error && (
              <div className="form-error" role="alert">
                {error}
              </div>
            )}
            <div className="modal-actions">
              {draft.id && !confirmDelete && (
                <button
                  type="button"
                  className="danger-text"
                  disabled={busy}
                  onClick={() => setConfirmDelete(true)}
                >
                  <Trash2 size={15} /> Delete identity
                </button>
              )}
              <span className="spacer" />
              <button
                type="button"
                className="button secondary"
                disabled={busy}
                onClick={() => edit(null)}
              >
                Back
              </button>
              <button
                type="submit"
                className="button primary"
                disabled={busy || confirmDelete}
              >
                {busy ? "Saving…" : "Save identity"}
              </button>
            </div>
          </form>
        ) : (
          <>
            <p className="modal-intro">
              Save credentials once, then use them for hosts or as a group
              default
            </p>
            <div className="identity-list">
              {identities.map((identity) => (
                <button
                  type="button"
                  key={identity.id}
                  className="identity-row"
                  onClick={() => edit(identity)}
                >
                  {identity.authMethod === "key" ? (
                    <KeyRound size={20} />
                  ) : (
                    <LockKeyhole size={20} />
                  )}
                  <span>
                    <strong>{identity.name}</strong>
                    <small>
                      {identity.username} ·{" "}
                      {identity.authMethod === "key" ? "SSH key" : "Password"}
                    </small>
                  </span>
                  <Pencil size={16} />
                </button>
              ))}
              {identities.length === 0 && (
                <div className="identities-empty">
                  <div className="no-idt-yet-t">
                    <KeyRound size={20} />
                    <strong>No identities yet</strong>
                  </div>
                  <p>Create one to reuse credentials across hosts</p>
                </div>
              )}
            </div>
            <div className="modal-actions">
              <button
                type="button"
                className="button secondary"
                onClick={onClose}
              >
                Done
              </button>
              <span className="spacer" />
              <button
                type="button"
                className="button primary"
                onClick={() => edit(emptyIdentity)}
              >
                <Plus size={16} /> New identity
              </button>
            </div>
          </>
        )}
      </div>
    </EditorSidebar>
  );
}
