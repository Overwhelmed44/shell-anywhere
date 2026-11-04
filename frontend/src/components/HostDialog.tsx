import { useRef, useState, type FormEvent } from "react";
import { KeyRound, LockKeyhole, Trash2 } from "lucide-react";
import { api, type Host, type Group, type Identity } from "../api";
import { errorText } from "../utils";
import GroupSelect from "./GroupSelect";
import Dropdown from "./Dropdown";
import EditorSidebar from "./EditorSidebar";

function HostDialog({
  host,
  groups,
  identities,
  onManageIdentities,
  onClose,
  onSaved,
  onDelete,
}: {
  host: Host;
  groups: Group[];
  identities: Identity[];
  onManageIdentities: () => void;
  onClose: () => void;
  onSaved: (host: Host) => void;
  onDelete: (id: string) => void;
}) {
  const [storedDraft, setStoredDraft] = useState<Host>(() => ({
    ...host,
    useGroupIdentity:
      host.useGroupIdentity ||
      (!host.id &&
        !host.identityId &&
        Boolean(
          groups.find((group) => group.id === host.groupId)?.defaultIdentityId,
        )),
  }));
  const credentialsChosen = useRef(false);
  const [inheritedAtOpen] = useState(host.useGroupIdentity);
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const currentDraft = (old: Host): Host => {
    const deleted =
      old.identityId &&
      !identities.some((identity) => identity.id === old.identityId);
    const detached =
      old.id &&
      inheritedAtOpen &&
      old.useGroupIdentity &&
      !host.useGroupIdentity &&
      !groups.find((group) => group.id === old.groupId)?.defaultIdentityId;
    if (!deleted && !detached) return old;
    return {
      ...old,
      identityId: "",
      useGroupIdentity: false,
      username: host.username || "root",
      authMethod: host.authMethod || "password",
      keyPath: host.keyPath,
      savedPassword: host.savedPassword,
    };
  };
  const draft = currentDraft(storedDraft);
  const setDraft = (update: (old: Host) => Host) =>
    setStoredDraft((old) => update(currentDraft(old)));
  const update = <K extends keyof Host>(key: K, value: Host[K]) => {
    if (key === "username" || key === "keyPath" || key === "authMethod")
      credentialsChosen.current = true;
    setDraft((old) => ({ ...old, [key]: value }));
  };
  const ownCredentials = !draft.identityId && !draft.useGroupIdentity;
  const ownSavedPassword =
    host.savedPassword && !host.identityId && !host.useGroupIdentity;
  const defaultIdentity = identities.find(
    (identity) =>
      identity.id ===
      groups.find((group) => group.id === draft.groupId)?.defaultIdentityId,
  );
  const selectedIdentity = draft.useGroupIdentity
    ? defaultIdentity
    : identities.find((identity) => identity.id === draft.identityId);
  const save = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      onSaved(await api.saveHost(draft, password));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <EditorSidebar
      title={host.id ? "Edit host" : "New host"}
      eyebrow="Host details"
      busy={busy}
      onClose={onClose}
    >
      <p className="modal-intro">
        Save a server and choose separate or reusable credentials
      </p>
      <form onSubmit={save}>
        <div className="form-grid">
          <label className="field span-2">
            <span>
              Display name <span className="optional">optional</span>
            </span>
            <input
              value={draft.name}
              onChange={(event) => update("name", event.target.value)}
              placeholder={draft.address || "Uses hostname or IP if blank"}
            />
          </label>
          <label className="field">
            Hostname or IP
            <input
              autoFocus
              value={draft.address}
              onChange={(event) => update("address", event.target.value)}
              placeholder="server.example.com"
              required
            />
          </label>
          <label className="field narrow">
            Port
            <input
              type="number"
              min="1"
              max="65535"
              value={draft.port}
              onChange={(event) => update("port", Number(event.target.value))}
              required
            />
          </label>
          <GroupSelect
            groups={groups}
            value={draft.groupId}
            onChange={(group) =>
              setDraft((old) => ({
                ...old,
                groupId: group,
                identityId:
                  !host.id && !credentialsChosen.current ? "" : old.identityId,
                useGroupIdentity: !group
                  ? false
                  : !host.id && !credentialsChosen.current
                    ? Boolean(
                        groups.find((item) => item.id === group)
                          ?.defaultIdentityId,
                      )
                    : old.useGroupIdentity,
              }))
            }
          />
        </div>
        <div className="auth-field">
          <Dropdown
            label="Credentials"
            value={draft.useGroupIdentity ? "group" : draft.identityId || "own"}
            icon={<KeyRound size={16} />}
            onChange={(value) => {
              credentialsChosen.current = true;
              setDraft((old) => ({
                ...old,
                identityId: value === "own" || value === "group" ? "" : value,
                useGroupIdentity: value === "group",
              }));
              setPassword("");
            }}
            options={[
              { value: "own", label: "Separate credentials" },
              ...(draft.groupId
                ? [
                    {
                      value: "group",
                      label: `Group default — ${defaultIdentity?.name ?? "ask when connecting"}`,
                    },
                  ]
                : []),
              ...identities.map((identity) => ({
                value: identity.id,
                label: `${identity.name} · ${identity.username}`,
                section: "Identities",
              })),
            ]}
          />
          <button
            type="button"
            className="identity-manage-link"
            onClick={onManageIdentities}
          >
            Manage identities
          </button>
        </div>
        {!ownCredentials && (
          <p className="vault-note identity-note">
            {selectedIdentity
              ? `Uses ${selectedIdentity.username} with ${selectedIdentity.authMethod === "key" ? "an SSH key" : "a password"} from ${selectedIdentity.name}`
              : "No group default is selected yet, so the app will ask for a password when connecting"}
          </p>
        )}
        {ownCredentials && (
          <>
            <label className="field connection-auth-type">
              Username
              <input
                value={draft.username}
                onChange={(event) => update("username", event.target.value)}
                placeholder="root"
                required
              />
            </label>
            <div className="field auth-field">
              <span>Authentication</span>
              <div className="segmented">
                <button
                  type="button"
                  className={draft.authMethod === "password" ? "selected" : ""}
                  onClick={() => update("authMethod", "password")}
                >
                  <LockKeyhole size={15} /> Password
                </button>
                <button
                  type="button"
                  className={draft.authMethod === "key" ? "selected" : ""}
                  onClick={() => update("authMethod", "key")}
                >
                  <KeyRound size={15} /> SSH key
                </button>
              </div>
            </div>
            <div className="connection-auth-type">
              {draft.authMethod === "password" && (
                <label className="field saved-password-field">
                  Password
                  <input
                    type="password"
                    value={password}
                    onChange={(event) => {
                      credentialsChosen.current = true;
                      setPassword(event.target.value);
                    }}
                    placeholder={
                      ownSavedPassword
                        ? "Leave blank to keep saved password"
                        : "Enter SSH password"
                    }
                    required={!ownSavedPassword}
                    autoComplete="new-password"
                  />
                </label>
              )}
              {draft.authMethod === "key" && (
                <label className="field">
                  Private key file
                  <div className="file-input">
                    <input
                      value={draft.keyPath}
                      onChange={(event) =>
                        update("keyPath", event.target.value)
                      }
                      placeholder="Path to your private key"
                      required
                    />
                    <button
                      type="button"
                      onClick={async () => {
                        try {
                          const path = await api.chooseKey();
                          if (path) update("keyPath", path);
                        } catch (err) {
                          setError(errorText(err));
                        }
                      }}
                    >
                      Browse
                    </button>
                  </div>
                </label>
              )}
            </div>
          </>
        )}
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions">
          {host.id && (
            <button
              className="danger-text"
              type="button"
              onClick={() => onDelete(host.id)}
            >
              <Trash2 size={15} /> Delete host
            </button>
          )}
          <span className="spacer" />
          <button className="button secondary" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button primary" type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save host"}
          </button>
        </div>
      </form>
    </EditorSidebar>
  );
}

export default HostDialog;
