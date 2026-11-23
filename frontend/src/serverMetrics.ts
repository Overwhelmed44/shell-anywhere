export type ServerCPU = { cores: number; total: number; idle: number; ioWait: number };
export type ServerMemory = { total: number; available: number; cached: number; swapTotal: number; swapFree: number };
export type ServerFilesystem = { device: string; mount: string; total: number; used: number; available: number; usedPercent: number };
export type ServerNetwork = { name: string; received: number; sent: number; receiveErrors: number; sendErrors: number };
export type ServerDiskIO = { name: string; readOperations: number; readBytes: number; writeOperations: number; writeBytes: number; busyMilliseconds: number };
export type ServerMetrics = {
  supported: boolean;
  message: string;
  collectedAt: number;
  hostname: string;
  os: string;
  kernel: string;
  uptimeSeconds: number;
  loadAverage: number[];
  cpu: ServerCPU | null;
  memory: ServerMemory | null;
  storage: ServerFilesystem[];
  network: ServerNetwork[];
  primaryInterface: string;
  disks: ServerDiskIO[];
};

export const serverMetricSections = ["overview", "memory", "storage", "network", "io"] as const;
export type ServerMetricSection = typeof serverMetricSections[number];
export const serverExtensionIds = ["server-info"] as const;
export type ServerExtensionId = typeof serverExtensionIds[number];
export const defaultServerExtensions: ServerExtensionId[] = ["server-info"];

export function validServerExtensions(value: unknown): ServerExtensionId[] {
  if (!Array.isArray(value)) return [...defaultServerExtensions];
  // Consolidate previously pinned metric widgets into the single extension.
  // Preserve an explicit empty selection so monitoring stays disabled.
  return value.includes("server-info") || serverMetricSections.some((id) => value.includes(id)) ? ["server-info"] : [];
}

export type NetworkRate = { name: string; received: number | null; sent: number | null };
export type DiskRate = { name: string; read: number | null; write: number | null; readOps: number | null; writeOps: number | null; busy: number | null };
export type ServerRates = { cpu: number | null; ioWait: number | null; network: NetworkRate[]; disks: DiskRate[] };

export function percent(part: number, total: number): number {
  return total > 0 ? Math.max(0, Math.min(100, part / total * 100)) : 0;
}

export function serverRates(current: ServerMetrics, previous: ServerMetrics | null): ServerRates {
  const elapsed = previous ? current.uptimeSeconds - previous.uptimeSeconds : 0;
  const valid = elapsed > 0 && elapsed <= 30;
  const rate = (now: number, before: number | undefined) =>
    valid && before !== undefined && now >= before ? (now - before) / elapsed : null;
  let cpu: number | null = null;
  let ioWait: number | null = null;
  if (valid && current.cpu && previous?.cpu && current.cpu.cores === previous.cpu.cores) {
    const total = current.cpu.total - previous.cpu.total;
    const idle = current.cpu.idle - previous.cpu.idle;
    const wait = current.cpu.ioWait - previous.cpu.ioWait;
    if (total > 0 && idle >= 0 && idle <= total) cpu = percent(total - idle, total);
    if (total > 0 && wait >= 0) ioWait = percent(wait, total);
  }
  return {
    cpu, ioWait,
    network: current.network.map((item) => {
      const old = previous?.network.find((entry) => entry.name === item.name);
      return { name: item.name, received: rate(item.received, old?.received), sent: rate(item.sent, old?.sent) };
    }),
    disks: current.disks.map((item) => {
      const old = previous?.disks.find((entry) => entry.name === item.name);
      const busy = rate(item.busyMilliseconds, old?.busyMilliseconds);
      return {
        name: item.name, read: rate(item.readBytes, old?.readBytes), write: rate(item.writeBytes, old?.writeBytes),
        readOps: rate(item.readOperations, old?.readOperations), writeOps: rate(item.writeOperations, old?.writeOperations),
        busy: busy === null ? null : Math.min(100, busy / 10),
      };
    }),
  };
}

export function primaryNetwork(data: ServerMetrics | null, rates: ServerRates | null): NetworkRate | null {
  return rates?.network.find((item) => item.name === data?.primaryInterface) ?? rates?.network[0] ?? null;
}

export function busiestDisk(rates: ServerRates | null): DiskRate | null {
  return rates?.disks.reduce<DiskRate | null>((best, disk) =>
    !best || (disk.read ?? 0) + (disk.write ?? 0) > (best.read ?? 0) + (best.write ?? 0) ? disk : best, null) ?? null;
}

export function formatBytes(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return "—";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let amount = Math.max(0, value);
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) {
    amount /= 1024;
    unit++;
  }
  return `${amount.toFixed(unit === 0 || amount >= 100 ? 0 : amount >= 10 ? 1 : 2)} ${units[unit]}`;
}

export function formatRate(value: number | null | undefined): string {
  return value === null || value === undefined ? "—" : `${formatBytes(value)}/s`;
}

export function formatPercent(value: number | null | undefined): string {
  return value === null || value === undefined ? "—" : `${Math.round(value)}%`;
}

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor(seconds % 86400 / 3600);
  const minutes = Math.floor(seconds % 3600 / 60);
  return days ? `${days}d ${hours}h` : hours ? `${hours}h ${minutes}m` : `${minutes}m`;
}

export type ServerHistory = {
  at: number; cpu: number | null; memory: number | null;
  network: NetworkRate[]; disks: DiskRate[];
};

export function historySample(data: ServerMetrics, rates: ServerRates): ServerHistory {
  return {
    at: data.collectedAt, cpu: rates.cpu,
    memory: data.memory ? percent(data.memory.total - data.memory.available, data.memory.total) : null,
    network: rates.network, disks: rates.disks,
  };
}
