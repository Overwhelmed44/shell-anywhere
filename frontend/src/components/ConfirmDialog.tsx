import { useEffect, useId, useRef, useState } from "react";
import { Download, LoaderCircle, Trash2, X } from "lucide-react";
import { errorText } from "../utils";

export type Confirmation = {
  title: string;
  description: string;
  confirmLabel: string;
  busyLabel: string;
  danger?: boolean;
  onConfirm: () => Promise<void>;
};

export default function ConfirmDialog({
  title,
  description,
  confirmLabel,
  busyLabel,
  danger,
  onConfirm,
  onClose,
}: Confirmation & { onClose: () => void }) {
  const titleId = useId();
  const descriptionId = useId();
  const container = useRef<HTMLDivElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const inFlight = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    cancelButton.current?.focus();
    return () => {
      if (previous?.isConnected) previous.focus();
    };
  }, []);

  const close = () => {
    if (!inFlight.current) onClose();
  };
  const confirm = async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      await onConfirm();
      onClose();
    } catch (reason) {
      setError(errorText(reason));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  };

  return (
    <div
      className="modal-backdrop confirmation-backdrop p-5"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) close();
      }}
    >
      <div
        ref={container}
        className="modal confirmation-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        aria-busy={busy}
        tabIndex={-1}
        onKeyDown={(event) => {
          if (event.key === "Escape") {
            event.preventDefault();
            event.stopPropagation();
            close();
          }
          if (event.key === "Tab") {
            const buttons = container.current?.querySelectorAll<HTMLButtonElement>(
              "button:not(:disabled)",
            );
            if (!buttons?.length) {
              event.preventDefault();
              container.current?.focus();
              return;
            }
            const first = buttons[0];
            const last = buttons[buttons.length - 1];
            if (
              event.shiftKey &&
              (document.activeElement === first || document.activeElement === container.current)
            ) {
              event.preventDefault();
              last.focus();
            } else if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault();
              first.focus();
            }
          }
        }}
      >
        <div className="modal-heading">
          <div className="modal-icon">
            {danger ? <Trash2 size={21} /> : <Download size={21} />}
          </div>
          <button
            type="button"
            className="icon-button"
            aria-label="Close dialog"
            disabled={busy}
            onClick={close}
          >
            <X size={18} />
          </button>
        </div>
        <h2 id={titleId}>{title}</h2>
        <p className="modal-intro" id={descriptionId}>{description}</p>
        {error && (
          <div className="form-error" role="alert">{error}</div>
        )}
        {busy && (
          <p className="confirmation-progress" role="status">
            <LoaderCircle size={16} className="account-spinner" /> {busyLabel}
          </p>
        )}
        <div className="modal-actions">
          <span className="spacer" />
          <button
            ref={cancelButton}
            type="button"
            className="button secondary"
            disabled={busy}
            onClick={close}
          >
            Cancel
          </button>
          <button
            type="button"
            className={`button ${danger ? "danger" : "primary"}`}
            disabled={busy}
            onClick={() => void confirm()}
          >
            {busy ? busyLabel : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
