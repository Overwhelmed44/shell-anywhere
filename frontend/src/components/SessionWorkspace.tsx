import { useRef, useState, type CSSProperties, type PointerEvent } from "react";
import type { Host } from "../api";
import type { Session, SplitGroup, SplitSide, Theme } from "../types";
import SessionView from "./SessionView";

type Props = {
  sessions: Session[];
  hosts: Host[];
  activeSession: string | null;
  split: SplitGroup | null;
  draggedSessionId: string | null;
  theme: Theme;
  reconnectingSessionId: string | null;
  onFocus: (id: string) => void;
  onEdit: (host: Host) => void;
  onReconnect: (session: Session) => void;
  onSplit: (draggedId: string, side: SplitSide) => void;
  onResize: (ratio: number) => void;
  onFileDestinationChange: (
    sessionId: string,
    name: string,
    destinationId: string,
  ) => void;
};

const sides: SplitSide[] = ["left", "right", "top", "bottom"];

export default function SessionWorkspace({
  sessions,
  hosts,
  activeSession,
  split,
  draggedSessionId,
  theme,
  reconnectingSessionId,
  onFocus,
  onEdit,
  onReconnect,
  onSplit,
  onResize,
  onFileDestinationChange,
}: Props) {
  const container = useRef<HTMLDivElement>(null);
  const [hoveredSide, setHoveredSide] = useState<SplitSide | null>(null);
  const canSplit = Boolean(
    draggedSessionId &&
      !split &&
      activeSession &&
      draggedSessionId !== activeSession &&
      sessions.length > 1 &&
      !sessions.some(
        (session) =>
          (session.id === activeSession || session.id === draggedSessionId) &&
          session.kind === "sftp",
      ),
  );
  const style =
    split && split.kind !== "sftp"
      ? ({ "--split-ratio": `${split.ratio}%` } as CSSProperties)
      : undefined;

  const resize = (event: PointerEvent<HTMLDivElement>) => {
    if (
      !split ||
      !container.current ||
      !event.currentTarget.hasPointerCapture(event.pointerId)
    )
      return;
    const bounds = container.current.getBoundingClientRect();
    const position =
      split.direction === "horizontal"
        ? (event.clientX - bounds.left) / bounds.width
        : (event.clientY - bounds.top) / bounds.height;
    onResize(Math.min(80, Math.max(20, Math.round(position * 100))));
  };

  return (
    <div
      ref={container}
      className={`session-workspace${!activeSession ? " hidden" : ""}${split ? (split.kind === "sftp" ? " sftp-split-group" : ` split-${split.direction}`) : ""}`}
      style={style}
    >
      {sessions.map((session) => {
        const placement = !activeSession
          ? "hidden"
          : split?.firstId === session.id
            ? "first"
            : split?.secondId === session.id
              ? "second"
              : !split && activeSession === session.id
                ? "single"
                : "hidden";
        return (
          <SessionView
            key={session.id}
            session={session}
            hosts={hosts}
            placement={placement}
            active={placement !== "hidden"}
            focused={activeSession === session.id}
            theme={theme}
            reconnecting={reconnectingSessionId === session.id}
            onEdit={onEdit}
            onFocus={onFocus}
            onReconnect={onReconnect}
            fileRatio={split?.firstId === session.id ? split.ratio : 50}
            onFileResize={onResize}
            onFileDestinationChange={onFileDestinationChange}
          />
        );
      })}
      {split && split.kind !== "sftp" && (
        <div
          className={`split-divider ${split.direction}`}
          role="separator"
          aria-label="Resize split terminals"
          aria-orientation={
            split.direction === "horizontal" ? "vertical" : "horizontal"
          }
          onPointerDown={(event) => {
            event.preventDefault();
            event.currentTarget.setPointerCapture(event.pointerId);
          }}
          onPointerMove={resize}
        />
      )}
      {canSplit && draggedSessionId && (
        <div
          className="split-drop-overlay"
          onDragLeave={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node))
              setHoveredSide(null);
          }}
        >
          {sides.map((side) => (
            <div
              key={side}
              className={`split-drop-target ${side}${hoveredSide === side ? " highlighted" : ""}`}
              onDragOver={(event) => {
                event.preventDefault();
                event.dataTransfer.dropEffect = "move";
                if (hoveredSide !== side) setHoveredSide(side);
              }}
              onDrop={(event) => {
                event.preventDefault();
                setHoveredSide(null);
                onSplit(draggedSessionId, side);
              }}
            >
              Split {side}
            </div>
          ))}
          <div className="split-drop-center">Drop on an edge to split</div>
        </div>
      )}
    </div>
  );
}
