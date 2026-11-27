# Shell Anywhere

A desktop SSH client built with Go, Wails, React, and xterm.js.

## Features

- Organize Personal workspace hosts into groups, search connections, and mark favorites. Groups have stable IDs, can be renamed, and can supply a default identity. Deleting a group keeps its hosts and preserves an inherited identity as a direct identity reference. New hosts default to user `root`; a blank display name uses the hostname or IP
- Manage reusable **Identities** from the sidebar. Each identity contains a username and password or SSH private key, with an optional passphrase. New hosts inherit their group's default identity unless separate credentials or another identity are explicitly chosen. Updating an identity affects future connections from every host using it. Deleting an identity asks for confirmation in the sidebar, keeps dependent hosts' credentials separately, and clears group defaults referencing it
- Create and edit hosts, groups, and identities in a right sidebar while keeping the workspace visible. Only one editor can be open at a time; opening another replaces the current editor. Settings show the saved SSH password as visible, editable text. Hosts using an identity show its password read-only; edit the shared password in identity settings
- Connect with a password or a local private key, including encrypted keys.
- Verify a new server's SSH host-key fingerprint before trusting it. If a saved key changes, a dialog shows the saved and new fingerprints and offers **Overwrite key & connect** or **Abort connection**. Replacement requires explicit confirmation of the presented key and is saved after successful SSH authentication
- Use interactive terminal tabs with automatic resizing.
- Copy selected terminal text with **Ctrl+C**, **Ctrl+Shift+C**, or **right-click → Copy**. With no selection, Ctrl+C still interrupts the running command. Paste with **Ctrl+V**, **Ctrl+Shift+V**, **Shift+Insert**, or **right-click → Paste**.
- Drag an inactive terminal tab onto the open terminal's left, right, top, or bottom edge to split the workspace. The two tabs become one group tab, and you can keep multiple groups open. Use a group's split icon to restore its separate tabs, or its close button to close both sessions. Drag the divider to resize the panes.
- When an SSH connection ends, use **Reconnect** in its terminal header. The existing tab, terminal history, and split group stay in place. Connections use saved credentials. Connection failures show an error without opening a password override dialog; change passwords in host or identity settings
- Open local terminals: PowerShell and Command Prompt on Windows, or your login shell on macOS and Linux. The menu lists the shells available on the current platform
- Open **Settings** beside **Sign in**, or click your username when signed in. Settings opens as a full page with appearance, terminal font size, scrollback, cursor blinking, and the SSH/SFTP connection timeout. Use **Back to workspace** to return to your open sessions without disconnecting them. Choose Dark, Midnight, Forest, Rose, Arctic, Light, Sand, or Paper for matching interface and terminal colors. Save changes to apply them; preferences are also synced when cloud syncing is enabled. The timeout covers connecting and authenticating, without limiting established sessions
- Save SSH passwords and successfully used private-key passphrases in the operating system's credential store, and include them when syncing
- Store hosts and trusted host keys in `shell-anywhere` inside the user's config directory. Existing settings move automatically from the old directory, including restored private keys

## SSH session extensions

Each SSH connection has **Terminal**, **Server load** and **Docker** views. Both extensions are currently enabled for every host, with no add/remove menu; old per-host extension selections are ignored. Server load is one extension with a full dashboard: CPU, memory, system details, storage, network and disk I/O appear together in responsive cards. Switch views without opening another connection or losing terminal output

**Split view** places the terminal on the left and the selected extension on the right within the same connection. Drag the divider to resize, use its arrow keys for small adjustments (Shift for larger steps), or double-click to reset to equal widths. Selecting another extension while split keeps the terminal visible. **Terminal** returns to a full terminal view; **Exit split** expands the last focused pane. Each connection keeps its current view and split width when switching connection tabs

The dashboard includes CPU usage, load averages, uptime, hostname, OS and kernel. Memory includes available RAM, reclaimable cache and swap. Storage reports filesystem usage and available space. Network shows receive/send rates and error counters per interface, with the default route identified. Disk I/O shows read/write throughput, IOPS and device busy time for whole devices. CPU and memory charts use a fixed percentage scale; network and I/O charts have throughput scales and selectors for a specific interface or device, with separate receive/send and read/write histories

Live statistics currently support Linux SSH hosts. The app reads standard system counters over a separate exec channel on the existing SSH connection; it installs no agent, requests no root permissions, and does not send commands to the interactive terminal. Sampling runs about every three seconds only while the SSH tab and application window are visible and Server load is enabled. CPU and throughput rates need two samples; rebooted or reset counters establish a new baseline

An unavailable counter or denied exec channel is shown in the extension without interrupting the terminal. Metrics requests have an eight-second timeout, bounded output, and one request at a time per SSH session. Refresh retries a failed or unsupported reading

### Docker

Open **Docker** from the SSH connection's view bar. The extension checks for the Docker CLI and access to the engine as the connected SSH user. A server without Docker shows an unavailable view with **Check again**; a stopped engine or insufficient permissions shows Docker's error and offers a retry. The app does not install Docker, elevate privileges, or require Docker on the client computer. Remote hosts must support a POSIX shell and have the Docker CLI available in the noninteractive SSH user's PATH. Docker uses that user's configured context and environment, including rootless setups

The **Containers** view lists running and stopped containers by name with status/health text and direct **Logs** and **Exec** buttons. Images and Compose project/service labels are hidden. Search by container name or ID and filter by state. Select a container name to see ports, timestamps, exit code, restart policy, mounts and network addresses. The single **Containers / Metrics** switch at the top changes the whole list to CPU, memory, network I/O and disk I/O readings. Click the **Memory** header or any memory value to switch the whole column between absolute usage/limit and Docker's reported memory percentage. Resource usage is collected only in the Metrics view

**Logs**, **Exec** and **Details** replace the container list with a full view inside the Docker extension. **Back** returns to the list with its search and state filter preserved. Logs and the exec terminal fill the available space instead of splitting it with the list. Logs have their own scroll area with mouse wheel, scrollbar and keyboard scrolling

**Logs** reads the latest 200 lines from stdout and stderr with timestamps; output is capped at the latest 256 KiB. **Exec** opens an interactive `sh` terminal inside a running container over a separate channel on the existing SSH connection. After the shell exits, choose `sh` or `bash` and reconnect; the chosen shell must be installed in the container. Input, resizing, clipboard shortcuts and themes use the normal terminal component. Switching between Details, Logs and Metrics or returning to the list keeps the selected container's shell alive; selecting a different container closes its exec channel. **End session** closes only the container shell and leaves the server terminal connected. Environment variables are not included in the details view

Controls support **Start**, **Stop**, **Restart**, **Pause** and **Resume** according to container state. Stopped containers can be removed after confirmation. Removal does not force-stop a container or remove its volumes or image; the container's writable layer is deleted. Docker commands run on separate exec channels over the existing SSH connection, with bounded responses and request timeouts. After a timed out action, refresh to check whether it completed on the server

Docker refreshes five seconds after each completed read while its view and the application window are visible. Switching to the terminal, another extension or another connection pauses Docker polling. Missing or inaccessible Docker stops polling until **Check again** is selected. Docker can be used full-width or beside the terminal in **Split view**

## SFTP file transfers

Click **SFTP** on a saved host to open an SFTP split group. The clicked host appears on the left, starting at `/`. The right pane starts with a destination chooser. SFTP uses the host's saved identity or separate credentials and the same trusted server keys as SSH, without starting a terminal

In the right pane, choose **This computer** or another saved host. Choosing a host opens a separate SFTP connection in that pane, starting at `/`, and asks for credentials or server-key confirmation when needed. Drag files or folders either way to copy between the hosts. Host-to-host transfers stream through the app without storing a local copy. Switching back to **This computer** closes the extra connection and returns to local files. Drag the divider to resize the two sides; each split group retains its destination and layout when switching tabs

Drag files or folders between the two panes to copy them, or select items and use the arrows between the panes. Dropping onto a folder copies into that folder; dropping onto the pane's empty area uses its current path. Folders are copied recursively, including empty folders and hidden files. Symbolic links and special files are not copied

You can also drop files and folders from Windows Explorer onto either pane displaying a remote host. **Upload files** and **Upload folder** upload from this computer to the host in the left pane. To download, choose **This computer** in the right pane; dragging remote files directly into Explorer is not supported

Double-click folders to open them, click a breadcrumb, or edit the path with the pencil button or **Ctrl+L**. Each pane has its own filename filter, sortable columns, and list/grid view. Use **Ctrl+F** to focus its filter, **Shift** to select a range, **Ctrl+A** to select visible items, and **Esc** to clear selection. Hover a row to copy its path, open a folder, or transfer a file. Use **Hidden files** to show dotfiles. Downloads go to the local folder currently open in the right pane

Dragging items shows a file-count preview and highlights the destination, including the specific folder under the pointer. Use the mouse wheel or trackpad while dragging between panes to scroll the file list under the cursor. Hold near the top or bottom of a file list to scroll while dragging; scrolling speeds up closer to the edge and also works for files dragged from the desktop. Each pane reports free and total capacity for its current filesystem, with a usage meter and a low-space warning. Capacity refreshes when browsing, refreshing, or completing a transfer. Remote capacity requires the server's OpenSSH statvfs extension; unavailable statistics do not affect file operations

Transfers show their progress and can be cancelled. Existing files require confirmation before replacement. Each file is written to a temporary file before replacing its destination, so cancelling a file midway preserves the existing destination; files already completed and folders already created remain. Closing the SFTP split group ends both connections and stops any active transfer. Each disconnected host can reconnect; use the reconnect icon beside the right pane's selector for the other host

## Optional Oxee account

Use **Sign in** in the top-right navigation or **Sign in with Oxee** in the sidebar to open the system browser. Sign in or create an account there, then return to Shell Anywhere. Your username replaces the navigation sign-in button, and the sidebar sign-in card disappears. Click your username, open **Account & sync**, and select **Sign out** to disconnect the account. An existing browser login can continue with the displayed account or select another one. Login is optional: hosts, SSH sessions, and local terminals work without an Oxee account and while offline.

After signing in, optionally enable **Cloud syncing**. A vault password is optional: leave it blank to sync hosts, groups, favorites, saved SSH passwords, private-key contents, saved key passphrases, and trusted server keys through your Oxee account. Sign in on another device to restore ready-to-use connections without a separate password

App settings sync as a separate JSON object of custom overrides alongside your connection vault. Only values that differ from the built-in defaults are saved in `settings.json` and sent to the server; the app overlays them on its defaults when loading. For example, changing only the theme saves `{ "theme": "forest" }`. Restoring defaults and saving writes `{}` and clears synced overrides. Explicit overrides such as `terminalCursorBlink: false` are preserved. On first setup, existing cloud settings are restored; if none exist, this device uploads its preferences. Later local changes upload automatically, including changes made offline. Settings are stored separately from the encrypted vault and use your Oxee account for access.

Adding a vault password enables end-to-end encryption using AES-256-GCM and an Argon2id-derived key. Enter that same password on a new device to unlock a protected vault. The derived vault key is remembered in the OS credential store under `shell-anywhere-vault`; the vault password and derived key are never sent to the server. Existing protected vaults still require their password or a saved key

Restored passwords and key passphrases go into the new device's OS credential store. Private keys are restored into the app's configuration directory under `keys/`, with device-local paths and owner-only file permissions on systems that support them. Passwords and key contents never appear in `hosts.json` or `vault.json`. A key passphrase is remembered after a successful SSH authentication; keys that have never been unlocked in this app still need their passphrase once. Oxee login sessions remain device-specific

The vault JSON and its envelope both use format version `1`. Without a vault password, the envelope contains the complete vault in `data` with `protected: false`; access is managed by your authenticated Oxee account. With a password, the complete vault, including SSH credentials, is encrypted in `encrypted` with `protected: true` and a `salt`

The vault document contains `workspaces.personal` and an `workspaces.others` map reserved for future workspace support. Within each workspace, `hosts`, `groups`, and `identities` are maps keyed by stable IDs. IDs are not repeated inside their values. Hosts reference groups through `groupId` and identities through `identityId`, or set `useGroupIdentity` to inherit `defaultIdentityId` from their group. Credentials are stored once in an identity; hosts with separate credentials store their own. `knownHosts` belongs to its workspace. See [vault.example.json](vault.example.json) for a complete example

Only Personal is exposed in the current interface. Creation, sharing, invitations, and transferring hosts or groups between workspaces are deferred until the workspace API is available. This change does not add workspace creation endpoints or migrations

Open your username → **Account & sync** → **Cloud syncing** to set up syncing, sync now, or pause syncing. Changes upload automatically and the app checks the cloud every minute. Whole vaults replace earlier versions without merging; local edits take priority on the next upload. If local and cloud lists differ during setup, choose which to use. Before replacing local hosts the app saves `hosts-before-sync.json`. **Not now** keeps everything local. Signing out stops syncing.

The provider is reached at `https://shell-aw.oxee.net/api` through the platform Nginx proxy. Login remains on the configured Oxee origin. Set `SHELLANYWHERE_PROVIDER_URL` to override the full provider API base for development or another deployment, for example `https://shell-aw.oxee.localhost/api`. Nginx forwards `/api/...` unchanged to the provider's Docker network alias `shell-aw`. Run the provider's `docker-compose.dev.yaml` alongside the platform, with a `shell-aw` bucket available in its configured S3 storage. The app uses msar for provider requests with Authorization and saves rotated refresh sessions. Sync metadata in `vault.json` is scoped to the signed-in account and server. All entities use `POST /api/v1/sync` for checks and `PUT /api/v1/sync` for uploads; the provider creates the account's sync record automatically, so no vault id or creation request is needed.

The app binds a temporary listener to `127.0.0.1` on an available port. Oxee stores that port in the pending request and returns a one-time authorization code to it after login/signup. Shell Anywhere accepts the callback only at `/oauth/<rid>` for its active request and exchanges the code with that request id. The server stores the code hash in `OAuthRequest.code` and deletes the request after a successful exchange. A separate `state` parameter is not used; rejected callbacks and failed exchanges leave the listener available for the valid callback. The listener closes on completion, cancellation, or after ten minutes. Refresh tokens stay in the operating system credential store under `shell-anywhere-oxee`, independently of SSH passwords and the browser session. They are never passed to the React UI or written to host configuration files.

SSH passwords and key passphrases use the credential-store service `shell-anywhere-ssh`. Existing login and SSH credentials migrate when read, and sign-out or credential deletion clears both the current and old entries. The saved theme also migrates automatically

The browser stays on Oxee and sends the callback in the background. The local listener permits CORS only from the configured Oxee origin and acknowledges success with the account identity after saving the session. It never returns refresh tokens to the browser. Oxee then shows an animated check, the connected account, and a message that the tab can be closed. If the local app cannot be reached or browser permission is denied, the page offers a retry without repeating login/signup. Some browsers request permission to connect to the local app.

**Sign out** removes the saved local login and attempts to revoke its server session when Oxee is reachable. For a local platform deployment, set `SHELLANYWHERE_OXEE_URL` before launching the app. The built-in default is `https://oxee.net` for all builds. `make dev` supplies `https://oxee.localhost` through the environment override. Overrides must use HTTPS, except loopback HTTP addresses for development. Certificate validation is skipped for `localhost`, its `.localhost` subdomains, and loopback IP addresses only, including authenticated msar requests, so self-signed development certificates work without installing them in Windows. Remote HTTPS addresses use normal certificate validation. The platform users/sessions services and frontend must be deployed together for this flow.

Authenticated API requests use the msar Go client. It captures issued access tokens, sends the `Authorization` header, manages refresh cookies, and supplies the Shell Anywhere platform header. Each restored account or revocation operation has an isolated cookie jar seeded from the system credential store; rotated refresh cookies are saved back there. Anonymous request creation and the one-time code exchange use ordinary HTTP without automatic retries.

## Development

Requirements: Go 1.25+, Node.js 24+, GNU Make, Wails v2.17, and the [Wails v2 platform dependencies](https://v2.wails.io/docs/gettingstarted/installation/). macOS builds require Xcode Command Line Tools

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm --prefix frontend install
make dev
```

The `make dev` target defaults to the local platform at `https://oxee.localhost` and the local provider at `https://shell-aw.oxee.localhost/api`. The app connects `localhost` and `*.localhost` directly to `127.0.0.1`, preserving the hostname for HTTPS and Nginx routing and bypassing environment proxies for local addresses. No hosts-file entries are required for the app. The local platform and provider must be running on that computer. Existing environment overrides are preserved. If an older terminal exports `SHELLANYWHERE_OXEE_URL=https://localhost`, update it to `https://oxee.localhost`: the old address returns a `308` redirect

```powershell
$env:SHELLANYWHERE_OXEE_URL = "https://oxee.localhost"
$env:SHELLANYWHERE_PROVIDER_URL = "https://shell-aw.oxee.localhost/api"
make dev
```

Run `make build` to create the app for the current platform: `build/bin/ShellAnywhere.exe` on Windows, `build/bin/Shell Anywhere.app` on macOS, or `build/bin/ShellAnywhere` on Linux. Run `make frontend` for a browser-only preview with hot reload; SSH, SFTP, and local terminals require `make dev`

On Ubuntu 24.04, install `build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`, then use `make dev BUILD_TAGS=webkit2_41` or `make build BUILD_TAGS=webkit2_41`. Linux saved credentials use the desktop's Secret Service, such as GNOME Keyring; a working D-Bus session and keyring provider are required for saving passwords and account tokens

macOS supports Intel and Apple Silicon on macOS 12+. The local terminal uses `$SHELL` when available, otherwise Zsh on macOS or Bash on Linux, with `/bin/sh` as the final fallback. Terminal copy/paste supports Command+C/Command+V on macOS as well as the Ctrl shortcuts

### Windows installer

Install [NSIS](https://nsis.sourceforge.io/Download) and ensure `makensis.exe` is on your `PATH`
With Windows Package Manager, you can use `winget install --id NSIS.NSIS --exact`, then reopen the terminal

```powershell
make installer
```

This creates `build/bin/ShellAnywhere-amd64-setup.exe`
The setup wizard installs for the current user without requiring administrator access, creates Start menu and desktop shortcuts, registers an uninstaller, and offers to launch the app when finished
If WebView2 is missing, its bootstrapper downloads and installs the runtime, which needs an internet connection
The installer does not bundle the provider service

For Windows ARM64, run `make installer WINDOWS_PLATFORM=windows/arm64` to create `build/bin/ShellAnywhere-arm64-setup.exe`
The equivalent direct command is `wails build -platform windows/amd64 -nsis -installscope user`
Product name, publisher, and version come from the `info` section in `wails.json`
Keep the three-part version up to date when publishing a release

### macOS and Linux packages

Build on the target operating system with the matching architecture, then package the result:

```bash
# macOS: amd64 for Intel, arm64 for Apple Silicon
bash scripts/package-macos.sh arm64

# Linux: amd64 or arm64, followed by the release version
bash scripts/package-linux.sh amd64 1.0.0
```

The macOS disk image contains **Shell Anywhere.app** and an **Applications** shortcut. Drag the app into Applications to install it. Builds are ad-hoc signed by Wails; Developer ID signing and Apple notarization are not configured, so macOS may require explicit approval to open a downloaded build

Linux packages include a `.deb` with a desktop launcher and a `.tar.gz` containing the executable, icon, license, and runtime instructions. The release builds target Ubuntu 24.04/glibc 2.39 or newer with GTK 3 and WebKitGTK 4.1; the archive is not a fully static build for every Linux distribution. Install the `.deb` with `sudo apt install ./ShellAnywhere-linux-amd64.deb`, or extract the archive and run `./ShellAnywhere`

### GitHub releases

The app repository includes `.github/workflows/release.yml`
After pushing the app source and workflow to GitHub, publish a release with a tag such as `v1.2.3`
Publishing a stable release or a prerelease starts the Windows, macOS, and Linux builds automatically; saving a draft or only pushing a tag does not

The workflow runs the Go tests and frontend checks on native runners and attaches these downloads to the release:

- `ShellAnywhere-amd64-setup.exe`
- `ShellAnywhere-arm64-setup.exe`
- `ShellAnywhere-macos-amd64.dmg` — Intel Mac
- `ShellAnywhere-macos-arm64.dmg` — Apple Silicon Mac
- `ShellAnywhere-linux-amd64.deb` and `.tar.gz`
- `ShellAnywhere-linux-arm64.deb` and `.tar.gz`

The application version comes from the tag's three numeric components, so `v1.2.3-beta.1` uses file version `1.2.3` and keeps the full tag in the build comments
Version components must be between 0 and 65535
GitHub release builds use `https://oxee.net` as the default account server; `SHELLANYWHERE_OXEE_URL` can still override it at runtime
Local builds and installers also default to `https://oxee.net`. `make dev` sets the local account and provider URLs through environment variables

No custom secret is required for these builds: the workflow uses GitHub's built-in token with `contents: write` to upload the packages. Apple notarization would require separate Apple signing credentials
To rebuild an existing release, rerun its workflow from the Actions tab; assets with the same names are replaced
The workflow does not commit version changes or push source code

The bottom-left footer shows the actual build version and checks the latest stable GitHub release once when the app starts
It shows **New version available** only when that release is newer, otherwise **Up to date**
If no release exists or the request fails, the footer shows that state instead of claiming the app is current
The result is cached for the app session, including failed checks; there is no periodic update check
When a newer stable release is available, **Update to v…** downloads the matching Windows installer, macOS disk image, or Linux `.deb` directly from `Overwhelmed44/shell-anywhere` without visiting GitHub
The app refreshes release metadata when clicked, accepts only the expected package for this operating system and architecture, and verifies its size and SHA-256 digest from GitHub before opening it
An app-styled confirmation explains the update before downloading the package, opening the installer, and closing Shell Anywhere, ending active sessions and transfers. Host and group deletion also use app-styled confirmations instead of browser popups
Finish the Windows setup wizard, drag the macOS app into Applications, or use the Linux package installer, then reopen the app
Linux requires a desktop handler for `.deb` packages; installations made from the portable archive still need to be updated manually
Failed downloads or installer launches leave the app open and show an error in the confirmation dialog so you can retry. API rate limits and network failures report their actual cause; a validated package from the startup check can still be downloaded if a metadata refresh temporarily fails. GitHub API redirects after repository renames are supported. Packages that are still building or lack a GitHub digest cannot be installed
GitHub builds embed the full release tag, including prerelease suffixes; local builds use the product version from `wails.json`

The React UI lives in `frontend/src`. `App.tsx` manages host and session state, while `components/` contains the dialogs, navigation, host list, cards, tabs, and session view. Shared TypeScript types are in `types.ts`, and the terminal implementation is in `TerminalPane.tsx`.

Choose **Separate credentials**, a saved identity, or **Group default** when adding or editing a host. Separate password credentials are saved automatically and loaded into host settings for viewing and editing. A group without a default identity needs an identity or separate credentials configured in settings before connecting. Identities sync alongside hosts, groups, and trusted server keys, with optional vault password encryption. On first connection, verify the displayed host-key fingerprint through a trusted channel before selecting **Trust & connect**. Encrypted SSH keys still ask for a passphrase when needed
