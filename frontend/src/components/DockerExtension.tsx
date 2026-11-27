import { useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Activity, ArrowDown, ArrowLeft, ArrowUp, ArrowUpDown, Box, Container, FileText, Info, ListFilter, LoaderCircle, Pause, Play, RefreshCw, RotateCw, Search, Square, TerminalSquare, Trash2 } from "lucide-react";
import type { DockerAction, DockerContainer, DockerDetails } from "../docker";
import { useDocker } from "../useDocker";
import ConfirmDialog, { type Confirmation } from "./ConfirmDialog";
import DockerExecPane from "./DockerExecPane";
import Dropdown from "./Dropdown";
import type { Theme } from "../types";
import "./DockerExtension.css";

type Props = { sessionId: string; active: boolean; focused: boolean; connected: boolean; theme: Theme };
const date = (value: string) => !value || value.startsWith("0001-") ? "—" : new Date(value).toLocaleString();
const stateOrder = ["running", "restarting", "paused", "created", "exited", "dead", "removing"];
const stateRank = (state: string) => {
  const index = stateOrder.indexOf(state);
  return index < 0 ? stateOrder.length : index;
};

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return <div className="docker-detail"><dt>{label}</dt><dd>{children === "" || children == null ? "—" : children}</dd></div>;
}

function ContainerDetails({ container, details }: { container: DockerContainer; details: DockerDetails | null }) {
  return <div className="docker-details-content" role="region" tabIndex={0} aria-label={`Details for ${container.name}`}>
    <dl className="docker-detail-grid">
      <Detail label="Container ID"><code>{container.id}</code></Detail>
      <Detail label="Status">{container.status}</Detail>
      <Detail label="Ports">{container.ports || "No exposed or published ports"}</Detail>
      {details && <>
        <Detail label="Created">{date(details.created)}</Detail>
        <Detail label="Started">{date(details.started)}</Detail>
        {container.state !== "running" && container.state !== "paused" && <>
          <Detail label="Finished">{date(details.finished)}</Detail>
          <Detail label="Exit code">{details.exitCode}</Detail>
        </>}
        <Detail label="Restart policy">{details.restartPolicy || "no"}</Detail>
        <Detail label="Restarts">{details.restartCount}</Detail>
      </>}
    </dl>
    {details && <>
      <h4>Mounts <span>{details.mounts.length}</span></h4>
      {details.mounts.length ? details.mounts.map((mount) => <div className="docker-mount" key={`${mount.Source}:${mount.Destination}`}>
        <strong>{mount.Destination}</strong><code>{mount.Source}</code><small>{mount.Type} · {mount.RW ? "Read / write" : "Read only"}</small>
      </div>) : <p className="docker-muted">No mounts</p>}
      <h4>Networks <span>{Object.keys(details.networks).length}</span></h4>
      {Object.entries(details.networks).map(([name, network]) => <div className="docker-network" key={name}>
        <strong>{name}</strong><span>{network.IPAddress || "No assigned IPv4 address"}</span>
        {network.Gateway && <small>Gateway {network.Gateway}</small>}
      </div>)}
    </>}
  </div>;
}

export default function DockerExtension({ sessionId, active, focused, connected, theme }: Props) {
  const [selectedId, setSelectedId] = useState("");
  const [containerOpen, setContainerOpen] = useState(false);
  const returnFocus = useRef<HTMLButtonElement | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState("all");
  const [sort, setSort] = useState<{ key: "state" | "name"; descending: boolean }>({ key: "state", descending: false });
  const [tab, setTab] = useState<"details" | "logs" | "exec">("details");
  const [showMetrics, setShowMetrics] = useState(false);
  const [memoryPercent, setMemoryPercent] = useState(false);
  const memoryToggleLabel = memoryPercent ? "Show absolute memory usage" : "Show memory usage as a percentage";
  const [execContainerId, setExecContainerId] = useState("");
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const docker = useDocker(sessionId, active, connected, showMetrics || !containerOpen ? "" : selectedId, tab, showMetrics);
  const { overview, pending, actionBusy } = docker;
  const containers = overview?.containers ?? [];
  const selected = containers.find((item) => item.id === selectedId);
  const showContainer = Boolean(selected && containerOpen && !showMetrics);
  const running = containers.filter((item) => item.state === "running").length;
  const paused = containers.filter((item) => item.state === "paused").length;
  const stopped = containers.filter((item) => item.state === "exited" || item.state === "created" || item.state === "dead").length;
  const ready = connected && overview?.status === "ready" && !docker.error;
  const filtered = containers.filter((item) => (filter === "all" || (filter === "stopped"
    ? ["exited", "created", "dead"].includes(item.state) : item.state === filter))
    && [item.name, item.id].some((value) => value.toLowerCase().includes(query.toLowerCase())))
    .sort((a, b) => {
      const comparison = sort.key === "state" ? stateRank(a.state) - stateRank(b.state) : a.name.localeCompare(b.name);
      return comparison * (sort.descending ? -1 : 1) || a.name.localeCompare(b.name) || a.id.localeCompare(b.id);
    });
  const sortColumn = (key: "state" | "name") => setSort((old) => ({ key, descending: old.key === key && !old.descending }));
  const sortIcon = (key: "state" | "name") => sort.key !== key ? <ArrowUpDown size={12} />
    : sort.descending ? <ArrowDown size={12} /> : <ArrowUp size={12} />;
  const open = (container: DockerContainer, nextTab: "details" | "logs" | "exec", origin?: HTMLButtonElement) => {
    if (origin) returnFocus.current = origin;
    setSelectedId(container.id);
    setContainerOpen(true);
    setTab(nextTab);
    if (nextTab === "exec") setExecContainerId(container.id);
  };
  const back = () => {
    setContainerOpen(false);
    requestAnimationFrame(() => returnFocus.current?.focus());
  };
  const act = (container: DockerContainer, operation: DockerAction) => {
    void docker.action(container.id, operation, container.name).catch(() => {});
  };
  const remove = (container: DockerContainer) => setConfirmation({
    title: `Remove ${container.name}?`,
    description: "This removes the stopped container and its writable layer. Named volumes, bind mounts and the image are kept",
    confirmLabel: "Remove container", busyLabel: "Removing", danger: true,
    onConfirm: () => docker.action(container.id, "rm", container.name),
  });
  const controls = (container: DockerContainer) => {
    const isRunning = container.state === "running";
    const isPaused = container.state === "paused";
    const isStopped = ["exited", "created", "dead"].includes(container.state);
    const disabled = !ready || actionBusy || pending;
    return <div className="docker-actions" role="group" aria-label={`Controls for ${container.name}`}>
      {isStopped && container.state !== "dead" && <button className="docker-button tone-success" disabled={disabled} onClick={() => act(container, "start")}><Play size={13} />Start</button>}
      {(isRunning || container.state === "restarting") && <>
        <button className="docker-button tone-warning" disabled={disabled} onClick={() => act(container, "stop")}><Square size={13} />Stop</button>
        <button className="docker-button" disabled={disabled} onClick={() => act(container, "restart")}><RotateCw size={13} />Restart</button>
      </>}
      {isRunning && <button className="docker-button" disabled={disabled} onClick={() => act(container, "pause")}><Pause size={13} />Pause</button>}
      {isPaused && <button className="docker-button tone-success" disabled={disabled} onClick={() => act(container, "unpause")}><Play size={13} />Resume</button>}
      {isStopped && <button className="docker-button tone-danger" disabled={disabled} onClick={() => remove(container)}><Trash2 size={13} />Remove</button>}
    </div>;
  };

  return <section className={`docker-extension${showContainer ? " is-container-view" : ""}`} aria-label="Docker extension" aria-busy={pending || actionBusy}>
    <div className="docker-heading">
      {showContainer ? <button type="button" className="button secondary docker-back" onClick={back}
        aria-label="Back to containers"><ArrowLeft size={15} />Back</button>
        : <div className="docker-heading-icon"><Container size={22} /></div>}
      <div className="docker-heading-copy"><h3 title={showContainer ? selected?.name : undefined}>{showContainer ? selected?.name : "Docker"}</h3>
        <p>{showContainer ? `Docker / ${tab === "exec" ? "Exec" : tab === "logs" ? "Logs" : "Details"}`
          : overview?.status === "ready" ? `${overview.name || "Docker Engine"} · v${overview.version}` : "Container management on this server"}</p></div>
      <div className="docker-view-switch" role="group" aria-label="Docker view" hidden={showContainer}>
        <button type="button" aria-pressed={!showMetrics} onClick={() => setShowMetrics(false)}><Container size={14} />Containers</button>
        <button type="button" aria-pressed={showMetrics} onClick={() => setShowMetrics(true)}><Activity size={14} />Metrics</button>
      </div>
      <button type="button" className="icon-button" aria-label="Refresh Docker" title="Refresh Docker"
        disabled={!connected || pending || actionBusy} onClick={docker.refresh}>
        <RefreshCw size={16} className={pending ? "account-spinner" : ""} />
      </button>
    </div>
    {!connected ? <div className="docker-empty"><Container size={28} /><h4>Connection closed</h4><p>Reconnect to manage Docker on this server</p></div>
      : !overview && !docker.error ? <div className="docker-empty"><LoaderCircle size={24} className="account-spinner" /><h4>Checking Docker</h4><p>Connecting to the engine through your SSH session</p></div>
        : overview?.status === "missing" ? <div className="docker-empty"><Box size={30} /><h4>Docker is not installed</h4>
          <p>Install Docker on the server to use this extension, then check again</p><small>Docker must be available in the SSH user’s PATH</small>
          <button className="button secondary" disabled={pending} onClick={docker.refresh}>Check again</button></div>
          : overview?.status === "unavailable" ? <div className="docker-empty"><Container size={30} /><h4>Docker is unavailable</h4>
            <p>Make sure the Docker engine is running and your SSH user has permission to access it</p>
            {overview.warning && <pre className="docker-engine-error">{overview.warning}</pre>}
            <button className="button secondary" disabled={pending} onClick={docker.refresh}>Check again</button></div>
            : <>
              {docker.error && <div className="docker-error" role="alert">{docker.error}<button className="button secondary" disabled={pending} onClick={docker.refresh}>Try again</button></div>}
              {overview?.status === "ready" && <>
                <div className="docker-summary" hidden={showContainer}>
                  {[{ label: "Containers", value: containers.length }, { label: "Running", value: running }, { label: "Stopped", value: stopped }, { label: "Paused", value: paused }].map((item) =>
                    <div key={item.label}><span>{item.label}</span><strong>{item.value}</strong></div>)}
                </div>
                {overview.warning && <p className="docker-warning" role="status">{overview.warning}</p>}
                {actionBusy && <p className="docker-notice" role="status"><LoaderCircle size={14} className="account-spinner" />Applying container action</p>}
                {docker.actionError && <p className="docker-error" role="alert">{docker.actionError}</p>}
                {docker.notice && <p className="docker-notice" role="status">{docker.notice}</p>}
                <div className="docker-filterbar" hidden={showContainer}>
                  <label className="docker-search"><Search size={14} /><input aria-label="Search containers" placeholder="Search containers" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
                  <Dropdown className="docker-state-filter" label="Filter containers by state" value={filter} onChange={setFilter}
                    icon={<ListFilter size={14} />} options={[
                      { value: "all", label: "All states" }, { value: "running", label: "Running" },
                      { value: "stopped", label: "Stopped" }, { value: "paused", label: "Paused" },
                      { value: "restarting", label: "Restarting" },
                    ]} />
                </div>
                <div className={`docker-workspace${showContainer ? " container-open" : ""}`}>
                  <div className="docker-container-list" hidden={showContainer}>
                    {filtered.length ? <div className="docker-table-wrap"><table className="docker-table">
                      <thead><tr><th aria-sort={sort.key === "name" ? sort.descending ? "descending" : "ascending" : "none"}>
                        <button type="button" className="docker-sort" onClick={() => sortColumn("name")}>Container {sortIcon("name")}</button></th>
                        <th aria-sort={sort.key === "state" ? sort.descending ? "descending" : "ascending" : "none"}>
                          <button type="button" className="docker-sort" onClick={() => sortColumn("state")}
                            title={sort.key === "state" && !sort.descending ? "Running containers first · Click to reverse" : "Sort by state, running first"}>State {sortIcon("state")}</button></th>{showMetrics
                        ? <><th>CPU</th><th><button type="button" className="docker-memory-toggle" aria-pressed={memoryPercent}
                          title={memoryToggleLabel} onClick={() => setMemoryPercent((old) => !old)}>Memory <small>{memoryPercent ? "%" : "MB"}</small></button></th><th>Network I/O</th><th>Disk I/O</th></>
                        : <th>Actions</th>}</tr></thead>
                      <tbody>{filtered.map((container) => <tr key={container.id} className={!showMetrics && selectedId === container.id ? "selected" : ""}>
                        <td><button className="docker-select" aria-pressed={!showMetrics && selectedId === container.id}
                          disabled={showMetrics} onClick={(event) => open(container, "details", event.currentTarget)}>
                          <strong title={container.name}>{container.name}</strong>
                        </button></td>
                        <td><span className={`docker-state state-${container.state}`}>{container.state}</span><small className="docker-status" title={container.status}>{container.status}</small></td>
                        {showMetrics ? <><td>{pending ? container.cpu || "Reading" : container.cpu || "-"}</td><td><button type="button"
                          className="docker-memory-toggle docker-memory-value" title={memoryToggleLabel}
                          aria-label={`${memoryToggleLabel} for ${container.name}`} onClick={() => setMemoryPercent((old) => !old)}>
                          {(memoryPercent ? container.memoryPercent : container.memory) || "—"}</button></td>
                          <td>{container.netIO || "—"}</td><td>{container.blockIO || "—"}</td></>
                          : <td><div className="docker-row-actions"><button type="button" className="docker-button docker-row-button" disabled={!ready}
                            aria-label={`Logs for ${container.name}`} onClick={(event) => open(container, "logs", event.currentTarget)}><FileText size={13} />Logs</button>
                          <button type="button" className="docker-button docker-row-button tone-accent" disabled={!ready || container.state !== "running"}
                            title={container.state === "running" ? "Open container shell" : "Start the container to use Exec"}
                            aria-label={`Exec in ${container.name}`} onClick={(event) => open(container, "exec", event.currentTarget)}><TerminalSquare size={13} />Exec</button></div></td>}
                      </tr>)}</tbody>
                    </table></div> : <div className="docker-empty docker-no-containers"><Box size={26} /><h4>{containers.length ? "No matching containers" : "No containers yet"}</h4><p>{containers.length ? "Try another search or state filter" : "Containers created on the server will appear here"}</p></div>}
                  </div>
                  {selected && <section className="docker-inspector" hidden={!showContainer} aria-label={`${selected.name} details`}>
                    <div className="docker-inspector-heading"><span className={`docker-state state-${selected.state}`}>{selected.state}</span>{controls(selected)}</div>
                    <div className="docker-detail-tabs" role="group" aria-label="Container views">
                      <button aria-pressed={tab === "details"} onClick={() => setTab("details")}><Info size={14} />Details</button>
                      <button aria-pressed={tab === "logs"} onClick={() => setTab("logs")}><FileText size={14} />Logs</button>
                      <button aria-pressed={tab === "exec"} disabled={selected.state !== "running" && execContainerId !== selected.id}
                        onClick={() => open(selected, "exec")}><TerminalSquare size={14} />Exec</button>
                    </div>
                    {tab !== "exec" && docker.detailError && <p className="docker-error" role="alert">{docker.detailError}</p>}
                    {tab === "details" && <ContainerDetails container={selected} details={docker.details?.id === selected.id ? docker.details : null} />}
                    {tab === "logs" && <div className="docker-log-panel"><div className="docker-log-caption">Last 200 lines · stdout + stderr</div>
                        {docker.logs?.id === selected.id ? <>
                          {docker.logs.truncated && <p className="docker-warning">Large output trimmed to the latest 256 KiB</p>}
                          <div className="docker-log-scroll" role="log" aria-live="off" tabIndex={0} aria-label={`Logs for ${selected.name}`}>
                            <pre className="docker-logs">{docker.logs.text || "No logs available"}</pre>
                          </div>
                        </> : !docker.detailError && <p className="docker-muted">Reading container logs</p>}
                      </div>}
                    {execContainerId === selected.id && <div className="docker-exec-view" hidden={tab !== "exec"}>
                      <DockerExecPane key={selected.id} sessionId={sessionId} containerId={selected.id} name={selected.name}
                        connected={connected && selected.state === "running"} theme={theme} focused={focused}
                        active={active && showContainer && tab === "exec"} />
                    </div>}
                  </section>}
                </div>
                <div className="docker-footer" hidden={showContainer}><span>{showMetrics ? "Metrics refresh every 5 seconds" : "Containers refresh every minute"}</span><span>Using your SSH user's Docker access</span></div>
              </>}
            </>}
    {confirmation && active && connected && createPortal(<ConfirmDialog {...confirmation} onClose={() => setConfirmation(null)} />, document.body)}
  </section>;
}
