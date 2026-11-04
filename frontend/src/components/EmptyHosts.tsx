import { Plus, Wifi } from "lucide-react";

type Props = {
  query: string;
  selectedGroup: string;
  totalHosts: number;
  onNewHost: () => void;
};

export default function EmptyHosts({
  query,
  selectedGroup,
  totalHosts,
  onNewHost,
}: Props) {
  const firstHost = selectedGroup === "All hosts" && totalHosts === 0;
  return (
    <div className="empty-layout">
      <div className="empty-card">
        <div className="empty-illustration">
          <div className="illus-window">
            <div className="illus-title">
              <i />
              <i />
              <i />
              <span>terminal</span>
            </div>
            <div className="illus-body">
              <span className="green">➜</span> <span className="muted">~</span>{" "}
              ssh your-next-server<span className="cursor">_</span>
            </div>
          </div>
          <div className="illus-orbit orbit-one" />
          <div className="illus-orbit orbit-two" />
        </div>
        <h2>
          {query
            ? "No matching hosts"
            : firstHost
              ? "Your servers, one place."
              : "Nothing here yet"}
        </h2>
        <p>
          {query
            ? "Try a different search term."
            : firstHost
              ? "Add your first host to open a secure terminal in seconds."
              : "Add a host to this group or choose another view."}
        </p>
        <button className="button primary" onClick={onNewHost}>
          <Plus size={17} /> Add a host
        </button>
      </div>
      <div className="getting-started p-6">
        <div className="getting-started-tt">
            <div className="guide-icon">
              <Wifi size={18} />
            </div>
            <h3>Ready when you are</h3>
        </div>
        <p>
          Save a host once, then connect in a click. Your SSH sessions open in
          tabs right here.
        </p>
        <div className="guide-divider" />
        <div className="guide-step">
          <span>01</span>
          <div>
            <strong>Add your server</strong>
            <small>Hostname, username, and authentication</small>
          </div>
        </div>
        <div className="guide-step">
          <span>02</span>
          <div>
            <strong>Verify its identity</strong>
            <small>Confirm the SSH host fingerprint</small>
          </div>
        </div>
        <div className="guide-step">
          <span>03</span>
          <div>
            <strong>Start working</strong>
            <small>A full interactive terminal awaits</small>
          </div>
        </div>
      </div>
    </div>
  );
}
