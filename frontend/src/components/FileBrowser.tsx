import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type DragEvent,
  type PointerEvent,
} from "react";
import {
  ArrowLeft,
  ArrowRight,
  ArrowUpFromLine,
  Check,
  Eye,
  EyeOff,
  File,
  Folder,
  FolderOpen,
  FolderPlus,
  HardDrive,
  Home,
  Link,
  LoaderCircle,
  RefreshCw,
  Server,
  X,
} from "lucide-react";
import {
  api,
  onEvent,
  type FileEntry,
  type FileListing,
  type FileTransferProgress,
  type Host,
} from "../api";
import SFTPDestination, { type FileDestination } from "./SFTPDestination";
import { errorText } from "../utils";
import { OnFileDrop, OnFileDropOff } from "../../wailsjs/runtime/runtime";

type Side = "right" | "left";
type Pane = {
  listing: FileListing | null;
  input: string;
  loading: boolean;
  error: string;
};
type Transfer = {
  direction: "upload" | "download";
  paths: string[];
  target: string;
  sessionId?: string;
  peerSessionId?: string;
  destinationSide?: Side;
};
type Props = {
  sessionId: string;
  active: boolean;
  connected: boolean;
  hosts: Host[];
  host: Host;
  ratio: number;
  onResize: (ratio: number) => void;
  onDestinationChange: (name: string, id: string) => void;
};

const dragType = "application/x-shell-anywhere-files";
const emptyPane = (): Pane => ({
  listing: null,
  input: "",
  loading: false,
  error: "",
});

function sizeText(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unit]}`;
}

export default function FileBrowser({
  sessionId,
  active,
  connected,
  hosts,
  host,
  ratio,
  onResize,
  onDestinationChange,
}: Props) {
  const [destinationChosen, setDestinationChosen] = useState(false);
  const filePanes = useRef<HTMLDivElement>(null);
  const [destination, setDestination] = useState<FileDestination | null>(null);
  const [destinationBusy, setDestinationBusy] = useState(false);
  const destinationRef = useRef<FileDestination | null>(null);
  const [panes, setPanes] = useState<Record<Side, Pane>>({
    right: emptyPane(),
    left: emptyPane(),
  });
  const [selected, setSelected] = useState<Record<Side, string[]>>({
    right: [],
    left: [],
  });
  const [showHidden, setShowHidden] = useState(false);
  const [hover, setHover] = useState<{ side: Side; path: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [progress, setProgress] = useState<FileTransferProgress | null>(null);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState<{
    transfer: Transfer;
    paths: string[];
  } | null>(null);
  const [folder, setFolder] = useState<{ side: Side; name: string } | null>(
    null,
  );
  const sequence = useRef({ right: 0, left: 0 });
  const paths = useRef({ right: "", left: "/" });
  const currentTransfer = useRef("");
  const transferSession = useRef(sessionId);
  const transferring = useRef(false);
  const externalDrop = useRef<(x: number, y: number, files: string[]) => void>(
    () => {},
  );

  const load = useCallback(
    async (side: Side, directory: string) => {
      const request = ++sequence.current[side];
      setPanes((old) => ({
        ...old,
        [side]: { ...old[side], loading: true, error: "" },
      }));
      try {
        const listing = await (side === "right"
          ? destinationRef.current
            ? api.listSFTPFiles(destinationRef.current.id, directory)
            : api.listLocalFiles(directory)
          : api.listSFTPFiles(sessionId, directory));
        if (sequence.current[side] !== request) return;
        paths.current[side] = listing.path;
        setPanes((old) => ({
          ...old,
          [side]: { listing, input: listing.path, loading: false, error: "" },
        }));
        setSelected((old) => ({ ...old, [side]: [] }));
      } catch (reason) {
        if (sequence.current[side] === request)
          setPanes((old) => ({
            ...old,
            [side]: { ...old[side], loading: false, error: errorText(reason) },
          }));
      }
    },
    [sessionId],
  );

  useEffect(() => {
    if (connected)
      void Promise.resolve().then(() => load("left", paths.current.left));
  }, [connected, load]);

  useEffect(() => {
    const requests = sequence.current;
    return () => {
      requests.right++;
      requests.left++;
    };
  }, []);

  useEffect(
    () =>
      onEvent("files:progress", (data) => {
        const event = data as FileTransferProgress;
        if (
          event.sessionId === transferSession.current &&
          event.id === currentTransfer.current
        )
          setProgress(event);
      }),
    [sessionId],
  );

  const transfer = async (request: Transfer, overwrite = false) => {
    const mainId = request.sessionId ?? sessionId;
    const peerId = request.peerSessionId ?? destinationRef.current?.id ?? "";
    const mainConnected =
      mainId === sessionId
        ? connected
        : destinationRef.current?.id === mainId &&
          destinationRef.current.connected;
    if (
      !mainConnected ||
      (peerId && !destinationRef.current?.connected) ||
      destinationBusy ||
      transferring.current
    )
      return;
    if (!request.target || request.paths.length === 0) {
      setError("Choose files and a destination folder");
      return;
    }
    transferring.current = true;
    const id = crypto.randomUUID();
    currentTransfer.current = id;
    transferSession.current = mainId;
    setBusy(true);
    setCancelling(false);
    setError("");
    setConflict(null);
    setProgress({
      sessionId: mainId,
      id,
      direction: request.direction,
      phase: "preparing",
      current: "",
      copied: 0,
      total: 0,
      files: 0,
      completed: 0,
    });
    try {
      const result = await api.transferSFTP(
        mainId,
        id,
        request.direction,
        request.paths,
        request.target,
        overwrite,
        peerId,
      );
      if (result.status === "conflict")
        setConflict({
          transfer: { ...request, sessionId: mainId, peerSessionId: peerId },
          paths: result.conflicts,
        });
      setProgress((old) => (old ? { ...old, phase: result.status } : old));
    } catch (reason) {
      setError(errorText(reason));
      setProgress((old) => (old ? { ...old, phase: "failed" } : old));
    } finally {
      transferring.current = false;
      setBusy(false);
      setCancelling(false);
      void load(
        request.destinationSide ??
          (request.direction === "upload" ? "left" : "right"),
        paths.current[
          request.destinationSide ??
            (request.direction === "upload" ? "left" : "right")
        ],
      );
    }
  };

  useEffect(() => {
    externalDrop.current = (x, y, files) => {
      const element = document
        .elementFromPoint(x, y)
        ?.closest<HTMLElement>("[data-file-session]");
      if (!element || element.dataset.fileSession !== sessionId) return;
      const side = element.dataset.fileSide as Side;
      const targetId = side === "left" ? sessionId : destinationRef.current?.id;
      if (!targetId) return;
      const directory = document
        .elementFromPoint(x, y)
        ?.closest<HTMLElement>("[data-file-directory]")?.dataset.fileDirectory;
      setHover(null);
      void transfer({
        direction: "upload",
        sessionId: targetId,
        peerSessionId: "",
        destinationSide: side,
        paths: files,
        target: directory || paths.current[side],
      });
    };
  });

  useEffect(() => {
    if (!active || (!connected && !destination?.connected)) return;
    OnFileDrop((x, y, files) => externalDrop.current(x, y, files), true);
    return () => OnFileDropOff();
  }, [active, connected, destination?.connected]);

  const browseLocal = async () => {
    try {
      const directory = await api.chooseLocalFolder();
      if (directory) await load("right", directory);
    } catch (reason) {
      setError(errorText(reason));
    }
  };

  const uploadFiles = async (directory: boolean) => {
    try {
      const files = directory
        ? [await api.chooseLocalFolder()].filter(Boolean)
        : await api.chooseUploadFiles();
      if (files.length)
        await transfer({
          direction: "upload",
          peerSessionId: "",
          paths: files,
          target: paths.current.left,
        });
    } catch (reason) {
      setError(errorText(reason));
    }
  };

  const toggle = (side: Side, path: string, additive: boolean) => {
    setSelected((old) => ({
      ...old,
      [side]: additive
        ? old[side].includes(path)
          ? old[side].filter((value) => value !== path)
          : [...old[side], path]
        : [path],
    }));
  };

  const dropTarget = (event: DragEvent, side: Side) => {
    const row = (event.target as HTMLElement).closest<HTMLElement>(
      "[data-file-directory]",
    );
    return row?.dataset.fileDirectory || paths.current[side];
  };

  const drop = (event: DragEvent, side: Side) => {
    event.preventDefault();
    setHover(null);
    if (side === "right" && !destinationChosen) return;
    if (event.dataTransfer.types.includes("Files")) return;
    try {
      const data = JSON.parse(event.dataTransfer.getData(dragType)) as {
        side: Side;
        paths: string[];
        sessionId: string;
      };
      if (
        data.sessionId !== sessionId ||
        data.side === side ||
        !Array.isArray(data.paths)
      )
        return;
      void transfer({
        direction: side === "left" ? "upload" : "download",
        paths: data.paths,
        target: dropTarget(event, side),
      });
    } catch {
      /* Other app drags are ignored */
    }
  };

  const createFolder = async () => {
    if (!folder || !folder.name.trim()) return;
    const { side, name } = folder;
    try {
      await api.createFileFolder(
        side === "right" && destination ? destination.id : sessionId,
        side === "left" || destination ? "remote" : "local",
        paths.current[side],
        name.trim(),
      );
      setFolder(null);
      await load(side, paths.current[side]);
    } catch (reason) {
      setPanes((old) => ({
        ...old,
        [side]: { ...old[side], error: errorText(reason) },
      }));
    }
  };

  const pane = (side: Side) => {
    const state = panes[side];
    const isRemote = side === "left" || Boolean(destination);
    const disabled =
      destinationBusy ||
      (side === "right" && !destinationChosen) ||
      (side === "left"
        ? !connected
        : destination
          ? !destination.connected
          : false);
    const entries =
      state.listing?.entries.filter(
        (entry) => showHidden || !entry.name.startsWith("."),
      ) ?? [];
    const root = state.listing?.parent === state.listing?.path;
    const dropStyle = {
      "--wails-drop-target":
        isRemote && active && !disabled && !busy ? "drop" : "none",
    } as CSSProperties;
    return (
      <section
        className={`file-pane ${side}${hover?.side === side ? " file-drop-over" : ""}`}
        aria-label={side === "right" ? "Destination files" : "Source files"}
      >
        <div className="file-pane-title">
          {side === "right" ? (
            <SFTPDestination
              hosts={hosts}
              destination={destination}
              chosen={destinationChosen}
              disabled={busy || Boolean(conflict)}
              onBusyChange={setDestinationBusy}
              onChange={(next) => {
                const previous = destinationRef.current;
                destinationRef.current = next;
                setDestination(next);
                setDestinationChosen(true);
                onDestinationChange(
                  next?.host.name ?? "This computer",
                  next?.id ?? "",
                );
                if (previous?.id !== next?.id || !destinationChosen) {
                  sequence.current.right++;
                  paths.current.right = next ? "/" : "";
                  setPanes((old) => ({ ...old, right: emptyPane() }));
                  setSelected((old) => ({ ...old, right: [] }));
                  setProgress(null);
                  setError("");
                  setFolder(null);
                  void load("right", paths.current.right);
                } else if (next?.connected && !previous?.connected) {
                  void load("right", paths.current.right);
                }
              }}
            />
          ) : (
            <>
              <Server size={17} />
              <strong title={`${host.username}@${host.address}`}>
                {host.name}
              </strong>
            </>
          )}
          {(side === "left" || destinationChosen) && (
            <span>
              {selected[side].length
                ? `${selected[side].length} selected`
                : `${entries.length} items`}
            </span>
          )}
        </div>
        {(side === "left" || destinationChosen) && (
          <form
            className="file-location"
            onSubmit={(event) => {
              event.preventDefault();
              if (!disabled) void load(side, state.input);
            }}
          >
            <button
              type="button"
              className="icon-button"
              aria-label={`${side} parent folder`}
              title="Parent folder"
              disabled={disabled || !state.listing || root}
              onClick={() => void load(side, state.listing!.parent)}
            >
              <ArrowLeft size={16} />
            </button>
            <button
              type="button"
              className="icon-button"
              aria-label={`${side} home folder`}
              title="Home folder"
              disabled={disabled}
              onClick={() => void load(side, "")}
            >
              <Home size={16} />
            </button>
            <input
              aria-label={`${side} path`}
              value={state.input}
              disabled={disabled}
              onChange={(event) =>
                setPanes((old) => ({
                  ...old,
                  [side]: { ...old[side], input: event.target.value },
                }))
              }
              spellCheck={false}
            />
            {side === "right" && !destination && (
              <button
                type="button"
                className="icon-button"
                title="Choose local folder"
                aria-label="Choose local folder"
                onClick={() => void browseLocal()}
              >
                <FolderOpen size={16} />
              </button>
            )}
            <button
              type="button"
              className="icon-button"
              aria-label={`Refresh ${side} files`}
              disabled={disabled}
              onClick={() => void load(side, paths.current[side])}
            >
              {state.loading ? (
                <LoaderCircle size={16} className="account-spinner" />
              ) : (
                <RefreshCw size={16} />
              )}
            </button>
            <button
              type="button"
              className="icon-button"
              aria-label={`New ${side} folder`}
              title="New folder"
              disabled={disabled || !state.listing}
              onClick={() => setFolder({ side, name: "" })}
            >
              <FolderPlus size={16} />
            </button>
          </form>
        )}
        {folder?.side === side && (
          <form
            className="file-new-folder"
            onSubmit={(event) => {
              event.preventDefault();
              void createFolder();
            }}
          >
            <input
              autoFocus
              aria-label="Folder name"
              value={folder.name}
              onChange={(event) =>
                setFolder({ side, name: event.target.value })
              }
              placeholder="Folder name"
              required
            />
            <button type="submit" className="button secondary">
              Create
            </button>
            <button
              type="button"
              className="icon-button"
              aria-label="Cancel new folder"
              onClick={() => setFolder(null)}
            >
              <X size={15} />
            </button>
          </form>
        )}
        {state.error && (
          <div className="file-pane-error" role="alert">
            {state.error}
          </div>
        )}
        <div
          className="file-pane-body"
          data-file-session={sessionId}
          data-file-side={side}
          style={dropStyle}
          onDragOver={(event) => {
            if (
              disabled ||
              busy ||
              (!event.dataTransfer.types.includes(dragType) &&
                !(isRemote && event.dataTransfer.types.includes("Files")))
            )
              return;
            event.preventDefault();
            event.dataTransfer.dropEffect = "copy";
            const target = dropTarget(event, side);
            setHover((old) =>
              old?.side === side && old.path === target
                ? old
                : { side, path: target },
            );
          }}
          onDragLeave={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node))
              setHover(null);
          }}
          onDrop={(event) => drop(event, side)}
        >
          {side === "right" && !destinationChosen ? (
            <div className="file-destination-choices">
              <div className="file-destination-illustration">
                <HardDrive size={27} />
                <Server size={27} />
              </div>
              <h3>Choose a destination</h3>
              <p>
                Select this computer or a saved host above to browse files and
                transfer between the two sides
              </p>
              <button
                type="button"
                className="button secondary"
                disabled={destinationBusy}
                onClick={() => {
                  setDestinationChosen(true);
                  onDestinationChange("This computer", "");
                  void load("right", "");
                }}
              >
                <HardDrive size={16} /> This computer
              </button>
            </div>
          ) : (
            <>
              <table className="file-table" aria-label={`${side} file list`}>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Size</th>
                    <th>Modified</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((entry: FileEntry) => (
                    <tr
                      key={entry.path}
                      data-file-directory={
                        entry.directory ? entry.path : undefined
                      }
                      className={`${selected[side].includes(entry.path) ? "selected" : ""}${hover?.side === side && hover.path === entry.path ? " drop-folder" : ""}`}
                      tabIndex={0}
                      aria-selected={selected[side].includes(entry.path)}
                      draggable={!disabled && !busy && !entry.symlink}
                      onClick={(event) =>
                        toggle(side, entry.path, event.ctrlKey || event.metaKey)
                      }
                      onDoubleClick={() => {
                        if (entry.directory && !disabled)
                          void load(side, entry.path);
                      }}
                      onKeyDown={(event) => {
                        if (
                          event.key === "Enter" &&
                          entry.directory &&
                          !disabled
                        ) {
                          event.preventDefault();
                          void load(side, entry.path);
                        }
                        if (event.key === " ") {
                          event.preventDefault();
                          toggle(side, entry.path, true);
                        }
                      }}
                      onDragStart={(event) => {
                        const files = selected[side].includes(entry.path)
                          ? selected[side]
                          : [entry.path];
                        if (!selected[side].includes(entry.path))
                          toggle(side, entry.path, false);
                        event.dataTransfer.setData(
                          dragType,
                          JSON.stringify({ sessionId, side, paths: files }),
                        );
                        event.dataTransfer.effectAllowed = "copy";
                      }}
                      onDragEnd={() => setHover(null)}
                    >
                      <td>
                        <div className="file-name">
                          <input
                            type="checkbox"
                            aria-label={`Select ${entry.name}`}
                            checked={selected[side].includes(entry.path)}
                            onClick={(event) => event.stopPropagation()}
                            onChange={() => toggle(side, entry.path, true)}
                          />
                          {entry.symlink ? (
                            <Link size={17} />
                          ) : entry.directory ? (
                            <Folder size={17} />
                          ) : (
                            <File size={17} />
                          )}
                          <span title={entry.name}>{entry.name}</span>
                        </div>
                      </td>
                      <td>{entry.directory ? "—" : sizeText(entry.size)}</td>
                      <td>
                        {entry.modified > 0
                          ? new Date(entry.modified * 1000).toLocaleDateString()
                          : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {entries.length === 0 && (
                <div className="file-empty">
                  <FolderOpen size={30} />
                  <span>
                    {disabled
                      ? "Reconnect to browse remote files"
                      : state.loading
                        ? "Loading files…"
                        : "This folder is empty"}
                  </span>
                </div>
              )}
            </>
          )}
          {hover?.side === side && (
            <div className="file-drop-hint">
              {destination
                ? "Drop to copy"
                : side === "left"
                  ? "Drop to upload"
                  : "Drop to download"}
            </div>
          )}
        </div>
        <div className="file-pane-footer">
          {side === "left"
            ? "Drop files and folders here to copy"
            : !destinationChosen
              ? "Choose where you want to copy files"
              : destination
                ? "Drop files and folders here to copy"
                : "Drop remote files here to download"}
        </div>
      </section>
    );
  };

  const percentage = progress?.total
    ? Math.min(100, Math.round((progress.copied / progress.total) * 100))
    : progress?.phase === "completed"
      ? 100
      : 0;

  const changeRatio = (value: number) =>
    onResize(Math.min(80, Math.max(20, Math.round(value))));

  const resize = (event: PointerEvent<HTMLDivElement>) => {
    if (
      !filePanes.current ||
      !event.currentTarget.hasPointerCapture(event.pointerId)
    )
      return;
    const bounds = filePanes.current.getBoundingClientRect();
    changeRatio(((event.clientX - bounds.left) / bounds.width) * 100);
  };

  return (
    <div className="file-browser">
      <div className="file-actions">
        <button
          type="button"
          className="button secondary"
          disabled={
            !connected || busy || destinationBusy || !panes.left.listing
          }
          onClick={() => void uploadFiles(false)}
        >
          <ArrowUpFromLine size={15} /> Upload files
        </button>
        <button
          type="button"
          className="button secondary"
          disabled={
            !connected || busy || destinationBusy || !panes.left.listing
          }
          onClick={() => void uploadFiles(true)}
        >
          <FolderOpen size={15} /> Upload folder
        </button>
        <span className="spacer" />
        <button
          type="button"
          className="button secondary"
          aria-pressed={showHidden}
          onClick={() => setShowHidden((old) => !old)}
        >
          {showHidden ? <Eye size={15} /> : <EyeOff size={15} />} Hidden files
        </button>
      </div>
      <div
        className="file-panes"
        ref={filePanes}
        style={{ "--file-split-ratio": `${ratio}%` } as CSSProperties}
      >
        {pane("left")}
        <div className="file-transfer-buttons">
          <div
            className="file-split-divider"
            role="separator"
            aria-label="Resize SFTP split"
            aria-orientation="vertical"
            aria-valuemin={20}
            aria-valuemax={80}
            aria-valuenow={ratio}
            tabIndex={0}
            onPointerDown={(event) => {
              event.preventDefault();
              event.currentTarget.setPointerCapture(event.pointerId);
            }}
            onPointerMove={resize}
            onKeyDown={(event) => {
              if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
                event.preventDefault();
                changeRatio(ratio + (event.key === "ArrowLeft" ? -5 : 5));
              }
            }}
          />
          <button
            type="button"
            className="icon-button"
            title={
              destination ? "Copy selected to the right" : "Download selected"
            }
            aria-label={
              destination ? "Copy selected to the right" : "Download selected"
            }
            disabled={
              !connected ||
              !destinationChosen ||
              destinationBusy ||
              (destination !== null && !destination.connected) ||
              busy ||
              !selected.left.length ||
              !panes.right.listing
            }
            onClick={() =>
              void transfer({
                direction: "download",
                paths: selected.left,
                target: paths.current.right,
              })
            }
          >
            <ArrowRight size={20} />
          </button>
          <button
            type="button"
            className="icon-button"
            title={
              destination ? "Copy selected to the left" : "Upload selected"
            }
            aria-label={
              destination ? "Copy selected to the left" : "Upload selected"
            }
            disabled={
              !connected ||
              !destinationChosen ||
              destinationBusy ||
              (destination !== null && !destination.connected) ||
              busy ||
              !selected.right.length ||
              !panes.left.listing
            }
            onClick={() =>
              void transfer({
                direction: "upload",
                paths: selected.right,
                target: paths.current.left,
              })
            }
          >
            <ArrowLeft size={20} />
          </button>
        </div>
        {pane("right")}
      </div>
      {error && (
        <div className="file-error" role="alert">
          {error}
          <button
            type="button"
            className="icon-button"
            aria-label="Dismiss transfer error"
            onClick={() => setError("")}
          >
            <X size={15} />
          </button>
        </div>
      )}
      {conflict && (
        <div className="file-conflict" role="alert">
          <div>
            <strong>
              {conflict.paths.length}{" "}
              {conflict.paths.length === 1
                ? "file already exists"
                : "files already exist"}
            </strong>
            <p>Replace existing files in the destination folder?</p>
          </div>
          <button
            type="button"
            className="button secondary"
            onClick={() => {
              setConflict(null);
              setProgress(null);
            }}
          >
            Cancel
          </button>
          <button
            type="button"
            className="button primary"
            onClick={() => void transfer(conflict.transfer, true)}
          >
            Replace files
          </button>
        </div>
      )}
      {progress && !conflict && (
        <div className={`file-progress ${progress.phase}`} role="status">
          <div className="file-progress-summary">
            {busy ? (
              <LoaderCircle size={16} className="account-spinner" />
            ) : progress.phase === "completed" ? (
              <Check size={16} />
            ) : (
              <FolderOpen size={16} />
            )}
            <span>
              {cancelling
                ? "Stopping transfer…"
                : progress.phase === "preparing"
                  ? "Preparing files…"
                  : progress.phase === "completed"
                    ? "Transfer complete"
                    : progress.phase === "cancelled"
                      ? "Transfer cancelled"
                      : progress.phase === "failed"
                        ? "Transfer failed"
                        : `${progress.direction === "copy" ? "Copying" : progress.direction === "upload" ? "Uploading" : "Downloading"} ${progress.completed} of ${progress.files} files`}
            </span>
            <span className="spacer" />
            <span>
              {sizeText(progress.copied)} / {sizeText(progress.total)}
            </span>
            {busy ? (
              <button
                type="button"
                className="icon-button"
                disabled={cancelling}
                aria-label="Cancel transfer"
                onClick={() => {
                  setCancelling(true);
                  void api
                    .cancelSFTPTransfer(transferSession.current, progress.id)
                    .catch((reason) => {
                      setError(errorText(reason));
                      setCancelling(false);
                    });
                }}
              >
                <X size={16} />
              </button>
            ) : (
              <button
                type="button"
                className="icon-button"
                aria-label="Dismiss transfer status"
                onClick={() => setProgress(null)}
              >
                <X size={16} />
              </button>
            )}
          </div>
          <div
            className="file-progress-bar"
            role="progressbar"
            aria-label="File transfer"
            aria-valuenow={percentage}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <span style={{ width: `${percentage}%` }} />
          </div>
          {progress.current && (
            <div className="file-progress-path" title={progress.current}>
              {progress.current}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
