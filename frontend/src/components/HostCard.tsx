import {
  ArrowRight,
  FolderOpen,
  MoreHorizontal,
  Server,
  Star,
} from "lucide-react";
import type { MouseEvent } from "react";
import type { Host } from "../api";

type Props = {
  host: Host;
  connecting: boolean;
  onConnect: (host: Host) => void;
  onFiles: (host: Host) => void;
  onEdit: (host: Host) => void;
  onToggleFavorite: (host: Host) => void;
};

export default function HostCard({
  host,
  connecting,
  onConnect,
  onFiles,
  onEdit,
  onToggleFavorite,
}: Props) {
  const handleDoubleClick = (event: MouseEvent<HTMLDivElement>) => {
    if (connecting || (event.target as HTMLElement).closest("button")) return;
    onConnect(host);
  };

  return (
    <div
      className="host-card"
      onDoubleClick={handleDoubleClick}
      title="Double-click to connect"
    >
      <div className="host-card-top">
        <div className="host-card-top-name">
          <div className="host-icon">
            <Server size={20} />
          </div>
          <h3>{host.name}</h3>
        </div>

        <div className="host-actions">
          <button
            className={host.favorite ? "icon-button favorite" : "icon-button"}
            title={host.favorite ? "Remove from favorites" : "Add to favorites"}
            onClick={() => onToggleFavorite(host)}
          >
            <Star size={16} fill={host.favorite ? "currentColor" : "none"} />
          </button>
          <button
            className="icon-button"
            title="Edit host"
            onClick={() => onEdit(host)}
          >
            <MoreHorizontal size={18} />
          </button>
        </div>
      </div>

      <div className="host-address">
        {host.username}@{host.address}:{host.port}
      </div>
      <div className="host-card-bottom">
        <span>
          <span className="host-status-dot" />
          {host.authMethod === "key" ? "KEY" : "PASSWORD"}
        </span>
        <div className="host-connect-actions">
          <button
            type="button"
            disabled={connecting}
            onClick={() => onFiles(host)}
            title="Browse files with SFTP"
          >
            <FolderOpen size={15} /> SFTP
          </button>
          <button disabled={connecting} onClick={() => onConnect(host)}>
            {connecting ? "Connecting…" : "SSH"} <ArrowRight size={15} />
          </button>
        </div>
      </div>
    </div>
  );
}
