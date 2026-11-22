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
  ArrowLeftRight,
  Check,
  Eye,
  EyeOff,
  FolderOpen,
  HardDrive,
  LoaderCircle,
  Server,
  X,
} from "lucide-react";
import {
  api,
  onEvent,
  type FileListing,
  type FileTransferProgress,
  type Host,
} from "../api";
import SFTPDestination, { type FileDestination } from "./SFTPDestination";
import FilePane, { fileDragType } from "./FilePane";
import { sizeText } from "./fileBrowserUtils";
import { errorText } from "../utils";
import { OnFileDrop, OnFileDropOff } from "../../wailsjs/runtime/runtime";
import "./FileBrowser.css";

type Side = "right" | "left";
type Pane = {
  listing: FileListing | null;
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

const emptyPane = (): Pane => ({
  listing: null,
  loading: false,
  error: "",
});

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
  const dragSource = useRef<Side | null>(null);
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
  const sequence = useRef({ right: 0, left: 0 });
  const paths = useRef({ right: "", left: "/" });
  const currentTransfer = useRef("");
  const transferSession = useRef(sessionId);
  const transferring = useRef(false);
  const externalDrop = useRef<(x: number, y: number, files: string[]) => void>(
    () => { },
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
          [side]: { listing, loading: false, error: "" },
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
      (conflict && !overwrite) ||
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
      const data = JSON.parse(event.dataTransfer.getData(fileDragType)) as {
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

  const pane = (side: Side) => {
    const state = panes[side];
    const isRemote = side === "left" || Boolean(destination);
    const disabled = destinationBusy || (side === "right" && !destinationChosen) ||
      (side === "left" ? !connected : destination ? !destination.connected : false);
    const canTransfer = connected && destinationChosen && !destinationBusy &&
      (!destination || destination.connected) && !busy && !conflict &&
      Boolean(panes.left.listing && panes.right.listing);
    const transferLabel = destination ? "Copy" : side === "left" ? "Download" : "Upload";
    const dropStyle = { "--wails-drop-target": isRemote && active && !disabled && !busy && !conflict ? "drop" : "none" } as CSSProperties;
    return (
      <FilePane
        key={side}
        side={side}
        sessionId={sessionId}
        active={active}
        storageSessionId={isRemote ? side === "left" ? sessionId : destination!.id : ""}
        title={side === "right" ? <SFTPDestination
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
            onDestinationChange(next?.host.name ?? "This computer", next?.id ?? "");
            if (previous?.id !== next?.id || !destinationChosen) {
              sequence.current.right++;
              paths.current.right = next ? "/" : "";
              setPanes((old) => ({ ...old, right: emptyPane() }));
              setSelected((old) => ({ ...old, right: [] }));
              setProgress(null);
              setError("");
              void load("right", paths.current.right);
            } else if (next?.connected && !previous?.connected) {
              void load("right", paths.current.right);
            }
          }}
        /> : <>
          <span className="file-host-icon">
            <Server size={18} />
          </span>
          <strong title={`${host.username}@${host.address}`}>
            {host.name}
          </strong>
          <span className="file-endpoint-label">SFTP</span>
        </>}
        subtitle={side === "left" ? `${host.username}@${host.address}` : !destinationChosen ? "Local computer or another server" : destination ? `${destination.host.username}@${destination.host.address}` : "Local filesystem"}
        ready={side === "left" || destinationChosen}
        disabled={disabled}
        busy={busy || Boolean(conflict)}
        listing={state.listing}
        loading={state.loading}
        error={state.error}
        selected={selected[side]}
        showHidden={showHidden}
        hoverPath={hover?.side === side ? hover.path : undefined}
        dropStyle={dropStyle}
        canTransfer={Boolean(canTransfer)}
        transferLabel={transferLabel}
        dropLabel={destination ? "Copy" : side === "left" ? "Upload" : "Download"}
        onNavigate={(directory) => void load(side, directory)}
        onSelect={(files) => setSelected((old) => ({ ...old, [side]: files }))}
        onCreateFolder={async (name) => {
          await api.createFileFolder(side === "right" && destination ? destination.id : sessionId, isRemote ? "remote" : "local", paths.current[side], name);
          await load(side, paths.current[side]);
        }}
        onBrowseLocal={side === "right" && !destination ? () => void browseLocal() : undefined}
        onTransfer={(files) => void transfer({ direction: side === "left" ? "download" : "upload", paths: files, target: paths.current[side === "left" ? "right" : "left"] })}
        onDragOver={(event) => {
          if (event.dataTransfer.types.includes(fileDragType) && dragSource.current === side) return;
          if (disabled || busy || conflict || (!event.dataTransfer.types.includes(fileDragType) && !(isRemote && event.dataTransfer.types.includes("Files")))) return;
          event.preventDefault();
          event.dataTransfer.dropEffect = "copy";
          const target = dropTarget(event, side);
          setHover((old) => old?.side === side && old.path === target ? old : { side, path: target });
        }}
        onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setHover(null); }}
        onDrop={(event) => { if (!disabled && !busy && !conflict) drop(event, side); }}
        onDragStart={() => { dragSource.current = side; }}
        onDragEnd={() => { dragSource.current = null; setHover(null); }}
        onDragScroll={(x, y) => {
          const element = document.elementFromPoint(x, y);
          const body = element?.closest<HTMLElement>("[data-file-session]");
          if (body?.dataset.fileSession !== sessionId || body.dataset.fileSide !== side) return;
          const target = element?.closest<HTMLElement>("[data-file-directory]")?.dataset.fileDirectory || paths.current[side];
          setHover((old) => old?.side === side && old.path === target ? old : { side, path: target });
        }}
        emptyDestination={<div className="file-destination-choices">

          <div className="file-destination-illustration">
            <span>
              <Server size={28} />
            </span>
            <ArrowLeftRight size={22} />
            <span>
              <HardDrive size={28} />
            </span>
          </div>


          <h3>Choose a destination</h3>


          <p>Open your computer or choose a saved host above, then drag files between the two sides</p>


          <button
            type="button"
            className="button primary"
            disabled={destinationBusy}
            onClick={() => { setDestinationChosen(true); onDestinationChange("This computer", ""); void load("right", ""); }}
          >
            <HardDrive size={16} /> Open this computer <ArrowRight size={15} />
          </button>


          <span className="file-destination-note">Your files stay between your devices</span>

        </div>}
      />
    );
  };
  const percentage = progress?.phase === "completed" ? 100 : progress?.total
    ? Math.min(100, Math.round((progress.copied / progress.total) * 100)) : 0;

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

        <span className="file-workspace-label">
          <ArrowLeftRight size={17} /> File transfer</span>


        <button
          type="button"
          className="button primary"
          disabled={
            !connected || busy || Boolean(conflict) || destinationBusy || !panes.left.listing
          }
          onClick={() => void uploadFiles(false)}
        >

          <ArrowUpFromLine size={15} /> Upload files
        </button>


        <button
          type="button"
          className="button secondary"
          disabled={
            !connected || busy || Boolean(conflict) || destinationBusy || !panes.left.listing
          }
          onClick={() => void uploadFiles(true)}
        >

          <FolderOpen size={15} /> Upload folder
        </button>


        <span className="spacer" />


        <button
          type="button"
          className="button secondary file-hidden-toggle"
          aria-pressed={showHidden}
          onClick={() => { setShowHidden((old) => !old); setSelected({ left: [], right: [] }); }}
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
              Boolean(conflict) ||
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
              Boolean(conflict) ||
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


      {!progress && !conflict && !error && <div className="file-transfer-idle">
        <ArrowLeftRight size={19} />
        <div>
          <strong>Ready when you are</strong>
          <small>Drag files between panes or drop them from your desktop</small>
        </div>
        <span className="file-shortcut-hint">
          <kbd>Shift</kbd> select a range <kbd>Ctrl A</kbd> select all</span>
      </div>}


      {error && (
        <div
          className="file-error"
          role="alert"
        >

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
        <div
          className="file-conflict"
          role="alert"
        >

          <div>

            <strong>

              {conflict.paths.length}
              {" "}


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
        <div
          className={`file-progress ${progress.phase}`}
          role="status"
        >

          <div className="file-progress-summary">

            {busy ? (
              <LoaderCircle
                size={16}
                className="account-spinner"
              />
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


            <span className="file-progress-percentage">
              {percentage}%</span>


            <span className="file-progress-bytes">

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
            <div
              className="file-progress-path"
              title={progress.current}
            >

              {progress.current}

            </div>
          )}

        </div>
      )}

    </div>
  );
}
