import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";
import { errorText } from "./utils";
import {
  historySample,
  serverRates,
  type ServerHistory,
  type ServerMetrics,
  type ServerRates
} from "./serverMetrics";

export type ServerMonitor = {
  data: ServerMetrics | null;
  rates: ServerRates | null;
  history: ServerHistory[];
  error: string;
  pending: boolean;
  refresh: () => void;
};

export function useServerMetrics(sessionId: string, active: boolean, connected: boolean, enabled: boolean): ServerMonitor {
  const [state, setState] = useState<Omit<ServerMonitor, "refresh">>({ data: null, rates: null, history: [], error: "", pending: false });
  const refreshCallback = useRef<() => void>(() => { });
  const inFlight = useRef(false);

  useEffect(() => {
    if (!active || !connected || !enabled) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let previous: ServerMetrics | null = null;
    let history: ServerHistory[] = [];
    let unsupported = false;
    const visible = () => !stopped && !document.hidden;
    const poll = async () => {
      clearTimeout(timer);
      if (!visible() || unsupported) return;
      if (inFlight.current) {
        timer = setTimeout(() => void poll(), 300);
        return;
      }
      inFlight.current = true;
      setState((old) => ({ ...old, pending: true }));
      try {
        const data = await api.getServerMetrics(sessionId);
        if (!visible()) return;
        const rates = serverRates(data, previous);
        history = [...history, historySample(data, rates)].slice(-60);
        previous = data;
        unsupported = !data.supported;
        setState({ data, rates, history, error: "", pending: false });
      } catch (reason) {
        if (!visible()) return;
        previous = null;
        history = [];
        setState((old) => ({ ...old, error: errorText(reason), rates: null, history: [], pending: false }));
      } finally {
        inFlight.current = false;
        if (visible() && !unsupported) timer = setTimeout(() => void poll(), 3000);
      }
    };
    const visibility = () => {
      clearTimeout(timer);
      previous = null;
      history = [];
      if (visible()) {
        setState((old) => ({ ...old, rates: null, history: [], pending: true }));
        void poll();
      }
    };
    refreshCallback.current = () => {
      unsupported = false;
      void poll();
    };
    document.addEventListener("visibilitychange", visibility);
    void Promise.resolve().then(() => {
      if (stopped) return;
      setState({ data: null, rates: null, history: [], error: "", pending: true });
      void poll();
    });
    return () => {
      stopped = true;
      clearTimeout(timer);
      refreshCallback.current = () => { };
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [sessionId, active, connected, enabled]);

  const refresh = useCallback(() => refreshCallback.current(), []);
  return { ...state, refresh };
}
