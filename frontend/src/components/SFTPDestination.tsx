import { useEffect, useRef, useState } from "react";
import { HardDrive, LoaderCircle, RefreshCw, Server } from "lucide-react";
import { api, onEvent, type Host } from "../api";
import type { AuthPrompt } from "../types";
import { errorText } from "../utils";
import ConnectDialog from "./ConnectDialog";
import Dropdown from "./Dropdown";

export type FileDestination = {
  id: string;
  host: Host;
  connected: boolean;
};

type Props = {
  hosts: Host[];
  destination: FileDestination | null;
  chosen: boolean;
  disabled: boolean;
  onChange: (destination: FileDestination | null) => void;
  onBusyChange: (busy: boolean) => void;
};

export default function SFTPDestination({
  hosts,
  destination,
  chosen,
  disabled,
  onChange,
  onBusyChange,
}: Props) {
  const [prompt, setPrompt] = useState<AuthPrompt | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [connectingHost, setConnectingHost] = useState<Host | null>(null);
  const connection = useRef<FileDestination | null>(null);
  const attempt = useRef(0);
  const inFlight = useRef(false);
  const mounted = useRef(true);
  const closed = useRef(new Set<string>());
  const callbacks = useRef({ onChange, onBusyChange });

  useEffect(() => {
    callbacks.current = { onChange, onBusyChange };
  }, [onChange, onBusyChange]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      // Invalidate any connection attempt started after mounting
      // eslint-disable-next-line react-hooks/exhaustive-deps
      attempt.current++;
      // Close the latest connection, including any host chosen after mounting
      // eslint-disable-next-line react-hooks/exhaustive-deps
      const current = connection.current;
      connection.current = null;
      if (current) void api.disconnectSession(current.id).catch(() => {});
    };
  }, []);

  useEffect(
    () =>
      onEvent("terminal:closed", (data) => {
        const event = data as { sessionId: string; reason: string };
        if (inFlight.current) closed.current.add(event.sessionId);
        const current = connection.current;
        if (current?.id !== event.sessionId) return;
        const next = { ...current, connected: false };
        connection.current = next;
        callbacks.current.onChange(next);
        setError(event.reason || "SFTP connection closed");
      }),
    [],
  );

  const cancel = () => {
    attempt.current++;
    setPrompt(null);
    setConnectingHost(null);
    setError("");
    callbacks.current.onBusyChange(false);
  };

  const connect = async (request: AuthPrompt) => {
    if (inFlight.current) return;
    const ticket = ++attempt.current;
    inFlight.current = true;
    closed.current.clear();
    setBusy(true);
    setConnectingHost(request.host);
    setError("");
    callbacks.current.onBusyChange(true);

    try {
      const result = request.reconnectSessionId
        ? await api.reconnectSFTP(
            request.reconnectSessionId,
            request.host.workspaceId,
            request.host.id,
            request.password,
            request.passphrase,
            request.fingerprint,
          )
        : await api.connectSFTP(
            request.host.workspaceId,
            request.host.id,
            request.password,
            request.passphrase,
            request.fingerprint,
          );

      if (ticket !== attempt.current) {
        if (result.sessionId)
          void api.disconnectSession(result.sessionId).catch(() => {});
        return;
      }

      if (result.status === "host_key_unknown") {
        setPrompt({
          ...request,
          fingerprint: result.fingerprint,
          keyType: result.keyType,
        });
        return;
      }

      if (result.status === "passphrase_required") {
        setPrompt(request);
        return;
      }

      if (!result.sessionId || closed.current.has(result.sessionId)) {
        throw new Error("The SFTP connection closed before it was ready");
      }

      let host = request.host;
      if (request.password && !host.identityId && !host.useGroupIdentity) {
        try {
          host = await api.saveHost(host, request.password);
        } catch (reason) {
          setError(
            `Connected, but password could not be saved: ${errorText(reason)}`,
          );
        }
      }

      if (ticket !== attempt.current) {
        void api.disconnectSession(result.sessionId).catch(() => {});
        return;
      }

      const previous = connection.current;
      const next = {
        id: result.sessionId,
        host,
        connected: !closed.current.has(result.sessionId),
      };
      connection.current = next;
      callbacks.current.onChange(next);
      if (previous && previous.id !== next.id)
        void api.disconnectSession(previous.id).catch(() => {});
      setPrompt(null);
      setConnectingHost(null);
      callbacks.current.onBusyChange(false);
    } catch (reason) {
      if (ticket === attempt.current) {
        setError(errorText(reason));
        setPrompt(request);
      }
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(false);
    }
  };

  const choose = (value: string) => {
    if (disabled || inFlight.current) return;
    setError("");
    if (value === "local") {
      const previous = connection.current;
      connection.current = null;
      callbacks.current.onChange(null);
      if (previous) void api.disconnectSession(previous.id).catch(() => {});
      return;
    }

    const host = hosts.find(
      (item) => `${item.workspaceId}/${item.id}` === value,
    );
    if (!host) return;
    if (
      connection.current?.connected &&
      connection.current.host.id === host.id &&
      connection.current.host.workspaceId === host.workspaceId
    )
      return;
    const request: AuthPrompt = {
      host,
      mode: "sftp",
      password: "",
      passphrase: "",
    };
    setConnectingHost(host);
    callbacks.current.onBusyChange(true);
    if (host.authMethod === "key" || host.savedPassword) void connect(request);
    else setPrompt(request);
  };

  const reconnect = () => {
    if (!destination) return;
    const host =
      hosts.find(
        (item) =>
          item.id === destination.host.id &&
          item.workspaceId === destination.host.workspaceId,
      ) ?? destination.host;
    const request: AuthPrompt = {
      host,
      mode: "sftp",
      reconnectSessionId: destination.id,
      password: "",
      passphrase: "",
    };
    callbacks.current.onBusyChange(true);
    if (host.authMethod === "key" || host.savedPassword) void connect(request);
    else setPrompt(request);
  };

  const selected = connectingHost ?? destination?.host;
  const options = [
    { value: "local", label: "This computer" },
    ...hosts.map((host) => ({
      value: `${host.workspaceId}/${host.id}`,
      label: `${host.name} · ${host.username}@${host.address}`,
      section: "Saved hosts",
    })),
  ];

  return (
    <>
      <div className="file-destination">
        <Dropdown
          label="Other destination"
          value={
            selected
              ? `${selected.workspaceId}/${selected.id}`
              : chosen
                ? "local"
                : ""
          }
          placeholder="Choose destination"
          options={options}
          onChange={choose}
          disabled={disabled || busy || Boolean(prompt)}
          icon={
            busy ? (
              <LoaderCircle size={17} className="account-spinner" />
            ) : destination ? (
              <Server size={17} />
            ) : (
              <HardDrive size={17} />
            )
          }
        />
        {destination && !destination.connected && (
          <button
            type="button"
            className="icon-button"
            aria-label="Reconnect other host"
            title={error || "Reconnect"}
            disabled={disabled || busy}
            onClick={reconnect}
          >
            <RefreshCw size={16} />
          </button>
        )}
        {error && !prompt && (
          <span className="file-destination-error" role="alert" title={error}>
            {error}
          </span>
        )}
      </div>
      {prompt && (
        <ConnectDialog
          prompt={prompt}
          busy={busy}
          error={error}
          onChange={setPrompt}
          onConnect={() => void connect(prompt)}
          onClose={cancel}
        />
      )}
    </>
  );
}
