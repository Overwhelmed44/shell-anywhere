import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import type { DockerAction, DockerDetails, DockerLogs, DockerOverview } from "./docker";
import { errorText } from "./utils";

export function useDocker(sessionId: string, active: boolean, connected: boolean, selectedId: string, tab: "details" | "logs" | "exec", includeMetrics: boolean) {
  const [overview, setOverview] = useState<DockerOverview | null>(null);
  const [details, setDetails] = useState<DockerDetails | null>(null);
  const [logs, setLogs] = useState<(DockerLogs & { id: string }) | null>(null);
  const [error, setError] = useState("");
  const [detailError, setDetailError] = useState("");
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState("");
  const [pending, setPending] = useState(false);
  const [actionBusy, setActionBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  // Serialize polling, details and actions, including requests from an old effect.
  const queue = useRef<Promise<unknown>>(Promise.resolve());
  const commandRunning = useRef(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; };
  }, []);
  const enqueue = useCallback(<T,>(operation: () => Promise<T>): Promise<T> => {
    const next = queue.current.catch(() => {}).then(operation);
    queue.current = next;
    return next;
  }, []);

  useEffect(() => {
    if (!active || !connected) return;
    const refreshInterval = includeMetrics || (selectedId !== "" && tab === "logs") ? 5000 : 60000;
    let stopped = false;
    let polling = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const visible = () => !stopped && !document.hidden;
    const poll = async () => {
      clearTimeout(timer);
      if (!visible() || polling) return;
      if (commandRunning.current) {
        timer = setTimeout(() => void poll(), 300);
        return;
      }
      polling = true;
      let ready = false;
      try {
        await enqueue(async () => {
          if (!visible() || commandRunning.current) return;
          setPending(true);
          setDetailError("");
          const data = await api.getDockerOverview(sessionId, includeMetrics);
          if (!visible()) return;
          ready = data.status === "ready";
          setOverview(data);
          setError("");
          if (!ready || tab === "exec" || !selectedId || !data.containers.some((item) => item.id === selectedId)) return;
          try {
            if (tab === "logs") {
              const result = await api.getDockerLogs(sessionId, selectedId);
              if (visible()) setLogs({ ...result, id: selectedId });
            } else {
              const result = await api.getDockerDetails(sessionId, selectedId);
              if (visible()) setDetails(result);
            }
            if (visible()) setDetailError("");
          } catch (reason) {
            if (visible()) setDetailError(errorText(reason));
          }
        });
      } catch (reason) {
        ready = true; // Retry transient SSH/engine errors while the view is open.
        if (visible()) setError(errorText(reason));
      } finally {
        polling = false;
        if (!stopped) setPending(false);
        if (visible() && ready) timer = setTimeout(() => void poll(), refreshInterval);
      }
    };
    const visibility = () => {
      clearTimeout(timer);
      if (visible()) void poll();
    };
    document.addEventListener("visibilitychange", visibility);
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [sessionId, active, connected, selectedId, tab, includeMetrics, revision, enqueue]);

  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  const action = async (id: string, operation: DockerAction, name: string) => {
    if (!active || !connected) throw new Error("Reconnect and open Docker to manage this container");
    if (commandRunning.current) throw new Error("A Docker action is already running");
    commandRunning.current = true;
    setActionBusy(true);
    setActionError("");
    setNotice("");
    try {
      await enqueue(() => api.dockerContainerAction(sessionId, id, operation));
      if (alive.current) {
        const labels: Record<DockerAction, string> = { start: "Started", stop: "Stopped", restart: "Restarted", pause: "Paused", unpause: "Resumed", rm: "Removed" };
        setNotice(`${labels[operation]} ${name}`);
      }
    } catch (reason) {
      if (alive.current) setActionError(errorText(reason));
      throw reason;
    } finally {
      commandRunning.current = false;
      if (alive.current) {
        setActionBusy(false);
        refresh();
      }
    }
  };
  return { overview, details, logs, error, detailError, actionError, notice, pending, actionBusy, refresh, action };
}
