import { useEffect, useRef, useState, type FormEvent } from "react";
import { ArrowLeft, Check, ChevronRight, Cloud, LoaderCircle, LogIn, LogOut, Moon, Palette, Plug, Sun, TerminalSquare, UserRound } from "lucide-react";
import type { AccountAction, AccountStatus, VaultStatus } from "../api";
import { defaultSettings, type AppSettings } from "../settings";
import { errorText } from "../utils";
import { themeIds, themes } from "../themes";
import "./SettingsPage.css";

type Props = {
  settings: AppSettings;
  ready: boolean;
  loadError: string;
  onRetry: () => void;
  onSave: (settings: AppSettings) => Promise<void>;
  onClose: () => void;
  account: AccountStatus;
  accountBusy: boolean;
  onAccountAction: (action: AccountAction) => void;
  vault: VaultStatus;
  onSyncSettings: () => void;
  suspended: boolean;
};

const sections = [
  { id: "appearance", label: "Appearance", icon: Palette },
  { id: "terminal", label: "Terminal", icon: TerminalSquare },
  { id: "connections", label: "Connections", icon: Plug },
  { id: "account", label: "Account & sync", icon: UserRound },
] as const;
type Section = typeof sections[number]["id"];

export default function SettingsPage({ settings, ready, loadError, onRetry, onSave, onClose, account, accountBusy, onAccountAction, vault, onSyncSettings, suspended }: Props) {
  const [section, setSection] = useState<Section>("appearance");
  const [draft, setDraft] = useState(settings);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const page = useRef<HTMLElement>(null);
  const saving = useRef(false);
  const dirty = (Object.keys(settings) as (keyof AppSettings)[]).some((key) => settings[key] !== draft[key]);

  const previousSettings = useRef(settings);
  useEffect(() => {
    const previous = previousSettings.current;
    previousSettings.current = settings;
    // Refresh untouched fields while preserving edits made in the open page
    setDraft((current) => {
      const next = { ...current };
      for (const key of Object.keys(settings) as (keyof AppSettings)[]) {
        if (current[key] === previous[key]) Object.assign(next, { [key]: settings[key] });
      }
      return next;
    });
  }, [settings]);

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    page.current?.focus();
    return () => {
      if (previous?.isConnected) previous.focus();
    };
  }, []);

  const change = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    setDraft((old) => ({ ...old, [key]: value }));
    setError("");
  };
  const close = () => {
    if (!saving.current) onClose();
  };
  const save = async (event: FormEvent) => {
    event.preventDefault();
    if (saving.current || !ready) return;
    const ranges = [
      ["connectionTimeoutSeconds", 1, 300, "connections", "Connection timeout must be between 1 and 300 seconds"],
      ["terminalFontSize", 10, 24, "terminal", "Font size must be between 10 and 24"],
      ["terminalScrollback", 100, 100000, "terminal", "Scrollback must be between 100 and 100000 lines"],
    ] as const;
    for (const [key, minimum, maximum, page, message] of ranges) {
      if (!Number.isInteger(draft[key]) || draft[key] < minimum || draft[key] > maximum) {
        setSection(page);
        setError(message);
        return;
      }
    }
    saving.current = true;
    setBusy(true);
    setError("");
    try {
      await onSave(draft);
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      saving.current = false;
      setBusy(false);
    }
  };

  return (
      <main
        ref={page}
        className="settings-page"
        inert={suspended}
        aria-labelledby="settings-title"
        aria-busy={busy || (!ready && !loadError)}
        tabIndex={-1}
      >
        <header className="settings-heading">
          <button type="button" className="settings-back" onClick={close} disabled={busy}><ArrowLeft size={18} /> Back to workspace</button>
          <div><h1 id="settings-title">Settings</h1><p>Make Shell Anywhere your own</p></div>
        </header>
        <form className="settings-form" onSubmit={(event) => void save(event)} noValidate>
          <div className="settings-layout">
            <nav className="settings-nav" aria-label="Settings sections">
              {sections.map(({ id, label, icon: Icon }) => (
                <button key={id} type="button" className={section === id ? "active" : ""} aria-current={section === id ? "page" : undefined} onClick={() => setSection(id)}>
                  <Icon size={17} /> {label}
                </button>
              ))}
              <p>{vault.enabled ? "Preferences sync with your Oxee account" : "Preferences are saved on this device"}</p>
            </nav>
            <div className="settings-panel">
              {!ready ? (
                <div className="settings-loading" role="status">
                  {loadError ? <><p>{loadError}</p><button type="button" className="button secondary" onClick={onRetry}>Try again</button></> : <><LoaderCircle size={20} className="account-spinner" /> Loading settings</>}
                </div>
              ) : (
                <fieldset disabled={busy}>
                  {section === "appearance" && <>
                    <h3>Appearance</h3>
                    <p className="settings-description">Make your workspace feel at home</p>
                    <div className="settings-label" id="theme-label">Theme</div>
                    <div role="group" aria-labelledby="theme-label">
                      {(["dark", "light"] as const).map((mode) => (
                        <div className="theme-category" key={mode}>
                          <h4>{mode === "dark" ? "Dark themes" : "Light themes"}</h4>
                          <div className="theme-options">
                            {themeIds.filter((id) => themes[id].mode === mode).map((id) => {
                              const theme = themes[id];
                              return <button key={id} type="button" className={`theme-option${draft.theme === id ? " selected" : ""}`} aria-pressed={draft.theme === id} onClick={() => change("theme", id)}>
                                <span className="theme-preview" style={theme.variables} aria-hidden="true"><i /><span><b /><b /><b /></span></span>
                                <span className="theme-option-label">{mode === "dark" ? <Moon size={16} /> : <Sun size={16} />}<strong>{theme.label}</strong>{draft.theme === id && <Check size={16} className="theme-selected" />}</span>
                                <span className="theme-option-description">{theme.description}</span>
                              </button>;
                            })}
                          </div>
                        </div>
                      ))}
                    </div>
                    <p className="settings-hint">Applies to the interface and your terminals when saved</p>
                  </>}
                  {section === "terminal" && <>
                    <h3>Terminal</h3>
                    <p className="settings-description">Fine-tune SSH and local terminals</p>
                    <div className="settings-fields">
                      <label className="field">Font size<span className="settings-input-unit"><input type="number" min="10" max="24" step="1" required value={draft.terminalFontSize || ""} onChange={(event) => change("terminalFontSize", event.target.valueAsNumber || 0)} /><span>px</span></span></label>
                      <label className="field">Scrollback<span className="settings-input-unit"><input type="number" min="100" max="100000" step="100" required value={draft.terminalScrollback || ""} onChange={(event) => change("terminalScrollback", event.target.valueAsNumber || 0)} /><span>lines</span></span></label>
                    </div>
                    <label className="settings-switch-row"><span><strong>Blinking cursor</strong><small>Keep the cursor visible while you work</small></span><input type="checkbox" role="switch" checked={draft.terminalCursorBlink} onChange={(event) => change("terminalCursorBlink", event.target.checked)} /></label>
                    <div className="terminal-settings-preview" style={{ ...themes[draft.theme].variables, color: themes[draft.theme].terminal.foreground, backgroundColor: themes[draft.theme].terminal.background, fontSize: Math.max(10, Math.min(24, draft.terminalFontSize)) }} aria-label="Terminal font preview"><span>root@server</span>:~$ ssh anywhere<span className={draft.terminalCursorBlink ? "preview-cursor blinking" : "preview-cursor"} aria-hidden="true" /></div>
                    <p className="settings-hint">Applies to open terminals when saved. Reducing scrollback discards older lines</p>
                  </>}
                  {section === "connections" && <>
                    <h3>Connections</h3>
                    <p className="settings-description">Choose how long to wait for a server</p>
                    <label className="field settings-timeout">Connection timeout<span className="settings-input-unit"><input type="number" min="1" max="300" step="1" required value={draft.connectionTimeoutSeconds || ""} onChange={(event) => change("connectionTimeoutSeconds", event.target.valueAsNumber || 0)} aria-describedby="timeout-hint" /><span>seconds</span></span></label>
                    <p className="settings-hint" id="timeout-hint">Time allowed to connect and authenticate over SSH or SFTP, from 1 to 300 seconds</p>
                    <div className="settings-note"><Plug size={18} /><p>Applies to new connections and reconnects. Established sessions stay open</p></div>
                  </>}
                  {section === "account" && <>
                    <div className="settings-signed-out-title">
                      <Cloud size={30} />
                      <h3>Account & sync</h3>
                    </div>

                    <p className="settings-description">Bring your connections and preferences to your other devices</p>
                    {account.user_id ? <>
                      <div className="settings-account"><span className="settings-avatar"><UserRound size={22} /></span><div><strong>{account.username}</strong><small>Signed in with Oxee</small></div></div>
                      <button type="button" className="settings-sync" onClick={onSyncSettings}><Cloud size={20} /><span><strong>Cloud syncing</strong><small>{vault.locked ? "Enter your vault password" : vault.error ? "Needs attention" : vault.busy ? "Syncing hosts and settings" : vault.enabled ? "Syncing hosts and settings automatically" : "Set up syncing across devices"}</small></span><ChevronRight size={17} /></button>
                      <button type="button" className="button secondary settings-sign-out" disabled={accountBusy} onClick={() => onAccountAction("logout")}><LogOut size={16} /> Sign out</button>
                    </> : <div className="settings-signed-out">

                            <strong>Your hosts, wherever you go</strong>
                            <p>Sign in with Oxee to sync hosts, identities, saved credentials, and app settings across your devices</p>
                            <button type="button" className="button primary" disabled={accountBusy} onClick={() => onAccountAction(account.pending ? "cancel" : "login")}><LogIn size={16} />{account.pending ? "Cancel sign-in" : "Sign in with Oxee"}</button>{account.pending && <small role="status">Continue signing in through your browser</small>}
                          </div>
                      }
                  </>}
                </fieldset>
              )}
            </div>
          </div>
          {error && <div className="form-error settings-error" role="alert">{error}</div>}
          <div className="settings-footer">
            <button type="button" className="settings-reset" disabled={busy || !ready} onClick={() => { setDraft(defaultSettings); setError(""); }}>Restore defaults</button>
            <span className="spacer" />
            <button type="button" className="button secondary" disabled={busy || !dirty} onClick={() => { setDraft(settings); setError(""); }}>Discard changes</button>
            <button type="submit" className="button primary" disabled={busy || !ready || !dirty}>{busy && <LoaderCircle size={15} className="account-spinner" />}{busy ? "Saving:" : "Save changes"}</button>
          </div>
        </form>
      </main>
  );
}
