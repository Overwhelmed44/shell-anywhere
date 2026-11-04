import type { Host } from "./api";

export type LocalShell = "powershell" | "cmd" | "shell";

type SessionBase = {
  id: string;
  connected: boolean;
  reason?: string;
};
export type Session = SessionBase &
  (
    | { kind: "ssh"; host: Host }
    | { kind: "sftp"; host: Host }
    | { kind: "local"; shell: LocalShell; shellName?: string }
  );

export type SplitSide = "left" | "right" | "top" | "bottom";
export type SplitLayout = {
  firstId: string;
  secondId: string;
  direction: "horizontal" | "vertical";
  ratio: number;
};
export type SplitGroup =
  | (SplitLayout & { id: string; kind?: "terminal" })
  | {
      id: string;
      kind: "sftp";
      firstId: string;
      secondId?: string;
      direction: "horizontal";
      ratio: number;
      destinationName: string;
    };
export type AuthPrompt = {
  mode?: "ssh" | "sftp";
  host: Host;
  reconnectSessionId?: string;
  password: string;
  passphrase: string;
  fingerprint?: string;
  keyType?: string;
};
export type Theme = "dark" | "light";
