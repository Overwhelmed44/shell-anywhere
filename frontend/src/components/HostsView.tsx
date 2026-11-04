import { Activity, Plus, Search, Trash2, Pencil } from "lucide-react";
import { isDesktop, type Host } from "../api";
import EmptyHosts from "./EmptyHosts";
import HostCard from "./HostCard";

type Props = {
  hosts: Host[];
  filteredHosts: Host[];
  selectedGroup: string;
  groupName: string;
  onEditGroup: () => void;
  isGroup: boolean;
  query: string;
  visible: boolean;
  onQueryChange: (query: string) => void;
  onNewHost: () => void;
  onNewGroup: () => void;
  onDeleteGroup: (name: string) => void;
  onConnect: (host: Host) => void;
  onFiles: (host: Host) => void;
  connectingHostId: string | null;
  onEdit: (host: Host) => void;
  onToggleFavorite: (host: Host) => void;
};

export default function HostsView({
  hosts,
  filteredHosts,
  selectedGroup,
  groupName,
  onEditGroup,
  isGroup,
  query,
  visible,
  onQueryChange,
  onNewHost,
  onNewGroup,
  onDeleteGroup,
  onConnect,
  onFiles,
  connectingHostId,
  onEdit,
  onToggleFavorite,
}: Props) {
  return (
    <div className={visible ? "hosts-view" : "hosts-view hidden"}>
      <div className="page-head">
        <div>
          <div className="eyebrow">YOUR CONNECTIONS</div>
          <h1>{selectedGroup === "All hosts" ? "Hosts" : groupName}</h1>
          <p>Everything you need to stay connected to your servers</p>
        </div>
        <div className="page-actions">
          {isGroup && (
            <button className="button secondary" onClick={onEditGroup}>
              <Pencil size={16} /> Edit group
            </button>
          )}
          {isGroup && (
            <button
              className="button secondary delete-group-button"
              onClick={() => onDeleteGroup(selectedGroup)}
            >
              <Trash2 size={16} /> Delete group
            </button>
          )}
          {!isGroup && (
            <button
              className="button secondary add-button"
              onClick={onNewGroup}
            >
              <Plus size={17} /> New group
            </button>
          )}
          <button className="button primary add-button" onClick={onNewHost}>
            <Plus size={17} /> New host
          </button>
        </div>
      </div>
      <div className="toolbar">
        <div className="search">
          <Search size={18} />
          <input
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="Search hosts, users, or addresses"
            aria-label="Search hosts"
          />
        </div>
        <span className="host-total">
          {filteredHosts.length} {filteredHosts.length === 1 ? "host" : "hosts"}
        </span>
      </div>
      {!isDesktop && (
        <div className="notice px-4 py-3 font-sm">
          <Activity size={17} /> Preview mode. Run the Wails desktop app to save
          hosts and connect.
        </div>
      )}
      {filteredHosts.length === 0 ? (
        <EmptyHosts
          query={query}
          selectedGroup={selectedGroup}
          totalHosts={hosts.length}
          onNewHost={onNewHost}
        />
      ) : (
        <div className="host-grid">
          {filteredHosts.map((host) => (
            <HostCard
              key={host.id}
              host={host}
              onConnect={onConnect}
              onFiles={onFiles}
              connecting={connectingHostId === host.id}
              onEdit={onEdit}
              onToggleFavorite={onToggleFavorite}
            />
          ))}
        </div>
      )}
    </div>
  );
}
