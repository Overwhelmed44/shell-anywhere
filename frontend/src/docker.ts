export type DockerAction = "start" | "stop" | "restart" | "pause" | "unpause" | "rm";
export type DockerContainer = {
  id: string; name: string; image: string; state: string; status: string; ports: string;
  project: string; service: string; cpu: string; memory: string; memoryPercent: string; netIO: string; blockIO: string;
};
export type DockerOverview = {
  status: "ready" | "missing" | "unavailable"; message: string; warning: string;
  version: string; name: string; containers: DockerContainer[];
};
export type DockerDetails = {
  id: string; created: string; started: string; finished: string; exitCode: number;
  restartCount: number; restartPolicy: string;
  mounts: { Type: string; Source: string; Destination: string; RW: boolean }[];
  networks: Record<string, { IPAddress: string; Gateway: string }>;
};
export type DockerLogs = { text: string; truncated: boolean };
