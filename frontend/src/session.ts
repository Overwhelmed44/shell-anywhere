import type { Session } from "./types";

export function sessionTitle(session: Session): string {
  if (session.kind === "sftp") return `${session.host.name} · SFTP`;
  if (session.kind === "ssh") return session.host.name;
  return (
    session.shellName ||
    (session.shell === "powershell"
      ? "PowerShell"
      : session.shell === "cmd"
        ? "Command Prompt"
        : "Shell")
  );
}

export function sessionSubtitle(session: Session): string {
  if (session.kind !== "local") {
    return `${session.host.username}@${session.host.address}:${session.host.port}`;
  }
  return "This computer";
}
