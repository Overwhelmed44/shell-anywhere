import { useEffect, useRef, useState } from "react";
import { ChevronDown, TerminalSquare } from "lucide-react";
import { api, isDesktop, type LocalShellOption } from "../api";
import type { LocalShell } from "../types";

export default function LocalTerminalMenu({
  onStart,
}: {
  onStart: (shell: LocalShell, name: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [shells, setShells] = useState<LocalShellOption[]>([]);
  const [error, setError] = useState("");
  const container = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isDesktop) return;
    let stopped = false;
    void api
      .getLocalShells()
      .then((options) => {
        if (!stopped) setShells(options);
      })
      .catch(() => {
        if (!stopped) setError("Could not load local shells");
      });
    return () => {
      stopped = true;
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const select = (shell: LocalShellOption) => {
    setOpen(false);
    onStart(shell.id, shell.name);
  };

  return (
    <div className="local-terminal-menu" ref={container}>
      <button
        className="local-terminal-trigger"
        type="button"
        disabled={!isDesktop || shells.length === 0}
        title={
          error ||
          (isDesktop
            ? "Open a terminal on this computer"
            : "Available in the desktop app")
        }
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((value) => !value)}
      >
        <TerminalSquare size={16} /> Local terminal <ChevronDown size={14} />
      </button>
      {open && (
        <div
          className="local-terminal-options"
          role="menu"
          aria-label="Local shells"
        >
          {shells.map((shell) => (
            <button
              key={shell.id}
              type="button"
              role="menuitem"
              onClick={() => select(shell)}
            >
              {shell.name}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
