import { useState, type FormEvent } from "react";
import { KeyRound } from "lucide-react";
import Dropdown from "./Dropdown";
import EditorSidebar from "./EditorSidebar";
import { api, type Group, type Identity } from "../api";
import { errorText } from "../utils";

type Props = {
  group: Group | null;
  identities: Identity[];
  onClose: () => void;
  onSaved: (group: Group) => void;
};

export default function GroupDialog({
  group,
  identities,
  onClose,
  onSaved,
}: Props) {
  const [name, setName] = useState(group?.name ?? "");
  const [selectedIdentityId, setIdentityId] = useState(
    group?.defaultIdentityId ?? "",
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const identityId = identities.some(
    (identity) => identity.id === selectedIdentityId,
  )
    ? selectedIdentityId
    : "";

  const save = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      onSaved(
        await api.saveGroup({
          id: group?.id ?? "",
          name,
          defaultIdentityId: identityId,
        }),
      );
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <EditorSidebar
      title={group ? "Edit group" : "New group"}
      eyebrow="Organize hosts"
      busy={busy}
      onClose={onClose}
    >
      <p className="modal-intro">
        Organize hosts and optionally choose an identity they can inherit
      </p>
      <form onSubmit={save}>
        <label className="field">
          Group name
          <input
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Work"
            maxLength={80}
            required
          />
        </label>
        <Dropdown
          label="Default identity"
          optional
          className="group-default-field"
          value={identityId}
          onChange={setIdentityId}
          disabled={busy}
          icon={<KeyRound size={16} />}
          options={[
            { value: "", label: "No default identity" },
            ...identities.map((identity) => ({
              value: identity.id,
              label: `${identity.name} · ${identity.username}`,
            })),
          ]}
        />
        <p className="vault-note identity-note">
          Hosts use this identity when their credentials are set to Group
          default
        </p>
        {error && <div className="form-error">{error}</div>}
        <div className="modal-actions">
          <span className="spacer" />
          <button className="button secondary" type="button" onClick={onClose}>
            Cancel
          </button>
          <button className="button primary" type="submit" disabled={busy}>
            {busy ? "Saving…" : group ? "Save group" : "Create group"}
          </button>
        </div>
      </form>
    </EditorSidebar>
  );
}
