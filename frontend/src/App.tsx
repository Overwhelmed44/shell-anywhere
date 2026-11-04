import { useEffect, useMemo, useRef, useState } from "react";
import { Check, X } from "lucide-react";
import {
  api,
  isDesktop,
  onEvent,
  type Host,
  type AccountAction,
  type AccountStatus,
  type VaultStatus,
  type UpdateStatus,
  type Group,
  type Identity,
} from "./api";
import { clearTerminal, pushTerminalData } from "./terminalBus";
import type {
  AuthPrompt,
  LocalShell,
  Session,
  SplitGroup,
  SplitSide,
  Theme,
} from "./types";
import { errorText } from "./utils";
import { sessionTitle } from "./session";
import ConnectDialog from "./components/ConnectDialog";
import GroupDialog from "./components/GroupDialog";
import HostDialog from "./components/HostDialog";
import HostsView from "./components/HostsView";
import SessionTabs from "./components/SessionTabs";
import SessionWorkspace from "./components/SessionWorkspace";
import Sidebar from "./components/Sidebar";
import Topbar from "./components/Topbar";
import IdentitiesDialog from "./components/IdentitiesDialog";
import VaultDialog from "./components/VaultDialog";
import "./App.css";

const initialTheme: Theme = (() => {
  try {
    return localStorage.getItem("shell-everywhere-theme") === "light"
      ? "light"
      : "dark";
  } catch {
    return "dark";
  }
})();
document.documentElement.dataset.theme = initialTheme;

const emptyHost: Host = {
  id: "",
  name: "",
  address: "",
  port: 22,
  username: "root",
  authMethod: "password",
  keyPath: "",
  groupId: "",
  identityId: "",
  useGroupIdentity: false,
  workspaceId: "personal",
  favorite: false,
  savedPassword: false,
};

const emptyVault: VaultStatus = {
  enabled: false,
  protected: false,
  locked: false,
  busy: false,
  needsChoice: false,
  localHosts: 0,
  cloudHosts: 0,
  lastSynced: "",
  error: "",
};

type EditorContent =
  | { kind: "host"; host: Host }
  | { kind: "group"; group: Group | null }
  | { kind: "identities" };

type Editor = EditorContent & { key: number };

function App() {
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [sessions, setSessions] = useState<Session[]>([]);
  const [activeSession, setActiveSession] = useState<string | null>(null);
  const [splitGroups, setSplitGroups] = useState<SplitGroup[]>([]);
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  const [draggedSessionId, setDraggedSessionId] = useState<string | null>(null);
  const [identities, setIdentities] = useState<Identity[]>([]);
  const [editor, setEditor] = useState<Editor | null>(null);
  const editorSequence = useRef(0);
  const [authPrompt, setAuthPrompt] = useState<AuthPrompt | null>(null);
  const [connecting, setConnecting] = useState(false);
  const [connectingHostId, setConnectingHostId] = useState<string | null>(null);
  const [reconnectingSessionId, setReconnectingSessionId] = useState<
    string | null
  >(null);
  const [connectError, setConnectError] = useState("");
  const [query, setQuery] = useState("");
  const [selectedGroup, setSelectedGroup] = useState("All hosts");
  const [toast, setToast] = useState("");
  const [account, setAccount] = useState<AccountStatus>({
    user_id: "",
    username: "",
    pending: false,
  });

  const [accountBusy, setAccountBusy] = useState(isDesktop);
  const [vault, setVault] = useState<VaultStatus>(emptyVault);
  const [showVault, setShowVault] = useState(false);
  const [update, setUpdate] = useState<UpdateStatus>({
    currentVersion: "",
    latestVersion: "",
    state: isDesktop ? "checking" : "unavailable",
  });
  const nextGroupId = useRef(0);
  const connectInFlight = useRef(false);
  const activeReconnectId = useRef<string | null>(null);
  const closedDuringReconnect = useRef(new Set<string>());
  const closedDuringConnect = useRef(new Map<string, string>());

  useEffect(() => {
    if (!isDesktop) return;
    let stopped = false;
    void api
      .getAppVersion()
      .then((version) => {
        if (!stopped)
          setUpdate((previous) => ({ ...previous, currentVersion: version }));
      })
      .catch(() => {});
    void api
      .getUpdateStatus()
      .then((status) => {
        if (!stopped) setUpdate(status);
      })
      .catch(() => {
        if (!stopped)
          setUpdate((previous) => ({ ...previous, state: "unavailable" }));
      });
    return () => {
      stopped = true;
    };
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("shell-everywhere-theme", theme);
    } catch {
      /* Storage may be unavailable in preview mode. */
    }
  }, [theme]);

  useEffect(() => {
    if (isDesktop) {
      api
        .getWorkspace()
        .then((workspace) => {
          setHosts(workspace.hosts);
          setGroups(workspace.groups);
          setIdentities(workspace.identities);
        })
        .catch((reason) => setToast(errorText(reason)));
    }
    const offOutput = onEvent("terminal:output", (data) => {
      const event = data as { sessionId: string; data: string };
      pushTerminalData(event.sessionId, event.data);
    });
    const offClosed = onEvent("terminal:closed", (data) => {
      const event = data as { sessionId: string; reason: string };
      if (connectInFlight.current) {
        closedDuringConnect.current.set(event.sessionId, event.reason);
      }
      setSessions((old) =>
        old.map((session) =>
          session.id === event.sessionId
            ? { ...session, connected: false, reason: event.reason }
            : session,
        ),
      );
    });
    const offAccount = onEvent("account:changed", (data) => {
      setAccount(data as AccountStatus);
      setAccountBusy(false);
    });

    const offAccountError = onEvent("account:error", (data) => {
      setToast(String(data));
      setAccountBusy(false);
    });

    if (isDesktop) {
      api
        .getAccount()
        .then(setAccount)
        .catch((err) => setToast(errorText(err)))
        .finally(() => setAccountBusy(false));
    }

    return () => {
      offOutput();
      offClosed();
      offAccount();
      offAccountError();
    };
  }, []);

  useEffect(() => {
    const offVault = onEvent("vault:changed", (data) =>
      setVault(data as VaultStatus),
    );
    const offHosts = onEvent("hosts:changed", () => {
      void api
        .getWorkspace()
        .then((workspace) => {
          const nextHosts = workspace.hosts;
          const nextGroups = workspace.groups;
          setIdentities(workspace.identities);
          setHosts(nextHosts);
          setGroups(nextGroups);
          setSessions((previous) =>
            previous.map((session) => {
              if (session.kind === "local") return session;
              const host = nextHosts.find(
                (item) => item.id === session.host.id,
              );
              return host ? { ...session, host } : session;
            }),
          );
        })
        .catch((reason) => setToast(errorText(reason)));
    });

    return () => {
      offVault();
      offHosts();
    };
  }, []);

  useEffect(() => {
    if (!account.user_id) return;

    let stopped = false;

    api
      .getVaultStatus()
      .then((status) => {
        if (stopped) return;
        setVault(status);
        if (!status.enabled || status.locked) setShowVault(true);
        if (status.enabled && !status.locked) {
          void api
            .syncVault()
            .then((next) => {
              if (!stopped) setVault(next);
            })
            .catch(() => {});
        }
      })
      .catch((reason) => {
        if (!stopped) setToast(errorText(reason));
      });

    return () => {
      stopped = true;
    };
  }, [account.user_id]);

  useEffect(() => {
    if (!toast) return;
    const timeout = setTimeout(() => setToast(""), 4500);
    return () => clearTimeout(timeout);
  }, [toast]);

  const filteredHosts = useMemo(
    () =>
      hosts
        .filter((host) => {
          const matchGroup =
            selectedGroup === "All hosts" ||
            (selectedGroup === "Favorites"
              ? host.favorite
              : host.groupId === selectedGroup);
          const text =
            `${host.name} ${host.address} ${host.username} ${groups.find((group) => group.id === host.groupId)?.name ?? ""}`.toLowerCase();
          return matchGroup && text.includes(query.toLowerCase());
        })
        .sort(
          (a, b) =>
            Number(b.favorite) - Number(a.favorite) ||
            a.name.localeCompare(b.name),
        ),
    [hosts, groups, selectedGroup, query],
  );
  const active = sessions.find((session) => session.id === activeSession);
  const activeGroup = splitGroups.find((group) => group.id === activeGroupId);
  const splitLabel = activeGroup
    ? [activeGroup.firstId, activeGroup.secondId]
        .map((id) => sessions.find((session) => session.id === id))
        .filter((session): session is Session => Boolean(session))
        .map(sessionTitle)
        .concat(
          activeGroup.kind === "sftp" ? [activeGroup.destinationName] : [],
        )
        .join(" + ")
    : "";

  const selectSession = (id: string | null) => {
    setActiveSession(id);
    setActiveGroupId(null);
  };

  const selectSplitGroup = (id: string) => {
    const group = splitGroups.find((item) => item.id === id);
    if (!group) return;
    setActiveSession(
      group.kind !== "sftp" &&
        (activeSession === group.firstId || activeSession === group.secondId)
        ? activeSession
        : group.firstId,
    );
    setActiveGroupId(id);
  };

  const unsplitGroup = (id: string) => {
    const group = splitGroups.find((item) => item.id === id);
    if (!group || group.kind === "sftp") return;
    setSplitGroups((old) => old.filter((item) => item.id !== id));
    if (activeGroupId === id) setActiveGroupId(null);
  };

  const closeSplitGroup = (id: string) => {
    const group = splitGroups.find((item) => item.id === id);
    if (!group) return;
    const ids = new Set(
      [group.firstId, group.secondId].filter((id): id is string => Boolean(id)),
    );
    for (const id of ids) {
      if (activeReconnectId.current === id) {
        closedDuringReconnect.current.add(id);
      }
      void api.disconnectSession(id).catch(() => {});
      clearTerminal(id);
    }
    setSessions((old) => old.filter((session) => !ids.has(session.id)));
    setSplitGroups((old) => old.filter((item) => item.id !== id));
    if (activeGroupId === id) {
      const next = sessions.find((session) => !ids.has(session.id));
      const nextGroup = splitGroups.find(
        (item) =>
          item.id !== id &&
          (item.firstId === next?.id || item.secondId === next?.id),
      );
      setActiveSession(next?.id ?? null);
      setActiveGroupId(nextGroup?.id ?? null);
    }
  };

  const splitSession = (draggedId: string, side: SplitSide) => {
    if (
      !activeSession ||
      activeGroupId ||
      draggedId === activeSession ||
      !sessions.some((session) => session.id === draggedId) ||
      sessions.some(
        (session) =>
          (session.id === draggedId || session.id === activeSession) &&
          session.kind === "sftp",
      ) ||
      splitGroups.some(
        (group) =>
          group.firstId === draggedId ||
          group.secondId === draggedId ||
          group.firstId === activeSession ||
          group.secondId === activeSession,
      )
    )
      return;
    const baseId = activeSession;
    const draggedFirst = side === "left" || side === "top";
    const id = String(++nextGroupId.current);
    setSplitGroups((old) => [
      ...old,
      {
        id,
        firstId: draggedFirst ? draggedId : baseId,
        secondId: draggedFirst ? baseId : draggedId,
        direction:
          side === "left" || side === "right" ? "horizontal" : "vertical",
        ratio: 50,
      },
    ]);
    setActiveSession(draggedId);
    setActiveGroupId(id);
    setDraggedSessionId(null);
  };

  const startLocalShell = async (shell: LocalShell, shellName: string) => {
    try {
      const id = await api.startLocalShell(shell);
      setSessions((old) => [
        ...old,
        { id, kind: "local", shell, shellName, connected: true },
      ]);
      selectSession(id);
    } catch (err) {
      setToast(errorText(err));
    }
  };

  const connect = async (prompt: AuthPrompt) => {
    if (connectInFlight.current) return;
    connectInFlight.current = true;
    activeReconnectId.current = prompt.reconnectSessionId ?? null;
    closedDuringConnect.current.clear();
    setConnecting(true);
    setConnectingHostId(prompt.host.id);
    setReconnectingSessionId(prompt.reconnectSessionId ?? null);
    setConnectError("");
    try {
      const result =
        prompt.mode === "sftp"
          ? prompt.reconnectSessionId
            ? await api.reconnectSFTP(
                prompt.reconnectSessionId,
                prompt.host.workspaceId,
                prompt.host.id,
                prompt.password,
                prompt.passphrase,
                prompt.fingerprint,
              )
            : await api.connectSFTP(
                prompt.host.workspaceId,
                prompt.host.id,
                prompt.password,
                prompt.passphrase,
                prompt.fingerprint,
              )
          : prompt.reconnectSessionId
            ? await api.reconnectHost(
                prompt.reconnectSessionId,
                prompt.host.id,
                prompt.password,
                prompt.passphrase,
                prompt.fingerprint,
                prompt.host.workspaceId,
              )
            : await api.connectHost(
                prompt.host.id,
                prompt.password,
                prompt.passphrase,
                prompt.fingerprint,
                prompt.host.workspaceId,
              );
      if (result.status === "passphrase_required") {
        setAuthPrompt(prompt);
      } else if (result.status === "host_key_unknown") {
        setAuthPrompt({
          ...prompt,
          fingerprint: result.fingerprint,
          keyType: result.keyType,
        });
      } else if (result.sessionId) {
        if (
          prompt.reconnectSessionId &&
          closedDuringReconnect.current.has(prompt.reconnectSessionId)
        ) {
          void api.disconnectSession(result.sessionId).catch(() => {});
          clearTerminal(result.sessionId);
          setAuthPrompt(null);
          return;
        }
        const closedReason = closedDuringConnect.current.get(result.sessionId);
        let connectedHost = prompt.host;
        if (
          prompt.host.authMethod === "password" &&
          prompt.password &&
          !prompt.host.identityId &&
          !prompt.host.useGroupIdentity
        ) {
          try {
            connectedHost = await api.saveHost(prompt.host, prompt.password);
            setHosts((old) =>
              old.map((host) =>
                host.id === connectedHost.id ? connectedHost : host,
              ),
            );
          } catch (err) {
            setToast(
              `Connected, but password could not be saved: ${errorText(err)}`,
            );
          }
        }
        if (prompt.reconnectSessionId) {
          const reconnectId = prompt.reconnectSessionId;
          setSessions((old) =>
            old.map((session) =>
              session.id === reconnectId && session.kind !== "local"
                ? {
                    ...session,
                    host: connectedHost,
                    connected: !closedReason,
                    reason: closedReason,
                  }
                : session,
            ),
          );
        } else {
          setSessions((old) => [
            ...old,
            {
              id: result.sessionId!,
              kind: prompt.mode ?? "ssh",
              host: connectedHost,
              connected: !closedReason,
              reason: closedReason,
            },
          ]);
          if (prompt.mode === "sftp") {
            const groupId = String(++nextGroupId.current);
            setSplitGroups((old) => [
              ...old,
              {
                id: groupId,
                kind: "sftp",
                firstId: result.sessionId!,
                direction: "horizontal",
                ratio: 50,
                destinationName: "Choose destination",
              },
            ]);
            setActiveSession(result.sessionId);
            setActiveGroupId(groupId);
          } else {
            selectSession(result.sessionId);
          }
        }
        setAuthPrompt(null);
      }
    } catch (err) {
      setAuthPrompt(prompt);
      setConnectError(errorText(err));
    } finally {
      if (prompt.reconnectSessionId) {
        closedDuringReconnect.current.delete(prompt.reconnectSessionId);
      }
      activeReconnectId.current = null;
      closedDuringConnect.current.clear();
      connectInFlight.current = false;
      setConnecting(false);
      setConnectingHostId(null);
      setReconnectingSessionId(null);
    }
  };
  const startConnect = (host: Host, mode: "ssh" | "sftp" = "ssh") => {
    setConnectError("");
    const prompt: AuthPrompt = { host, mode, password: "", passphrase: "" };
    if (host.authMethod === "key" || host.savedPassword) {
      void connect(prompt);
    } else {
      setAuthPrompt(prompt);
    }
  };
  const startReconnect = (session: Session) => {
    if (
      session.kind === "local" ||
      session.connected ||
      connectInFlight.current
    )
      return;
    const host = hosts.find((item) => item.id === session.host.id);
    if (!host) {
      setToast("This host was deleted. Save it again to reconnect.");
      return;
    }
    setConnectError("");
    const prompt: AuthPrompt = {
      host,
      reconnectSessionId: session.id,
      mode: session.kind,
      password: "",
      passphrase: "",
    };
    if (host.authMethod === "key" || host.savedPassword) {
      void connect(prompt);
    } else {
      setAuthPrompt(prompt);
    }
  };
  const closeTab = (id: string) => {
    const fileGroup = splitGroups.find(
      (item) => item.kind === "sftp" && item.firstId === id,
    );
    if (fileGroup) {
      closeSplitGroup(fileGroup.id);
      return;
    }
    if (activeReconnectId.current === id) {
      closedDuringReconnect.current.add(id);
    }
    void api.disconnectSession(id).catch(() => {});
    clearTerminal(id);
    setSessions((old) => old.filter((session) => session.id !== id));
    const group = splitGroups.find(
      (item) => item.firstId === id || item.secondId === id,
    );
    if (group && group.kind !== "sftp") {
      const remainingId = group.firstId === id ? group.secondId : group.firstId;
      setSplitGroups((old) => old.filter((item) => item.id !== group.id));
      if (activeGroupId === group.id || activeSession === id) {
        setActiveGroupId(null);
        setActiveSession(remainingId);
      }
    } else if (activeSession === id) {
      const next = sessions.find((session) => session.id !== id);
      const nextGroup = splitGroups.find(
        (item) => item.firstId === next?.id || item.secondId === next?.id,
      );
      setActiveSession(next?.id ?? null);
      setActiveGroupId(nextGroup?.id ?? null);
    }
  };
  const saveHost = (host: Host, key?: number) => {
    setHosts((old) =>
      host.id && old.some((item) => item.id === host.id)
        ? old.map((item) => (item.id === host.id ? host : item))
        : [...old, host],
    );
    setEditor((current) => (current?.key === key ? null : current));
    setToast(host.name + " saved");
  };
  const refreshWorkspace = async () => {
    const workspace = await api.getWorkspace();
    setHosts(workspace.hosts);
    setGroups(workspace.groups);
    setIdentities(workspace.identities);
    setEditor((current) => {
      if (current?.kind !== "host" || !current.host.id) return current;
      const host = workspace.hosts.find((host) => host.id === current.host.id);
      return host ? { ...current, host } : current;
    });
    setSessions((old) =>
      old.map((session) => {
        if (session.kind === "local") return session;
        const host = workspace.hosts.find(
          (host) => host.id === session.host.id,
        );
        return host ? { ...session, host } : session;
      }),
    );
  };
  const saveGroup = (group: Group, key: number) => {
    void refreshWorkspace().catch((reason) => setToast(errorText(reason)));
    setEditor((current) => (current?.key === key ? null : current));
    setSelectedGroup(group.id);
    setActiveSession(null);
    setActiveGroupId(null);
    setToast(group.name + " saved");
  };
  const deleteGroup = async (id: string) => {
    const name = groups.find((group) => group.id === id)?.name ?? "Group";
    const count = hosts.filter((host) => host.groupId === id).length;
    const message =
      count === 0
        ? `Delete the group "${name}"?`
        : `Delete the group "${name}"? ${count} ${count === 1 ? "host" : "hosts"} will move to No group. Their credentials will stay available`;
    if (!window.confirm(message)) return;
    try {
      await api.deleteGroup(id);
      setGroups((old) => old.filter((group) => group.id !== id));
      await refreshWorkspace();
      setSelectedGroup("All hosts");
      setQuery("");
      setToast("Group deleted");
    } catch (err) {
      setToast(errorText(err));
    }
  };
  const openEditor = (content: EditorContent) => {
    setEditor({ ...content, key: ++editorSequence.current });
  };
  const closeEditor = (key: number) => {
    setEditor((current) => (current?.key === key ? null : current));
  };
  const editHost = (host: Host) => openEditor({ kind: "host", host });
  const newHost = () => {
    const group = groups.find((group) => group.id === selectedGroup);
    editHost({
      ...emptyHost,
      groupId: group?.id ?? "",
      useGroupIdentity: Boolean(group?.defaultIdentityId),
    });
  };
  const openGroup = (group: Group | null = null) =>
    openEditor({ kind: "group", group });
  const openIdentities = () => openEditor({ kind: "identities" });
  const deleteHost = async (id: string) => {
    if (
      !window.confirm("Delete this saved host? Active sessions will stay open.")
    )
      return;
    try {
      await api.deleteHost(id);
      setHosts((old) => old.filter((host) => host.id !== id));
      setEditor((current) =>
        current?.kind === "host" && current.host.id === id ? null : current,
      );
      setToast("Host deleted");
    } catch (err) {
      setToast(errorText(err));
    }
  };

  const toggleFavorite = async (host: Host) => {
    try {
      saveHost(await api.saveHost({ ...host, favorite: !host.favorite }));
    } catch (err) {
      setToast(errorText(err));
    }
  };

  const accountAction = async (action: AccountAction) => {
    if (accountBusy) return;

    if (!isDesktop) {
      setToast("Open the desktop app to sign in with Oxee");
      return;
    }

    setAccountBusy(true);

    try {
      if (action === "login") {
        await api.beginLogin();
      } else if (action === "cancel") {
        await api.cancelLogin();
      } else {
        await api.logout();
      }

      setAccount(await api.getAccount());
    } catch (err) {
      setToast(errorText(err));
    } finally {
      setAccountBusy(false);
    }
  };

  return (
    <div className={`app-shell${editor ? " has-editor" : ""}`}>
      <Sidebar
        onIdentities={openIdentities}
        update={update}
        account={account}
        accountBusy={accountBusy}
        onAccountAction={(action) => void accountAction(action)}
        hosts={hosts}
        groups={groups}
        selectedGroup={selectedGroup}
        onNewGroup={() => openGroup()}
        onSelectGroup={(group) => {
          setSelectedGroup(group);
          setActiveSession(null);
          setActiveGroupId(null);
        }}
      />
      <div className="main-area">
        <Topbar
          vault={vault}
          onSyncSettings={() => setShowVault(true)}
          account={account}
          accountBusy={accountBusy}
          onAccountAction={(action) => void accountAction(action)}
          label={
            activeGroup
              ? splitLabel
              : active
                ? sessionTitle(active)
                : (groups.find((group) => group.id === selectedGroup)?.name ??
                  selectedGroup)
          }
          theme={theme}
          onToggleTheme={() => setTheme(theme === "dark" ? "light" : "dark")}
          onAbout={() => setToast("Shell Anywhere — Go, React, and Wails")}
          onStartLocal={(shell, name) => void startLocalShell(shell, name)}
        />
        <SessionTabs
          sessions={sessions}
          activeSession={activeSession}
          activeGroupId={activeGroupId}
          onSelect={selectSession}
          onSelectGroup={selectSplitGroup}
          onClose={closeTab}
          onDragStart={setDraggedSessionId}
          onDragEnd={() => setDraggedSessionId(null)}
          groups={splitGroups}
          onUnsplitGroup={unsplitGroup}
          onCloseGroup={closeSplitGroup}
        />
        <main className="content">
          <HostsView
            hosts={hosts}
            filteredHosts={filteredHosts}
            selectedGroup={selectedGroup}
            groupName={
              groups.find((group) => group.id === selectedGroup)?.name ??
              selectedGroup
            }
            onEditGroup={() =>
              openGroup(
                groups.find((group) => group.id === selectedGroup) ?? null,
              )
            }
            isGroup={groups.some((group) => group.id === selectedGroup)}
            query={query}
            visible={!activeSession}
            onQueryChange={setQuery}
            onNewHost={newHost}
            onNewGroup={() => openGroup()}
            onDeleteGroup={(name) => void deleteGroup(name)}
            onConnect={startConnect}
            onFiles={(host) => startConnect(host, "sftp")}
            connectingHostId={connectingHostId}
            onEdit={editHost}
            onToggleFavorite={(host) => void toggleFavorite(host)}
          />
          <SessionWorkspace
            sessions={sessions}
            hosts={hosts}
            activeSession={activeSession}
            split={activeGroup ?? null}
            draggedSessionId={draggedSessionId}
            theme={theme}
            reconnectingSessionId={reconnectingSessionId}
            onFocus={setActiveSession}
            onEdit={editHost}
            onReconnect={startReconnect}
            onFileDestinationChange={(sessionId, name, destinationId) =>
              setSplitGroups((old) =>
                old.map((group) =>
                  group.kind === "sftp" && group.firstId === sessionId
                    ? {
                        ...group,
                        destinationName: name,
                        secondId: destinationId || undefined,
                      }
                    : group,
                ),
              )
            }
            onSplit={splitSession}
            onResize={(ratio) =>
              setSplitGroups((old) =>
                old.map((group) =>
                  group.id === activeGroupId ? { ...group, ratio } : group,
                ),
              )
            }
          />
        </main>
      </div>
      {editor?.kind === "host" && (
        <HostDialog
          key={editor.key}
          host={editor.host}
          identities={identities}
          onManageIdentities={openIdentities}
          groups={groups}
          onClose={() => closeEditor(editor.key)}
          onSaved={(host) => saveHost(host, editor.key)}
          onDelete={deleteHost}
        />
      )}
      {showVault && account.user_id && (
        <VaultDialog
          status={vault}
          onChange={setVault}
          onClose={() => setShowVault(false)}
        />
      )}
      {editor?.kind === "group" && (
        <GroupDialog
          key={editor.key}
          group={editor.group}
          identities={identities}
          onClose={() => closeEditor(editor.key)}
          onSaved={(group) => saveGroup(group, editor.key)}
        />
      )}
      {editor?.kind === "identities" && (
        <IdentitiesDialog
          key={editor.key}
          identities={identities}
          hosts={hosts}
          groups={groups}
          onChanged={refreshWorkspace}
          onClose={() => closeEditor(editor.key)}
        />
      )}
      {authPrompt && (
        <ConnectDialog
          prompt={authPrompt}
          busy={connecting}
          error={connectError}
          onChange={setAuthPrompt}
          onConnect={() => void connect(authPrompt)}
          onClose={() => setAuthPrompt(null)}
        />
      )}
      {toast && (
        <div className="toast">
          <Check size={16} /> {toast}
          <button onClick={() => setToast("")} aria-label="Dismiss">
            <X size={14} />
          </button>
        </div>
      )}
    </div>
  );
}

export default App;
