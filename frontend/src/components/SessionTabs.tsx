import { Columns2, FolderOpen, HardDrive, Rows2, X } from "lucide-react";
import type { Session, SplitGroup } from "../types";
import { sessionTitle } from "../session";

type Props = {
  sessions: Session[];
  activeSession: string | null;
  activeGroupId: string | null;
  groups: SplitGroup[];
  onSelect: (id: string | null) => void;
  onSelectGroup: (id: string) => void;
  onClose: (id: string) => void;
  onDragStart: (id: string) => void;
  onDragEnd: () => void;
  onUnsplitGroup: (id: string) => void;
  onCloseGroup: (id: string) => void;
};

type SessionTabProps = Pick<
  Props,
  "onSelect" | "onClose" | "onDragStart" | "onDragEnd"
> & {
  session: Session;
  active: boolean;
  canDrag: boolean;
};

function SessionTab({
  session,
  active,
  canDrag,
  onSelect,
  onClose,
  onDragStart,
  onDragEnd,
}: SessionTabProps) {
  const title = sessionTitle(session);
  return (
    <div className={`tab session-tab${active ? " active" : ""}`}>
      <button
        className={`session-tab-select${canDrag ? " draggable" : ""}`}
        type="button"
        title={canDrag ? "Drag onto the open terminal to split" : undefined}
        draggable={canDrag}
        onClick={() => onSelect(session.id)}
        onDragStart={(event) => {
          if (!canDrag) {
            event.preventDefault();
            return;
          }
          event.dataTransfer.effectAllowed = "move";
          event.dataTransfer.setData("text/plain", session.id);
          onDragStart(session.id);
        }}
        onDragEnd={onDragEnd}
      >
        {session.kind === "sftp" && <FolderOpen size={14} aria-hidden="true" />}
        <span className={session.connected ? "tab-dot connected" : "tab-dot"} />
        <span className="session-tab-title">{title}</span>
      </button>
      <button
        className="tab-close"
        type="button"
        title={`Close ${title}`}
        aria-label={`Close ${title}`}
        draggable={false}
        onDragStart={(event) => event.preventDefault()}
        onClick={() => onClose(session.id)}
      >
        <X size={13} />
      </button>
    </div>
  );
}

export default function SessionTabs({
  sessions,
  activeSession,
  activeGroupId,
  groups,
  onSelect,
  onSelectGroup,
  onClose,
  onDragStart,
  onDragEnd,
  onUnsplitGroup,
  onCloseGroup,
}: Props) {
  if (sessions.length === 0) return null;
  const groupBySession = new Map<string, SplitGroup>();
  const groupIndex = new Map<string, number>();
  for (const group of groups) {
    const firstIndex = sessions.findIndex(
      (session) => session.id === group.firstId,
    );
    const secondIndex = sessions.findIndex(
      (session) => session.id === group.secondId,
    );
    if (firstIndex < 0 || (group.kind !== "sftp" && secondIndex < 0)) continue;
    groupBySession.set(group.firstId, group);
    if (group.secondId) groupBySession.set(group.secondId, group);
    groupIndex.set(
      group.id,
      group.kind === "sftp" ? firstIndex : Math.min(firstIndex, secondIndex),
    );
  }
  const tabProps = { onSelect, onClose, onDragStart, onDragEnd };

  return (
    <div className="tabbar">
      <button
        className={!activeSession ? "tab hosts-tab active" : "tab hosts-tab"}
        onClick={() => onSelect(null)}
      >
        <HardDrive size={15} /> Hosts
      </button>
      {sessions.map((session, index) => {
        const group = groupBySession.get(session.id);
        if (group) {
          if (index !== groupIndex.get(group.id)) return null;
          const first = sessions.find((item) => item.id === group.firstId)!;
          const second = sessions.find((item) => item.id === group.secondId)!;
          const fileGroup = group.kind === "sftp";
          return (
            <div
              key={`split-${group.id}`}
              className={`tab split-tab-group${activeGroupId === group.id ? " active" : ""}`}
            >
              {fileGroup ? (
                <span className="split-tab-icon" title="SFTP split group">
                  <Columns2 size={15} />
                </span>
              ) : (
                <button
                  className="split-tab-unsplit"
                  type="button"
                  title="Unsplit into separate terminal tabs"
                  aria-label="Unsplit terminals"
                  onClick={() => onUnsplitGroup(group.id)}
                >
                  {group.direction === "horizontal" ? (
                    <Columns2 size={15} />
                  ) : (
                    <Rows2 size={15} />
                  )}
                </button>
              )}
              <button
                className="split-tab-select"
                type="button"
                onClick={() => onSelectGroup(group.id)}
                title={
                  fileGroup ? "Show SFTP split group" : "Show split terminals"
                }
              >
                <span>
                  {sessionTitle(first)} +{" "}
                  {group.kind === "sftp"
                    ? group.destinationName
                    : sessionTitle(second)}
                </span>
              </button>
              <button
                className="tab-close"
                type="button"
                title={
                  fileGroup ? "Close SFTP split group" : "Close both terminals"
                }
                aria-label={
                  fileGroup
                    ? "Close SFTP split group"
                    : "Close split group and both terminals"
                }
                draggable={false}
                onDragStart={(event) => event.preventDefault()}
                onClick={() => onCloseGroup(group.id)}
              >
                <X size={13} />
              </button>
            </div>
          );
        }
        return (
          <SessionTab
            key={session.id}
            session={session}
            active={!activeGroupId && activeSession === session.id}
            canDrag={Boolean(
              !activeGroupId &&
                activeSession &&
                activeSession !== session.id &&
                session.kind !== "sftp" &&
                sessions.find((item) => item.id === activeSession)?.kind !==
                  "sftp",
            )}
            {...tabProps}
          />
        );
      })}
    </div>
  );
}
