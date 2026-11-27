import { useRef, useState, type CSSProperties, type ReactNode } from "react";
import { Activity, Columns2, Container, TerminalSquare } from "lucide-react";
import { serverExtensionLabels, type ServerExtensionId } from "../serverMetrics";

export type SSHView = "terminal" | "extension" | "split";

type Props = {
  id: string;
  view: SSHView;
  active: boolean;
  focused: boolean;
  extension: ReactNode;
  extensions: ServerExtensionId[];
  selectedExtension: ServerExtensionId;
  onExtensionChange: (id: ServerExtensionId) => void;
  onViewChange: (view: SSHView) => void;
  terminal: (state: { active: boolean; focused: boolean }) => ReactNode;
};

// These are views of one session, so keep the terminal mounted across layouts.
export default function SSHWorkspace({ id, view, active, focused, extension, extensions, selectedExtension, onExtensionChange, onViewChange, terminal }: Props) {
  const container = useRef<HTMLDivElement>(null);
  const [ratio, setRatio] = useState(50);
  const lastFocusedPane = useRef<"terminal" | "extension">("terminal");
  const [focusedPane, setFocusedPane] = useState<"terminal" | "extension" | "divider">("terminal");
  const layout = extension ? view : "terminal";
  const terminalVisible = layout !== "extension";
  const extensionVisible = layout !== "terminal";
  const extensionLabel = serverExtensionLabels[selectedExtension];
  const focusPane = (pane: "terminal" | "extension") => {
    lastFocusedPane.current = pane;
    setFocusedPane(pane);
  };
  const select = (next: SSHView) => {
    focusPane(next === "extension" ? "extension" : "terminal");
    onViewChange(next);
  };
  const clampRatio = (value: number) => setRatio(Math.min(80, Math.max(20, Math.round(value))));

  return <div className="ssh-workspace">
    <div className="ssh-viewbar" role="group" aria-label="Connection views">
      <button type="button" className="ssh-view-button" aria-pressed={layout === "terminal"}
        aria-controls={`${id}-terminal`} onClick={() => select("terminal")}>
        <TerminalSquare size={14} />Terminal
      </button>
      {extension && <>
        {extensions.map((item) => <button key={item} type="button" className="ssh-view-button"
          aria-pressed={extensionVisible && item === selectedExtension}
          aria-controls={`${id}-extension`} onClick={() => {
            onExtensionChange(item);
            if (layout !== "split") select("extension");
            else focusPane("extension");
          }}>
          {item === "docker" ? <Container size={14} /> : <Activity size={14} />}{serverExtensionLabels[item]}
        </button>)}
        <button type="button" className="ssh-view-button ssh-split-toggle" aria-pressed={layout === "split"}
          aria-label={layout === "split" ? "Exit split" : "Split view"}
          aria-controls={`${id}-panes`} title={layout === "split" ? "Return to a single view" : `Show terminal and ${extensionLabel} side by side`}
          onClick={() => select(layout === "split" ? lastFocusedPane.current : "split")}>
          <Columns2 size={14} /><span>{layout === "split" ? "Exit split" : "Split view"}</span>
        </button>
      </>}
    </div>
    <div ref={container} id={`${id}-panes`} className={`ssh-view-panes layout-${layout}`}
      style={{ "--ssh-terminal-ratio": `${ratio}%` } as CSSProperties}>
      <div id={`${id}-terminal`} className="ssh-terminal-pane" hidden={!terminalVisible}
        onPointerDownCapture={() => focusPane("terminal")}
        onFocusCapture={() => focusPane("terminal")}>
        {terminal({ active: active && terminalVisible, focused: focused && terminalVisible && focusedPane === "terminal" })}
      </div>
      {extension && <div id={`${id}-extension`} className="ssh-extension-pane" hidden={!extensionVisible}
        onPointerDownCapture={() => focusPane("extension")}
        onFocusCapture={() => focusPane("extension")}>
        {extension}
      </div>}
      {layout === "split" && <div className="ssh-pane-divider" role="separator" tabIndex={0}
        aria-label={`Resize terminal and ${extensionLabel}`} aria-orientation="vertical"
        aria-valuemin={20} aria-valuemax={80} aria-valuenow={ratio} aria-valuetext={`Terminal ${ratio}%, ${extensionLabel} ${100 - ratio}%`}
        onFocus={() => setFocusedPane("divider")}
        onPointerDown={(event) => {
          event.preventDefault();
          event.currentTarget.focus();
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (!event.currentTarget.hasPointerCapture(event.pointerId) || !container.current) return;
          const bounds = container.current.getBoundingClientRect();
          if (bounds.width > 0) clampRatio((event.clientX - bounds.left) / bounds.width * 100);
        }}
        onPointerUp={(event) => {
          if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
        }}
        onDoubleClick={() => setRatio(50)}
        onKeyDown={(event) => {
          const next = event.key === "ArrowLeft" ? ratio - (event.shiftKey ? 10 : 2)
            : event.key === "ArrowRight" ? ratio + (event.shiftKey ? 10 : 2)
            : event.key === "Home" ? 20 : event.key === "End" ? 80 : null;
          if (next === null) return;
          event.preventDefault();
          clampRatio(next);
        }} />}
    </div>
  </div>;
}
