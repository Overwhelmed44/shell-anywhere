import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Copy, ClipboardPaste } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { api } from "./api";
import { subscribeTerminal } from "./terminalBus";
import { copyText, readClipboardText } from "./clipboard";
import "@xterm/xterm/css/xterm.css";

const clipboardModifier = navigator.platform.includes("Mac") ? "Cmd" : "Ctrl";

type Props = {
  sessionId: string;
  active: boolean;
  focused: boolean;
  connected: boolean;
  theme: "dark" | "light";
  label: string;
};

const terminalThemes = {
  dark: {
    background: "#10141b",
    foreground: "#dbe5ef",
    cursor: "#c1c1ff",
    selectionBackground: "#7772df88",
    black: "#202936",
    red: "#f58d91",
    green: "#90d8a5",
    yellow: "#f2ce86",
    blue: "#8fb8f8",
    magenta: "#c7a8ee",
    cyan: "#8edbd5",
    white: "#dbe5ef",
    brightBlack: "#778390",
    brightWhite: "#ffffff",
  },
  light: {
    background: "#ffffff",
    foreground: "#20243a",
    cursor: "#514a98",
    selectionBackground: "#c1c1ff99",
    black: "#20243a",
    red: "#b54251",
    green: "#18805b",
    yellow: "#9b711d",
    blue: "#3559b6",
    magenta: "#6e50a8",
    cyan: "#126d82",
    white: "#59627b",
    brightBlack: "#737a8c",
    brightWhite: "#20243a",
  },
};

export default function TerminalPane({
  sessionId,
  active,
  focused,
  connected,
  theme,
  label,
}: Props) {
  const container = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);
  const fit = useRef<FitAddon | null>(null);
  const menuElement = useRef<HTMLDivElement>(null);
  const [menu, setMenu] = useState<{
    x: number;
    y: number;
    selection: string;
  } | null>(null);
  const [clipboardError, setClipboardError] = useState("");

  const copy = async (selection: string) => {
    if (!selection) return;

    try {
      setClipboardError("");
      await copyText(selection);
    } catch {
      setClipboardError("Could not copy text");
    }
  };

  const paste = async (term: Terminal) => {
    try {
      setClipboardError("");
      const text = await readClipboardText();

      if (terminal.current === term) {
        term.paste(text);
      }
    } catch {
      setClipboardError("Could not paste text");
    }
  };

  useEffect(() => {
    if (!container.current) return;
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"Cascadia Code", "JetBrains Mono", Consolas, monospace',
      fontSize: 13,
      lineHeight: 1.35,
      scrollback: 5000,
      theme: terminalThemes.dark,
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(container.current);
    terminal.current = term;
    fit.current = fitAddon;

    term.attachCustomKeyEventHandler((event) => {
      if (event.type !== "keydown" || event.altKey) return true;

      const modifier = event.ctrlKey || event.metaKey;
      const copyKey = event.code === "KeyC" || event.key.toLowerCase() === "c";
      const pasteKey = event.code === "KeyV" || event.key.toLowerCase() === "v";

      if (modifier && copyKey && (term.hasSelection() || event.shiftKey)) {
        event.preventDefault();
        void copy(term.getSelection());

        return false;
      }

      if ((modifier && pasteKey) || (event.shiftKey && event.key === "Insert")) {
        event.preventDefault();
        void paste(term);

        return false;
      }

      return true;
    });

    const openMenu = (event: MouseEvent) => {
      event.preventDefault();
      event.stopPropagation();

      setMenu({
        x: Math.max(8, Math.min(event.clientX, window.innerWidth - 200)),
        y: Math.max(8, Math.min(event.clientY, window.innerHeight - 92)),
        selection: term.getSelection(),
      });
    };

    const element = container.current;
    element.addEventListener("contextmenu", openMenu, true);

    const removeInput = term.onData((data) => {
      void api.writeSession(sessionId, data).catch(() => {});
    });
    const unsubscribe = subscribeTerminal(sessionId, (data) =>
      term.write(data),
    );
    return () => {
      element.removeEventListener("contextmenu", openMenu, true);
      removeInput.dispose();
      unsubscribe();
      term.dispose();
      terminal.current = null;
      fit.current = null;
    };
  }, [sessionId]);

  useEffect(() => {
    if (!menu || !active) return;

    menuElement.current?.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus();

    const closeOnOutsideClick = (event: PointerEvent) => {
      if (!menuElement.current?.contains(event.target as Node)) {
        setMenu(null);
      }
    };

    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setMenu(null);
        terminal.current?.focus();
      }
    };

    const close = () => setMenu(null);

    document.addEventListener("pointerdown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", close);
    window.addEventListener("blur", close);

    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", close);
      window.removeEventListener("blur", close);
    };
  }, [menu, active]);

  useEffect(() => {
    if (!clipboardError) return;

    const timeout = setTimeout(() => setClipboardError(""), 4000);

    return () => clearTimeout(timeout);
  }, [clipboardError]);

  useEffect(() => {
    if (terminal.current)
      terminal.current.options.theme = terminalThemes[theme];
  }, [theme]);

  useEffect(() => {
    if (!active || !container.current) return;
    let frame = 0;
    let lastRows = 0;
    let lastColumns = 0;
    const resize = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        if (!fit.current || !terminal.current || !connected) return;
        fit.current.fit();
        const { rows, cols } = terminal.current;
        if (rows !== lastRows || cols !== lastColumns) {
          lastRows = rows;
          lastColumns = cols;
          void api.resizeSession(sessionId, rows, cols).catch(() => {});
        }
      });
    };
    const observer = new ResizeObserver(resize);
    observer.observe(container.current);
    resize();
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [active, connected, sessionId]);

  useEffect(() => {
    if (focused) terminal.current?.focus();
  }, [focused]);

  return (
    <>
      <div ref={container} className="terminal-canvas" aria-label={label} />
      {active && menu && createPortal(
        <div
          ref={menuElement}
          className="terminal-context-menu"
          role="menu"
          aria-label="Terminal clipboard"
          style={{ left: menu.x, top: menu.y }}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <button
            type="button"
            role="menuitem"
            disabled={!menu.selection}
            onClick={() => {
              void copy(menu.selection);
              setMenu(null);
              terminal.current?.focus();
            }}
          >
            <Copy size={15} /> Copy <kbd>{clipboardModifier}+C</kbd>
          </button>
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              if (terminal.current) void paste(terminal.current);
              setMenu(null);
              terminal.current?.focus();
            }}
          >
            <ClipboardPaste size={15} /> Paste <kbd>{clipboardModifier}+V</kbd>
          </button>
        </div>,
        document.body,
      )}
      {clipboardError && createPortal(
        <div className="terminal-clipboard-error" role="alert">
          {clipboardError}
        </div>,
        document.body,
      )}
    </>
  );
}
