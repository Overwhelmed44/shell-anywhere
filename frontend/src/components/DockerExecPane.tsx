import { useEffect, useRef, useState } from "react";
import { LoaderCircle, RefreshCw, Square } from "lucide-react";
import { api, onEvent } from "../api";
import { clearTerminal } from "../terminalBus";
import TerminalPane from "../TerminalPane";
import { errorText } from "../utils";
import type { Theme } from "../types";

type Props = {
  sessionId: string; containerId: string; name: string;
  active: boolean; focused: boolean; connected: boolean; theme: Theme;
};

export default function DockerExecPane({ sessionId, containerId, name, active, focused, connected, theme }: Props) {
  const [shell, setShell] = useState<"sh" | "bash">("sh");
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState({ id: "", pending: true, closed: false, reason: "", error: "" });
  const current = useRef("");
  const [stopping, setStopping] = useState(false);
  useEffect(() => {
    if (!connected) return;
    let stopped = false;
    let id = "";
    const earlyClosed = new Map<string, string>();
    const offClosed = onEvent("terminal:closed", (data) => {
      const event = data as { sessionId: string; reason: string };
      if (event.sessionId === id) {
        setState((old) => ({ ...old, closed: true, reason: event.reason }));
      } else if (!id) {
        earlyClosed.set(event.sessionId, event.reason);
      }
    });
    // Defer startup so Strict Mode's discarded effect never opens a shell.
    void Promise.resolve().then(async () => {
      if (stopped) return;
      setState({ id: "", pending: true, closed: false, reason: "", error: "" });
      try {
        id = await api.startDockerExec(sessionId, containerId, shell);
        if (stopped) {
          await api.disconnectSession(id).catch(() => {});
          clearTerminal(id);
          return;
        }
        current.current = id;
        setState({ id, pending: false, closed: earlyClosed.has(id), reason: earlyClosed.get(id) ?? "", error: "" });
        earlyClosed.clear();
      } catch (reason) {
        if (!stopped) setState({ id: "", pending: false, closed: true, reason: "", error: errorText(reason) });
      }
    });
    return () => {
      stopped = true;
      offClosed();
      if (id) {
        void api.disconnectSession(id).catch(() => {});
        clearTerminal(id);
      }
      current.current = "";
    };
  }, [sessionId, containerId, shell, attempt, connected]);
  const end = async () => {
    if (!current.current || stopping) return;
    setStopping(true);
    try {
      await api.disconnectSession(current.current);
      setState((old) => ({ ...old, closed: true, reason: "Container shell closed" }));
    } catch (reason) {
      setState((old) => ({ ...old, error: errorText(reason) }));
    } finally {
      setStopping(false);
    }
  };
  const live = connected && !state.closed && !state.pending;
  return <div className="docker-exec-panel">
    <div className="docker-exec-toolbar">
      <label>Shell<select aria-label="Container shell" value={shell} disabled={state.pending || stopping || live}
        onChange={(event) => setShell(event.target.value as "sh" | "bash")}><option value="sh">sh</option><option value="bash">bash</option></select></label>
      {live ? <button className="docker-button tone-warning" disabled={stopping} onClick={() => void end()}><Square size={13} />End session</button>
        : <button className="docker-button" disabled={state.pending || !connected || stopping} onClick={() => setAttempt((old) => old + 1)}><RefreshCw size={13} />Reconnect</button>}
    </div>
    {state.pending && connected && <p className="docker-muted"><LoaderCircle size={14} className="account-spinner" />Opening container shell</p>}
    {state.error && <p className="docker-error" role="alert">{state.error}</p>}
    {(!connected || !state.pending && state.closed) && <p className="docker-muted" role="status">{connected ? state.reason : "The container must be running to use Exec"}</p>}
    {state.id && <div className="terminal-frame docker-exec-terminal">
      <TerminalPane sessionId={state.id} active={active} focused={active && focused && live}
        connected={live} theme={theme} label={`${name} container shell`} />
    </div>}
  </div>;
}
