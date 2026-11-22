import { useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent, type KeyboardEvent, type ReactNode } from "react";
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp, ArrowUpToLine, Check, ChevronRight, Code2, Copy, File, FileArchive, FileImage, FileText, Folder, FolderOpen, FolderPlus, HardDrive, Home, LayoutGrid, Link, List, LoaderCircle, Music2, Pencil, RefreshCw, Search, Upload, X } from "lucide-react";
import { api, type FileEntry, type FileListing, type FileStorage } from "../api";
import { copyText } from "../clipboard";
import { errorText } from "../utils";
import { breadcrumbs, fileKind, sizeText, visibleFiles, type FileSort } from "./fileBrowserUtils";
import { createFileDragScroll } from "./fileDragScroll";
import { startFilePointerDrag } from "./filePointerDrag";

export type FileSide = "left" | "right";
export const fileDragType = "application/x-shell-anywhere-files";

type Props = {
  side: FileSide;
  sessionId: string;
  active: boolean;
  storageSessionId: string;
  title: ReactNode;
  subtitle: string;
  ready: boolean;
  disabled: boolean;
  busy: boolean;
  listing: FileListing | null;
  loading: boolean;
  error: string;
  selected: string[];
  showHidden: boolean;
  hoverPath?: string;
  dropStyle: CSSProperties;
  emptyDestination?: ReactNode;
  canTransfer: boolean;
  transferLabel: string;
  dropLabel: string;
  onNavigate: (path: string) => void;
  onSelect: (paths: string[]) => void;
  onCreateFolder: (name: string) => Promise<void>;
  onBrowseLocal?: () => void;
  onTransfer: (paths: string[]) => void;
  onDragOver: (event: DragEvent) => void;
  onDragLeave: (event: DragEvent) => void;
  onDrop: (event: DragEvent) => void;
  onDragEnd: () => void;
  onDragStart: () => void;
  onDragScroll: (x: number, y: number) => void;
};

function FileIcon({ entry }: { entry: FileEntry }) {
  const kind = fileKind(entry);
  const Icon = { folder: Folder, link: Link, code: Code2, image: FileImage, archive: FileArchive, text: FileText, media: Music2, file: File }[kind];
  return <span className={`file-type-icon ${kind}`}>
    <Icon
      size={19}
      strokeWidth={1.7}
    />
  </span>;
}

export default function FilePane({ side, sessionId, active, storageSessionId, title, subtitle, ready, disabled, busy, listing, loading, error, selected, showHidden, hoverPath, dropStyle, emptyDestination, canTransfer, transferLabel, dropLabel, onNavigate, onSelect, onCreateFolder, onBrowseLocal, onTransfer, onDragOver, onDragLeave, onDrop, onDragStart, onDragEnd, onDragScroll }: Props) {
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<FileSort>({ key: "name", descending: false });
  const [view, setView] = useState<"list" | "grid">("list");
  const [editingPath, setEditingPath] = useState(false);
  const [path, setPath] = useState("");
  const [folderName, setFolderName] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [actionError, setActionError] = useState("");
  const [copiedPath, setCopiedPath] = useState("");
  const [dragging, setDragging] = useState(false);
  const [storage, setStorage] = useState<{ sessionId: string; path: string; data: FileStorage } | null>(null);
  const location = `${storageSessionId}:${listing?.path ?? ""}`;
  const [previousLocation, setPreviousLocation] = useState(location);
  const anchor = useRef("");
  const searchInput = useRef<HTMLInputElement>(null);
  const pathButton = useRef<HTMLButtonElement>(null);
  const rows = useRef(new Map<string, HTMLTableRowElement>());
  const fileBody = useRef<HTMLDivElement>(null);
  const dragScroll = useRef<ReturnType<typeof createFileDragScroll> | null>(null);
  const scrollCallback = useRef(onDragScroll);
  const cancelPointerDrag = useRef<(() => void) | null>(null);
  const dragEndCallback = useRef(onDragEnd);
  const entries = useMemo(() => visibleFiles(listing?.entries ?? [], query, showHidden, sort), [listing, query, showHidden, sort]);
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const allSelected = entries.length > 0 && entries.every((entry) => selectedSet.has(entry.path));
  const selectedBytes = listing?.entries.reduce((total, entry) => total + (selectedSet.has(entry.path) && !entry.directory ? entry.size : 0), 0) ?? 0;
  const crumbs = breadcrumbs(listing?.path ?? "");
  const root = listing?.parent === listing?.path;
  const currentStorage = !disabled && storage?.sessionId === storageSessionId && storage?.path === listing?.path ? storage.data : null;
  const usedPercentage = currentStorage?.total ? Math.max(0, Math.min(100, (1 - currentStorage.free / currentStorage.total) * 100)) : 0;

  // Reset folder-specific controls before rendering a different location.
  if (location !== previousLocation) {
    setPreviousLocation(location);
    setQuery("");
    setEditingPath(false);
    setFolderName(null);
    setActionError("");
  }

  useEffect(() => {
    scrollCallback.current = onDragScroll;
    dragEndCallback.current = onDragEnd;
  }, [onDragScroll, onDragEnd]);

  useEffect(() => () => cancelPointerDrag.current?.(), [active, disabled, busy, location]);

  useEffect(() => {
    const element = fileBody.current;
    if (!element || !active || !ready || disabled || busy) return;
    const scrolling = createFileDragScroll(element, (x, y) => scrollCallback.current(x, y));
    dragScroll.current = scrolling;
    const leave = (event: globalThis.DragEvent) => {
      if (!(event.target instanceof Node) || !element.contains(event.target)) scrolling.stop();
    };
    const hide = () => { if (document.hidden) scrolling.stop(); };
    const wheel = (event: WheelEvent) => {
      if (scrolling.wheel(event.deltaX, event.deltaY, event.deltaMode)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };
    element.addEventListener("wheel", wheel, { passive: false });
    window.addEventListener("dragover", leave, true);
    window.addEventListener("drop", scrolling.stop, true);
    window.addEventListener("dragend", scrolling.stop, true);
    window.addEventListener("blur", scrolling.stop);
    document.addEventListener("visibilitychange", hide);
    return () => {
      scrolling.stop();
      dragScroll.current = null;
      element.removeEventListener("wheel", wheel);
      window.removeEventListener("dragover", leave, true);
      window.removeEventListener("drop", scrolling.stop, true);
      window.removeEventListener("dragend", scrolling.stop, true);
      window.removeEventListener("blur", scrolling.stop);
      document.removeEventListener("visibilitychange", hide);
    };
  }, [active, ready, disabled, busy, location]);

  useEffect(() => {
    if (!listing || disabled || !ready) return;
    let stopped = false;
    void api.getFileStorage(storageSessionId, listing.path).then((data) => {
      if (!stopped) setStorage({ sessionId: storageSessionId, path: listing.path, data });
    }).catch(() => {
      if (!stopped) setStorage(null);
    });
    return () => { stopped = true; };
  }, [listing, storageSessionId, disabled, ready]);

  const navigate = (next: string) => {
    if (disabled) return;
    setQuery("");
    setEditingPath(false);
    setFolderName(null);
    setActionError("");
    anchor.current = "";
    onNavigate(next);
  };

  const select = (entry: FileEntry, additive: boolean, range = false) => {
    if (disabled) return;
    const start = entries.findIndex((item) => item.path === anchor.current);
    const end = entries.indexOf(entry);
    if (range && start >= 0) {
      const paths = entries.slice(Math.min(start, end), Math.max(start, end) + 1).map((item) => item.path);
      onSelect(additive ? [...new Set([...selected, ...paths])] : paths);
    } else {
      anchor.current = entry.path;
      onSelect(additive ? selectedSet.has(entry.path) ? selected.filter((item) => item !== entry.path) : [...selected, entry.path] : [entry.path]);
    }
  };

  const editPath = () => {
    setPath(listing?.path ?? "");
    setEditingPath(true);
  };

  const shortcuts = (event: KeyboardEvent<HTMLElement>) => {
    if (disabled || !ready) return;
    const input = (event.target as HTMLElement).closest("input, button");
    const modifier = event.ctrlKey || event.metaKey;
    if (modifier && event.key.toLowerCase() === "l") {
      event.preventDefault();
      editPath();
    } else if (modifier && event.key.toLowerCase() === "f") {
      event.preventDefault();
      searchInput.current?.focus();
    } else if (!input && modifier && event.key.toLowerCase() === "a") {
      event.preventDefault();
      onSelect(entries.map((entry) => entry.path));
    } else if (!input && event.altKey && event.key === "ArrowUp" && listing && !root) {
      event.preventDefault();
      navigate(listing.parent);
    } else if (event.key === "Escape") {
      setFolderName(null);
      setEditingPath(false);
      onSelect([]);
    }
  };

  const copyPath = async (value: string) => {
    try {
      await copyText(value);
      setCopiedPath(value);
      window.setTimeout(() => setCopiedPath((old) => old === value ? "" : old), 1800);
    } catch (reason) {
      setActionError(errorText(reason));
    }
  };

  const changeSort = (key: FileSort["key"]) => setSort((old) => ({ key, descending: old.key === key ? !old.descending : false }));
  const sortButton = (key: FileSort["key"], label: string) => (
    <button
      type="button"
      className={sort.key === key ? "file-sort active" : "file-sort"}
      onClick={() => changeSort(key)}
    >

      {label}
      {sort.key === key && (sort.descending ? <ArrowDown size={12} /> : <ArrowUp size={12} />)}

    </button>
  );

  return (
    <section
      className={`file-pane ${side}${hoverPath !== undefined ? " file-drop-over" : ""}${dragging ? " file-drag-source" : ""}`}
      aria-label={side === "left" ? "Source files" : "Destination files"}
      onKeyDown={shortcuts}
    >

      <div className="file-pane-heading">

        <div className="file-pane-title">
          {title}
        </div>


        <div className="file-pane-subtitle">
          <span className={`file-connection-dot${disabled || !ready ? " muted" : ""}`} />
          {subtitle}
        </div>

      </div>


      {ready && <>

        <div className="file-location">

          <button
            type="button"
            className="icon-button"
            aria-label={`${side} parent folder`}
            title="Parent folder (Alt+Up)"
            disabled={disabled || !listing || root}
            onClick={() => listing && navigate(listing.parent)}
          >
            <ArrowUpToLine size={16} />
          </button>


          <button
            type="button"
            className="icon-button"
            aria-label={`${side} home folder`}
            title="Home folder"
            disabled={disabled}
            onClick={() => navigate("")}
          >
            <Home size={16} />
          </button>


          {editingPath ? <form
            className="file-path-form"
            onSubmit={(event) => { event.preventDefault(); navigate(path); }}
          >

            <input
              autoFocus
              aria-label={`${side} path`}
              value={path}
              disabled={disabled}
              spellCheck={false}
              onChange={(event) => setPath(event.target.value)}
              onKeyDown={(event) => { if (event.key === "Escape") { event.stopPropagation(); setEditingPath(false); pathButton.current?.focus(); } }}
            />


            <button
              type="submit"
              className="icon-button"
              aria-label={`Go to ${side} path`}
              disabled={disabled}
            >
              <ArrowRight size={15} />
            </button>

          </form> : <nav
            className="file-breadcrumbs"
            aria-label={`${side} folder path`}
          >

            {crumbs.map((crumb, index) => <span key={crumb.path}>

              {index > 0 && <ChevronRight size={12} />}


              <button
                type="button"
                disabled={disabled}
                title={crumb.path}
                aria-current={index === crumbs.length - 1 ? "location" : undefined}
                onClick={() => navigate(crumb.path)}
              >
                {crumb.name}
              </button>

            </span>)}


            {!listing && <span className="file-path-placeholder">
              {loading ? "Opening folder" : "No folder open"}
            </span>}

          </nav>}


          {!editingPath && <button
            ref={pathButton}
            type="button"
            className="icon-button file-edit-path"
            aria-label={`Edit ${side} path`}
            title="Edit path (Ctrl+L)"
            disabled={disabled}
            onClick={editPath}
          >
            <Pencil size={14} />
          </button>}


          {onBrowseLocal && <button
            type="button"
            className="icon-button"
            title="Choose local folder"
            aria-label="Choose local folder"
            disabled={disabled}
            onClick={onBrowseLocal}
          >
            <FolderOpen size={16} />
          </button>}


          <button
            type="button"
            className="icon-button"
            aria-label={`Refresh ${side} files`}
            title="Refresh files"
            disabled={disabled || loading}
            onClick={() => onNavigate(listing?.path ?? "")}
          >
            <RefreshCw
              size={15}
              className={loading ? "account-spinner" : ""}
            />
          </button>

        </div>


        <div className="file-pane-tools">

          <label className="file-search">
            <Search size={15} />
            <input
              ref={searchInput}
              aria-label={`Search ${side} files`}
              placeholder="Filter files"
              value={query}
              disabled={disabled}
              onChange={(event) => { setQuery(event.target.value); onSelect([]); }}
              onKeyDown={(event) => { if (event.key === "Escape") { event.stopPropagation(); setQuery(""); } }}
            />
            {query && <button
              type="button"
              className="icon-button"
              aria-label={`Clear ${side} search`}
              onClick={() => { setQuery(""); searchInput.current?.focus(); }}
            >
              <X size={13} />
            </button>}
          </label>


          <button
            type="button"
            className="icon-button"
            aria-label={`New ${side} folder`}
            title="New folder"
            disabled={disabled || busy || !listing}
            onClick={() => { setFolderName(""); setActionError(""); }}
          >
            <FolderPlus size={16} />
          </button>


          <div
            className="file-view-switch"
            role="group"
            aria-label={`${side} file view`}
          >

            <button
              type="button"
              className="icon-button"
              aria-label={`${side} list view`}
              title="List view"
              aria-pressed={view === "list"}
              onClick={() => setView("list")}
            >
              <List size={16} />
            </button>


            <button
              type="button"
              className="icon-button"
              aria-label={`${side} grid view`}
              title="Grid view"
              aria-pressed={view === "grid"}
              onClick={() => setView("grid")}
            >
              <LayoutGrid size={15} />
            </button>

          </div>

        </div>

      </>}


      {folderName !== null && <form
        className="file-new-folder"
        onSubmit={async (event) => { event.preventDefault(); if (creating || disabled || !folderName.trim()) return; setCreating(true); setActionError(""); try { await onCreateFolder(folderName.trim()); setFolderName(null); } catch (reason) { setActionError(errorText(reason)); } finally { setCreating(false); } }}
      >

        <FolderPlus size={17} />
        <input
          autoFocus
          aria-label="Folder name"
          placeholder="New folder name"
          value={folderName}
          disabled={creating || disabled}
          onChange={(event) => setFolderName(event.target.value)}
          required
        />


        <button
          type="submit"
          className="button secondary"
          disabled={creating || disabled || !folderName.trim()}
        >
          {creating ? <LoaderCircle
            size={14}
            className="account-spinner"
          /> : "Create"}
        </button>


        <button
          type="button"
          className="icon-button"
          aria-label="Cancel new folder"
          disabled={creating}
          onClick={() => setFolderName(null)}
        >
          <X size={15} />
        </button>

      </form>}


      {(error || actionError) && <div
        className="file-pane-error"
        role="alert"
      >
        {error || actionError}
      </div>}


      <div
        ref={fileBody}
        className="file-pane-body"
        data-file-session={sessionId}
        data-file-side={side}
        style={dropStyle}
        onDragOver={(event) => {
          onDragOver(event);
          if (event.defaultPrevented) dragScroll.current?.update(event.clientX, event.clientY);
          else dragScroll.current?.stop();
        }}
        onDragLeave={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node)) dragScroll.current?.stop();
          onDragLeave(event);
        }}
        onDrop={(event) => { dragScroll.current?.stop(); onDrop(event); }}
        tabIndex={ready ? 0 : undefined}
        aria-label={`${side} files`}
        aria-busy={loading}
      >

        {!ready ? emptyDestination : <>

          {loading && listing && <div className="file-loading-line" />}


          <table
            className={`file-table${view === "grid" ? " file-grid" : ""}`}
            role="grid"
            aria-multiselectable="true"
            aria-label={`${side} file list`}
          >

            <thead>
              <tr>

                <th aria-sort={sort.key === "name" ? sort.descending ? "descending" : "ascending" : "none"}>
                  <div className="file-name-heading">
                    <input
                      type="checkbox"
                      aria-label={`Select all ${side} files`}
                      checked={allSelected}
                      ref={(element) => { if (element) element.indeterminate = selected.length > 0 && !allSelected; }}
                      disabled={disabled || !entries.length}
                      onChange={() => onSelect(allSelected ? [] : entries.map((entry) => entry.path))}
                    />
                    {sortButton("name", "Name")}
                  </div>
                </th>


                <th aria-sort={sort.key === "size" ? sort.descending ? "descending" : "ascending" : "none"}>
                  {sortButton("size", "Size")}
                </th>


                <th aria-sort={sort.key === "modified" ? sort.descending ? "descending" : "ascending" : "none"}>
                  {sortButton("modified", "Modified")}
                </th>

              </tr>
            </thead>


            <tbody>
              {entries.map((entry, index) => <tr key={entry.path} ref={(element) => { if (element) rows.current.set(entry.path, element); else rows.current.delete(entry.path); }} data-file-directory={entry.directory ? entry.path : undefined} className={`${selectedSet.has(entry.path) ? "selected" : ""}${hoverPath === entry.path ? " drop-folder" : ""}${entry.name.startsWith(".") ? " file-hidden" : ""}`} tabIndex={disabled ? -1 : 0} aria-selected={selectedSet.has(entry.path)} draggable={false} onClick={(event) => select(entry, event.ctrlKey || event.metaKey, event.shiftKey)} onDoubleClick={() => { if (entry.directory) navigate(entry.path); }} onKeyDown={(event) => {
                if (event.target !== event.currentTarget) return;
                if (event.key === "Enter" && entry.directory) { event.preventDefault(); navigate(entry.path); }
                if (event.key === " ") { event.preventDefault(); select(entry, true, event.shiftKey); }
                if (event.key === "ArrowDown" || event.key === "ArrowUp") { event.preventDefault(); const next = entries[index + (event.key === "ArrowDown" ? 1 : -1)]; if (next) { rows.current.get(next.path)?.focus(); select(next, event.ctrlKey || event.metaKey, event.shiftKey); } }
              }} onPointerDown={(event) => {
                if (event.button !== 0 || event.pointerType === "touch" || disabled || busy || entry.symlink) return;
                if ((event.target as Element).closest("input, button")) return;
                cancelPointerDrag.current?.();
                event.preventDefault();
                event.currentTarget.focus({ preventScroll: true });
                const files = selectedSet.has(entry.path) ? selected : [entry.path];
                cancelPointerDrag.current = startFilePointerDrag(event.currentTarget, event.nativeEvent, {
                  type: fileDragType,
                  data: JSON.stringify({ sessionId, side, paths: files }),
                  label: files.length === 1 ? entry.name : `${files.length} items`,
                  count: files.length,
                  onStart: () => {
                    if (!selectedSet.has(entry.path)) onSelect([entry.path]);
                    setDragging(true);
                    onDragStart();
                  },
                  onEnd: () => {
                    cancelPointerDrag.current = null;
                    setDragging(false);
                    dragEndCallback.current();
                  },
                });
              }} onDragStart={(event) => event.preventDefault()}>

                <td>
                  <div className="file-name">
                    <input
                      type="checkbox"
                      aria-label={`Select ${entry.name}`}
                      checked={selectedSet.has(entry.path)}
                      disabled={disabled}
                      onClick={(event) => event.stopPropagation()}
                      onDoubleClick={(event) => event.stopPropagation()}
                      onChange={() => select(entry, true)}
                    />
                    <FileIcon entry={entry} />
                    <span
                      className="file-entry-name"
                      title={entry.name}
                    >
                      {entry.name}
                    </span>
                    <div className="file-row-actions">

                      <button
                        type="button"
                        className="icon-button"
                        title={copiedPath === entry.path ? "Path copied" : "Copy path"}
                        aria-label={`Copy path for ${entry.name}`}
                        onClick={(event) => { event.stopPropagation(); void copyPath(entry.path); }}
                        onDoubleClick={(event) => event.stopPropagation()}
                      >
                        {copiedPath === entry.path ? <Check size={14} /> : <Copy size={14} />}
                      </button>


                      {entry.directory ? <button
                        type="button"
                        className="icon-button"
                        title="Open folder"
                        aria-label={`Open ${entry.name}`}
                        disabled={disabled}
                        onClick={(event) => { event.stopPropagation(); navigate(entry.path); }}
                        onDoubleClick={(event) => event.stopPropagation()}
                      >
                        <ChevronRight size={15} />
                      </button> : <button
                        type="button"
                        className="icon-button"
                        title={transferLabel}
                        aria-label={`${transferLabel} ${entry.name}`}
                        disabled={!canTransfer || entry.symlink}
                        onClick={(event) => { event.stopPropagation(); onTransfer([entry.path]); }}
                        onDoubleClick={(event) => event.stopPropagation()}
                      >
                        {side === "left" ? <ArrowRight size={15} /> : <ArrowLeft size={15} />}
                      </button>}

                    </div>
                  </div>
                  <span className="file-grid-detail">
                    {entry.directory ? "Folder" : sizeText(entry.size)}
                  </span>
                </td>


                <td>
                  {entry.directory ? "—" : sizeText(entry.size)}
                </td>
                <td title={entry.modified > 0 ? new Date(entry.modified * 1000).toLocaleString() : undefined}>
                  {entry.modified > 0 ? new Date(entry.modified * 1000).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" }) : "—"}
                </td>

              </tr>)}
            </tbody>

          </table>


          {entries.length === 0 && <div className="file-empty">
            {loading ? <LoaderCircle
              size={30}
              className="account-spinner"
            /> : query ? <Search size={30} /> : <FolderOpen size={32} />}
            <strong>
              {disabled ? "Connection paused" : loading ? "Opening folder" : query ? "No matching files" : "This folder is empty"}
            </strong>
            <span>
              {disabled ? "Reconnect to browse remote files" : loading ? "Getting everything ready" : query ? `Nothing matches “${query}” in this folder` : "Drag files here to start a transfer"}
            </span>
            {query && <button
              type="button"
              className="button secondary"
              onClick={() => setQuery("")}
            >Clear search</button>}
          </div>}

        </>}

      </div>


      {ready && <div className={`file-pane-footer${hoverPath !== undefined ? " is-drop-target" : selected.length ? " has-selection" : ""}`}>

        {hoverPath !== undefined ? <div className="file-drop-indicator" role="status">
          {dropLabel === "Upload" ? <Upload size={18} /> : dropLabel === "Download" ? <ArrowDown size={18} /> : <Copy size={18} />}
          <div className="file-drop-description">
            <strong>{hoverPath !== listing?.path ? `${dropLabel} into folder` : `Drop files to ${dropLabel.toLowerCase()}`}</strong>
            <span title={hoverPath}>{hoverPath}</span>
          </div>
        </div> : selected.length ? <>
          <span className="file-selection-count">
            {selected.length} selected{selectedBytes > 0 && <small>
              {sizeText(selectedBytes)}
            </small>}
          </span>
          <button
            type="button"
            className="icon-button"
            aria-label={`Clear ${side} selection`}
            title="Clear selection (Esc)"
            onClick={() => onSelect([])}
          >
            <X size={14} />
          </button>
          <button
            type="button"
            className="button file-selection-transfer"
            disabled={!canTransfer}
            onClick={() => onTransfer(selected)}
          >
            {transferLabel}
            {side === "left" ? <ArrowRight size={14} /> : <ArrowLeft size={14} />}
          </button>
        </> : <>
          <span>
            {entries.length}

            {entries.length === 1 ? "item" : "items"}
            {query && " found"}
          </span>
          <span className="file-footer-hint">Drag to transfer · Double-click to open</span>
        </>}

      </div>}


      {ready && <div
        className={`file-storage${currentStorage?.available && usedPercentage > 90 ? " low-space" : ""}`}
        title={currentStorage?.available ? `${sizeText(currentStorage.free)} available to your account on this filesystem` : storageSessionId ? "This SFTP server does not report filesystem capacity, or storage information is unavailable" : "Storage information is unavailable"}
      >

        <HardDrive size={13} />
        <span>
          {currentStorage?.available ? <>
            <strong>
              {sizeText(currentStorage.free)}
            </strong> free <span className="file-storage-total">of {sizeText(currentStorage.total)}
            </span>
          </> : "Storage unavailable"}
        </span>


        {currentStorage?.available && <span
          className="file-storage-meter"
          role="meter"
          aria-label={`${side} storage used`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(usedPercentage)}
        >
          <span style={{ width: `${usedPercentage}%` }} />
        </span>}

      </div>}

    </section>
  );
}
