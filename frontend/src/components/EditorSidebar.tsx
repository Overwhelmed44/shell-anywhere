import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { X } from "lucide-react";

type Props = {
  title: string;
  eyebrow?: string;
  busy?: boolean;
  onClose: () => void;
  children: ReactNode;
};

export default function EditorSidebar({
  title,
  eyebrow,
  busy,
  onClose,
  children,
}: Props) {
  const container = useRef<HTMLElement>(null);
  const [previous] = useState(
    () => document.activeElement as HTMLElement | null,
  );
  const titleId = useId();

  useEffect(() => {
    return () => {
      if (previous?.isConnected) previous.focus();
    };
  }, [previous]);

  useEffect(() => {
    if (container.current?.contains(document.activeElement)) return;
    (
      container.current?.querySelector<HTMLElement>("input") ??
      container.current?.querySelector<HTMLElement>("button")
    )?.focus();
  }, []);

  return (
    <aside
      ref={container}
      className="editor-sidebar"
      aria-labelledby={titleId}
      onKeyDown={(event) => {
        if (event.key === "Escape" && !busy) {
          event.preventDefault();
          event.stopPropagation();
          onClose();
        }
      }}
    >
      <div className="editor-heading">
        <div>
          {eyebrow && <div className="eyebrow">{eyebrow}</div>}
          <h2 id={titleId}>{title}</h2>
        </div>
        <button
          type="button"
          className="icon-button"
          onClick={onClose}
          disabled={busy}
          aria-label={`Close ${title.toLowerCase()}`}
        >
          <X size={18} />
        </button>
      </div>
      <div className="editor-content">{children}</div>
    </aside>
  );
}
