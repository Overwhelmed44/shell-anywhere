import {
  RefreshCw,
  Settings2,
  FolderOpen,
  TerminalSquare,
  Wifi,
  WifiOff,
} from "lucide-react";
import { useId, useState } from "react";
import FileBrowser from "./FileBrowser";
import TerminalPane from "../TerminalPane";
import type { Host } from "../api";
import type { Session, Theme } from "../types";
import { sessionSubtitle, sessionTitle } from "../session";
import { ServerInfoDashboard } from "./ServerExtensions";
import SSHWorkspace, { type SSHView } from "./SSHWorkspace";
import { serverExtensionIds, type ServerExtensionId } from "../serverMetrics";
import { useServerMetrics } from "../useServerMetrics";
import DockerExtension from "./DockerExtension";

type Props = {
  session: Session;
  hosts: Host[];
  active: boolean;
  focused: boolean;
  placement: "single" | "first" | "second" | "hidden";
  theme: Theme;
  reconnecting: boolean;
  onEdit: (host: Host) => void;
  onFocus: (id: string) => void;
  onReconnect: (session: Session) => void;
  fileRatio: number;
  onFileResize: (ratio: number) => void;
  onFileDestinationChange: (
    sessionId: string,
    name: string,
    destinationId: string,
  ) => void;
};

export default function SessionView({
  session,
  hosts,
  active,
  focused,
  placement,
  theme,
  reconnecting,
  onEdit,
  onFocus,
  onReconnect,
  fileRatio,
  onFileResize,
  onFileDestinationChange,
}: Props) {
  const extensions = [...serverExtensionIds];
  const [view, setView] = useState<SSHView>("terminal");
  const [selectedExtension, setSelectedExtension] = useState<ServerExtensionId>("server-info");
  const selected = extensions.includes(selectedExtension) ? selectedExtension : extensions[0] ?? "server-info";
  const panelId = useId();
  const monitor = useServerMetrics(session.id, active, session.connected, session.kind === "ssh" && extensions.includes("server-info"));
  const renderTerminal = ({ active: terminalActive, focused: terminalFocused }: { active: boolean; focused: boolean }) => (
    <div className="terminal-frame">
      <div className="terminal-toolbar">
        <div><span className="terminal-live-dot" /> {session.connected ? "Live terminal" : session.reason || "Session ended"}</div>
        <span>{session.kind === "ssh" ? `${session.host.username}@${session.host.address}` : sessionTitle(session)}</span>
      </div>
      <TerminalPane sessionId={session.id} active={terminalActive} focused={terminalFocused}
        connected={session.connected} theme={theme} label={`${sessionTitle(session)} terminal`} />
    </div>
  );
  return (
    <div
      className={`session-view pane-${placement}${focused ? " focused" : ""}${session.kind === "ssh" ? " has-server-extensions" : ""}`}
      onPointerDown={() => onFocus(session.id)}
    >
      <div className="session-header">
        <div className="session-host-icon">
          {session.kind === "sftp" ? (
            <FolderOpen size={20} />
          ) : (
            <TerminalSquare size={20} />
          )}
        </div>
        <div>
          <h2>
            {sessionTitle(session)}
          </h2>
          <p>
            {sessionSubtitle(session)}
          </p>
        </div>
        <div
          className={
            session.connected ? "session-state online" : "session-state"
          }
        >
          {session.connected ? <Wifi size={14} /> : <WifiOff size={14} />}
          <span>
            {reconnecting
              ? "Reconnecting"
              : session.connected
                ? "Connected"
                : "Disconnected"}
          </span>
        </div>
        {session.kind !== "local" && !session.connected && (
          <button
            className="button secondary session-reconnect"
            type="button"
            disabled={reconnecting}
            onClick={() => onReconnect(session)}
          >
            <RefreshCw size={15} />
            {reconnecting ? "Connecting…" : "Reconnect"}
          </button>
        )}
        {session.kind !== "local" && (
          <button
            className="icon-button"
            title="Edit host"
            onClick={() => onEdit(session.host)}
          >
            <Settings2 size={18} />
          </button>
        )}
      </div>
      {session.kind === "sftp" ? (
        <FileBrowser
          sessionId={session.id}
          connected={session.connected}
          active={active}
          hosts={hosts}
          host={session.host}
          ratio={fileRatio}
          onResize={onFileResize}
          onDestinationChange={(name, id) =>
            onFileDestinationChange(session.id, name, id)
          }
        />
      ) : session.kind === "ssh" ? (
        <SSHWorkspace id={panelId} view={view} active={active} focused={focused} onViewChange={setView}
          extensions={extensions} selectedExtension={selected} onExtensionChange={setSelectedExtension}
          terminal={renderTerminal}
          extension={extensions.length ? <>
            {extensions.includes("server-info") && <div className="ssh-extension-content" hidden={selected !== "server-info"}>
              <ServerInfoDashboard monitor={monitor} connected={session.connected} />
            </div>}
            {extensions.includes("docker") && <div className="ssh-extension-content" hidden={selected !== "docker"}>
              <DockerExtension sessionId={session.id} connected={session.connected} theme={theme} focused={focused}
                active={active && view !== "terminal" && selected === "docker"} />
            </div>}
          </> : null} />
      ) : renderTerminal({ active, focused })}
    </div>
  );
}
