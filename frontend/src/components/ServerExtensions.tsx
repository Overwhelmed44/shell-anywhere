import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import {
  Activity, ArrowDown, ArrowUp, Check, ChevronDown, Cpu, HardDrive,
  LoaderCircle, MemoryStick, Network, Plus, RefreshCw, Server,
} from "lucide-react";
import {
  formatBytes, formatPercent, formatRate, formatUptime, percent,
  serverExtensionIds, type ServerExtensionId,
} from "../serverMetrics";
import { metricChartPath } from "../serverMetricChart";
import type { ServerMonitor } from "../useServerMetrics";
import "./ServerExtensions.css";

type ToolbarProps = {
  pinned: ServerExtensionId[];
  active: boolean;
  onToggle: (id: ServerExtensionId) => void;
};

export function ServerExtensionToolbar({ pinned, active, onToggle }: ToolbarProps) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const menuId = useId();
  const visible = open && active;

  useEffect(() => {
    if (!visible) return;
    menu.current?.querySelector<HTMLButtonElement>("button")?.focus();
    const outside = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        trigger.current?.focus();
      } else if (event.key === "Tab") {
        setOpen(false);
      } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        const items = Array.from(menu.current?.querySelectorAll<HTMLButtonElement>("button") ?? []);
        const current = items.indexOf(document.activeElement as HTMLButtonElement);
        items[(current + (event.key === "ArrowDown" ? 1 : -1) + items.length) % items.length]?.focus();
      }
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", key);
    };
  }, [visible]);

  return <div className="server-extension-toolbar" ref={container}>
    <button ref={trigger} type="button" className={`server-extension-trigger${visible ? " open" : ""}`}
      title="Session extensions" aria-label="Session extensions" aria-haspopup="menu"
      aria-expanded={visible} aria-controls={visible ? menuId : undefined}
      onClick={() => setOpen((old) => !old)}
      onKeyDown={(event) => {
        if (event.key === "ArrowDown") {
          event.preventDefault();
          setOpen(true);
        }
      }}>
      <Plus size={15} /><span>Extensions</span>
      {pinned.length > 0 && <span className="server-extension-count">{pinned.length}</span>}
      <ChevronDown size={12} />
    </button>
    {visible && <div ref={menu} id={menuId} className="server-extension-menu" role="menu" aria-label="Session extensions">
      <div className="server-extension-menu-title">Session extensions</div>
      {serverExtensionIds.map((id) => <button key={id} type="button" role="menuitemcheckbox"
        aria-checked={pinned.includes(id)} onClick={() => {
          onToggle(id);
          setOpen(false);
          trigger.current?.focus();
        }}>
        <Activity size={17} />
        <span>Server load<small>CPU, RAM, storage, network and disk I/O</small></span>
        {pinned.includes(id) && <Check size={15} className="server-extension-check" />}
      </button>)}
      <div className="server-extension-menu-note">Opens a view on this SSH connection</div>
    </div>}
  </div>;
}

function Meter({ value, label }: { value: number; label: string }) {
  return <div className={`server-meter${value >= 90 ? " high" : ""}`} role="meter" aria-label={label}
    aria-valuenow={Math.round(value)} aria-valuemin={0} aria-valuemax={100}>
    <span style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
  </div>;
}

type ChartSeries = { label: string; values: (number | null)[]; value: number | null | undefined };

function MetricChart({ times, series, label, maximum }: {
  times: number[]; series: ChartSeries[]; label: string; maximum?: number;
}) {
  const format = maximum === 100 ? formatPercent : formatRate;
  const values = series.flatMap((item) => item.values).filter((value): value is number => value !== null && Number.isFinite(value));
  const top = maximum ?? Math.max(1, ...values) * 1.15;
  const paths = series.map((item) => metricChartPath(times.map((at, index) => ({ at, value: item.values[index] ?? null })), top));
  const warmingUp = !paths.some((path) => path.includes("L"));

  return <div className="server-chart">
    <div className="server-chart-legend">
      {series.map((item, index) => <div className={`server-chart-reading series-${index}`} key={item.label}>
        <span><i />{item.label}</span><strong>{format(item.value)}</strong>
      </div>)}
    </div>
    <div className="server-chart-plot">
      <div className="server-chart-scale" aria-hidden="true">
        <span>{format(top)}</span><span>{format(top / 2)}</span><span>{format(0)}</span>
      </div>
      <div className="server-chart-canvas">
        <svg viewBox="-2 -2 484 104" role="img" aria-label={`${label}${warmingUp ? ", collecting live samples" : ", recent live samples"}`}
          preserveAspectRatio="none">
          <path d="M0 0H480 M0 50H480 M0 100H480" className="server-chart-grid" vectorEffect="non-scaling-stroke" />
          {paths.map((path, index) => <path key={series[index].label} d={path} className={`server-chart-line series-${index}`}
            fill="none" vectorEffect="non-scaling-stroke" />)}
        </svg>
        {warmingUp && <span className="server-chart-warmup">Collecting live samples</span>}
      </div>
    </div>
    <div className="server-chart-time" aria-hidden="true">
      <span>{times.length ? new Date(times[0]).toLocaleTimeString() : "Waiting for samples"}</span><span>Now</span>
    </div>
  </div>;
}

function Stat({ label, children, note }: { label: string; children: ReactNode; note?: string }) {
  return <div className="server-stat">
    <span>{label}</span><strong title={typeof children === "string" ? children : undefined}>{children}</strong>
    {note && <small title={note}>{note}</small>}
  </div>;
}

function DashboardCard({ title, icon, className = "", actions, children }: {
  title: string; icon: ReactNode; className?: string; actions?: ReactNode; children: ReactNode;
}) {
  const titleId = useId();
  return <section className={`server-dashboard-card ${className}`} aria-labelledby={titleId}>
    <div className="server-card-heading">{icon}<h3 id={titleId}>{title}</h3>{actions}</div>
    {children}
  </section>;
}

type DashboardProps = { monitor: ServerMonitor; connected: boolean };

export function ServerInfoDashboard({ monitor, connected }: DashboardProps) {
  const { data, rates, history, error, pending } = monitor;
  const [selectedInterface, setSelectedInterface] = useState("");
  const [selectedDisk, setSelectedDisk] = useState("");
  const live = connected && data?.supported && !error;
  let content: ReactNode;

  if (!connected) {
    content = <div className="server-panel-message">Reconnect to resume server monitoring</div>;
  } else if (error) {
    content = <div className="server-panel-message server-panel-error" role="alert">
      {error}<button className="button secondary" type="button" onClick={monitor.refresh} disabled={pending}>Try again</button>
    </div>;
  } else if (!data) {
    content = <div className="server-panel-message"><LoaderCircle size={18} className="account-spinner" />Reading server information</div>;
  } else if (!data.supported) {
    content = <div className="server-panel-message">{data.message}</div>;
  } else {
    const memory = data.memory;
    const times = history.map((sample) => sample.at);
    const networkName = data.network.find((item) => item.name === selectedInterface)?.name
      ?? data.network.find((item) => item.name === data.primaryInterface)?.name ?? data.network[0]?.name ?? "";
    const network = rates?.network.find((item) => item.name === networkName);
    const counters = data.network.find((item) => item.name === networkName);
    const diskName = data.disks.find((item) => item.name === selectedDisk)?.name ?? data.disks[0]?.name ?? "";
    const disk = rates?.disks.find((item) => item.name === diskName);

    content = <div className="server-dashboard-grid">
      <DashboardCard title="CPU" icon={<Cpu size={15} />} className="server-card-cpu"
        actions={<span className="server-card-detail">{data.cpu ? `${data.cpu.cores} logical cores` : "Unavailable"}</span>}>
        <MetricChart times={times} maximum={100} label="CPU usage" series={[
          { label: "Usage", value: rates?.cpu, values: history.map((sample) => sample.cpu) },
        ]} />
        <div className="server-card-stats"><Stat label="I/O wait">{formatPercent(rates?.ioWait)}</Stat>
          <Stat label="Load · 1 / 5 / 15 min">{data.loadAverage.map((load) => load.toFixed(2)).join(" / ") || "-"}</Stat></div>
      </DashboardCard>

      <DashboardCard title="Memory" icon={<MemoryStick size={15} />} className="server-card-memory"
        actions={<span className="server-card-detail">{formatBytes(memory?.total)} RAM</span>}>
        {memory ? <>
          <MetricChart times={times} maximum={100} label="RAM usage" series={[
            { label: `${formatBytes(memory.total - memory.available)} used`, value: percent(memory.total - memory.available, memory.total), values: history.map((sample) => sample.memory) },
          ]} />
          <div className="server-card-stats"><Stat label="Available">{formatBytes(memory.available)}</Stat>
            <Stat label="Cached">{formatBytes(memory.cached)}</Stat>
            <Stat label="Swap used" note={memory.swapTotal ? `of ${formatBytes(memory.swapTotal)}` : "No swap configured"}>
              {formatBytes(memory.swapTotal - memory.swapFree)}
            </Stat></div>
        </> : <div className="server-panel-message">Memory information is not reported by this server</div>}
      </DashboardCard>

      <DashboardCard title="System" icon={<Server size={15} />} className="server-card-system">
        <div className="server-system-identity"><strong title={data.hostname}>{data.hostname || "Linux server"}</strong>
          <span title={data.os}>{data.os || "Linux"}</span></div>
        <div className="server-system-stats"><Stat label="Uptime">{formatUptime(data.uptimeSeconds)}</Stat>
          <Stat label="Kernel">{data.kernel || "-"}</Stat>
          <Stat label="Interfaces">{data.network.length}</Stat>
          <Stat label="Filesystems">{data.storage.length}</Stat></div>
      </DashboardCard>

      <DashboardCard title="Network" icon={<Network size={15} />} className="server-card-network"
        actions={data.network.length > 0 && <select aria-label="Network chart interface" value={networkName} onChange={(event) => setSelectedInterface(event.target.value)}>
          {data.network.map((item) => <option key={item.name} value={item.name}>{item.name}{item.name === data.primaryInterface ? " · Default route" : ""}</option>)}
        </select>}>
        {data.network.length ? <>
          <MetricChart times={times} label={`${networkName} network throughput`} series={[
            { label: "Receive", value: network?.received, values: history.map((sample) => sample.network.find((item) => item.name === networkName)?.received ?? null) },
            { label: "Send", value: network?.sent, values: history.map((sample) => sample.network.find((item) => item.name === networkName)?.sent ?? null) },
          ]} />
          <div className="server-table-wrap"><table className="server-metrics-table">
            <thead><tr><th>Interface</th><th>Receive</th><th>Send</th><th>RX / TX errors</th></tr></thead>
            <tbody>{data.network.map((item) => {
              const rate = rates?.network.find((entry) => entry.name === item.name);
              return <tr key={item.name} className={item.name === networkName ? "selected" : ""}>
                <td><span className="server-device-name">{item.name}{item.name === data.primaryInterface && <small>Default</small>}</span></td>
                <td><span className="server-rate"><ArrowDown size={12} />{formatRate(rate?.received)}</span></td>
                <td><span className="server-rate"><ArrowUp size={12} />{formatRate(rate?.sent)}</span></td>
                <td>{item.receiveErrors} / {item.sendErrors}</td>
              </tr>;
            })}</tbody>
          </table></div>
          {counters && <div className="server-panel-footnote">{networkName} since boot: {formatBytes(counters.received)} received · {formatBytes(counters.sent)} sent</div>}
        </> : <div className="server-panel-message">Network counters are not reported by this server</div>}
      </DashboardCard>

      <DashboardCard title="Disk I/O" icon={<Activity size={15} />} className="server-card-io"
        actions={data.disks.length > 0 && <select aria-label="Disk I/O chart device" value={diskName} onChange={(event) => setSelectedDisk(event.target.value)}>
          {data.disks.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
        </select>}>
        {data.disks.length ? <>
          <MetricChart times={times} label={`${diskName} disk throughput`} series={[
            { label: "Read", value: disk?.read, values: history.map((sample) => sample.disks.find((item) => item.name === diskName)?.read ?? null) },
            { label: "Write", value: disk?.write, values: history.map((sample) => sample.disks.find((item) => item.name === diskName)?.write ?? null) },
          ]} />
          <div className="server-table-wrap"><table className="server-metrics-table">
            <thead><tr><th>Device</th><th>Read</th><th>Write</th><th>IOPS</th><th>Busy</th></tr></thead>
            <tbody>{data.disks.map((item) => {
              const rate = rates?.disks.find((entry) => entry.name === item.name);
              const operations = rate && rate.readOps !== null && rate.writeOps !== null ? rate.readOps + rate.writeOps : null;
              return <tr key={item.name} className={item.name === diskName ? "selected" : ""}>
                <td>{item.name}</td><td>{formatRate(rate?.read)}</td><td>{formatRate(rate?.write)}</td>
                <td>{operations === null ? "-" : Math.round(operations)}</td><td>{formatPercent(rate?.busy)}</td>
              </tr>;
            })}</tbody>
          </table></div>
          <div className="server-panel-footnote">Whole-device activity · Select a device to chart its throughput</div>
        </> : <div className="server-panel-message">Disk I/O counters are not reported by this server</div>}
      </DashboardCard>

      <DashboardCard title="Storage" icon={<HardDrive size={15} />} className="server-card-storage"
        actions={<span className="server-card-detail">Filesystem capacity</span>}>
        {data.storage.length ? <div className="server-filesystems">
          {data.storage.map((item) => <div className="server-filesystem" key={`${item.device}:${item.mount}`}>
            <div className="server-filesystem-name"><HardDrive size={16} /><strong title={item.mount}>{item.mount}</strong>
              <span title={item.device}>{item.device}</span><b>{formatPercent(item.usedPercent)}</b></div>
            <Meter value={item.usedPercent} label={`${item.mount} storage used`} />
            <div className="server-filesystem-capacity"><span>{formatBytes(item.used)} used of {formatBytes(item.total)}</span>
              <strong>{formatBytes(item.available)} free</strong></div>
          </div>)}
        </div> : <div className="server-panel-message">Filesystem capacity is not reported by this server</div>}
      </DashboardCard>
    </div>;
  }

  return <section className="server-info-dashboard" aria-label="Server load dashboard">
    <div className="server-dashboard-heading"><div><Activity size={16} /><strong>Server load</strong></div>
      <span className={`server-panel-status${live ? " live" : ""}`}
        title={data ? `Last updated ${new Date(data.collectedAt).toLocaleTimeString()}` : undefined}>
        {!connected ? "Paused" : error ? "Unavailable" : !data ? "Connecting" : data.supported ? "Live · 3s" : "Not supported"}
      </span>
      <button type="button" className="icon-button" title="Refresh server information" aria-label="Refresh server information"
        disabled={!connected || pending} onClick={monitor.refresh}>
        <RefreshCw size={14} className={pending ? "account-spinner" : ""} />
      </button>
    </div>
    <div className="server-dashboard-body">{content}</div>
  </section>;
}
