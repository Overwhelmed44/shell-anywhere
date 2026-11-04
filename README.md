# Shell Anywhere

A desktop SSH client built with Go, Wails, React, and xterm.js.

## Features

- Organize Personal workspace hosts into groups, search connections, and mark favorites. Groups have stable IDs, can be renamed, and can supply a default identity. Deleting a group keeps its hosts and preserves an inherited identity as a direct identity reference. New hosts default to user `root`; a blank display name uses the hostname or IP
- Manage reusable **Identities** from the sidebar. Each identity contains a username and password or SSH private key, with an optional passphrase. New hosts inherit their group's default identity unless separate credentials or another identity are explicitly chosen. Updating an identity affects future connections from every host using it. Deleting an identity asks for confirmation in the sidebar, keeps dependent hosts' credentials separately, and clears group defaults referencing it
- Create and edit hosts, groups, and identities in a right sidebar while keeping the workspace visible. Only one editor can be open at a time; opening another replaces the current editor. Saved passwords are intentionally not loaded into the editor; leaving the password field blank keeps the saved value
- Connect with a password or a local private key, including encrypted keys.
- Verify a new server's SSH host-key fingerprint before trusting it. Changed host keys are rejected.
- Use interactive terminal tabs with automatic resizing.
- Copy selected terminal text with **Ctrl+C**, **Ctrl+Shift+C**, or **right-click → Copy**. With no selection, Ctrl+C still interrupts the running command. Paste with **Ctrl+V**, **Ctrl+Shift+V**, **Shift+Insert**, or **right-click → Paste**.
- Drag an inactive terminal tab onto the open terminal's left, right, top, or bottom edge to split the workspace. The two tabs become one group tab, and you can keep multiple groups open. Use a group's split icon to restore its separate tabs, or its close button to close both sessions. Drag the divider to resize the panes.
- When an SSH connection ends, use **Reconnect** in its terminal header. The existing tab, terminal history, and split group stay in place. Saved passwords reconnect directly; other credentials use the connection dialog.
- Open local terminals: PowerShell and Command Prompt on Windows, or your login shell on macOS and Linux. The menu lists the shells available on the current platform
- Switch between dark and light themes; the selected theme is remembered on this device.
- Save SSH passwords and successfully used private-key passphrases in the operating system's credential store, and include them in encrypted syncing
- Store hosts and trusted host keys in the user's config directory.

## SFTP file transfers

Click **SFTP** on a saved host to open an SFTP split group. The clicked host appears on the left, starting at `/`. The right pane starts with a destination chooser. SFTP uses the host's saved identity or separate credentials and the same trusted server keys as SSH, without starting a terminal

In the right pane, choose **This computer** or another saved host. Choosing a host opens a separate SFTP connection in that pane, starting at `/`, and asks for credentials or server-key confirmation when needed. Drag files or folders either way to copy between the hosts. Host-to-host transfers stream through the app without storing a local copy. Switching back to **This computer** closes the extra connection and returns to local files. Drag the divider to resize the two sides; each split group retains its destination and layout when switching tabs

Drag files or folders between the two panes to copy them, or select items and use the arrows between the panes. Dropping onto a folder copies into that folder; dropping onto the pane's empty area uses its current path. Folders are copied recursively, including empty folders and hidden files. Symbolic links and special files are not copied

You can also drop files and folders from Windows Explorer onto either pane displaying a remote host. **Upload files** and **Upload folder** upload from this computer to the host in the left pane. To download, choose **This computer** in the right pane; dragging remote files directly into Explorer is not supported

Double-click folders to open them, enter a path in the location bar, or use Home, Parent folder, Refresh, and New folder. Use **Hidden files** to show dotfiles. Downloads go to the local folder currently open in the right pane

Transfers show their progress and can be cancelled. Existing files require confirmation before replacement. Each file is written to a temporary file before replacing its destination, so cancelling a file midway preserves the existing destination; files already completed and folders already created remain. Closing the SFTP split group ends both connections and stops any active transfer. Each disconnected host can reconnect; use the reconnect icon beside the right pane's selector for the other host

## Optional Oxee account

Use **Sign in** in the top-right navigation or **Sign in with Oxee** in the sidebar to open the system browser. Sign in or create an account there, then return to Shell Anywhere. Your username replaces the navigation sign-in button, and the sidebar sign-in card disappears. Click your username and select **Sign out** to disconnect the account. An existing browser login can continue with the displayed account or select another one. Login is optional: hosts, SSH sessions, and local terminals work without an Oxee account and while offline.

After signing in, optionally enable **Host syncing** with a vault password. Hosts, groups, favorites, saved SSH passwords, private-key contents, saved key passphrases, and trusted server keys are encrypted together before upload using AES-256-GCM and an Argon2id-derived key. Sign in and enter the same vault password on another device to restore ready-to-use connections. The derived vault key is remembered in the OS credential store under `shell-anywhere-vault`; the vault password and derived key are never sent to the server

Restored passwords and key passphrases go into the new device's OS credential store. Private keys are restored into the app's configuration directory under `keys/`, with device-local paths and owner-only file permissions on systems that support them. Passwords and key contents never appear in `hosts.json` or `vault.json`. A key passphrase is remembered after a successful SSH authentication; keys that have never been unlocked in this app still need their passphrase once. Oxee login sessions remain device-specific

The vault JSON and its encrypted envelope both use format version `1`. Syncing encrypts the complete vault, including SSH credentials

The decrypted document contains `workspaces.personal` and an `workspaces.others` map reserved for future workspace support. Within each workspace, `hosts`, `groups`, and `identities` are maps keyed by stable IDs. IDs are not repeated inside their values. Hosts reference groups through `groupId` and identities through `identityId`, or set `useGroupIdentity` to inherit `defaultIdentityId` from their group. Credentials are stored once in an identity; hosts with separate credentials store their own. `knownHosts` belongs to its workspace. See [vault.example.json](vault.example.json) for a complete example

Only Personal is exposed in the current interface. Creation, sharing, invitations, and transferring hosts or groups between workspaces are deferred until the workspace API is available. This change does not add workspace creation endpoints or migrations

Open your username → **Host syncing** to set up syncing, sync now, or pause syncing. Changes upload automatically and the app checks the cloud every 30 seconds. Whole vaults replace earlier versions without merging; local edits take priority on the next upload. If local and cloud lists differ during setup, choose which to use. Before replacing local hosts the app saves `hosts-before-sync.json`. **Not now** keeps everything local. Signing out stops syncing.

The provider is reached at `https://shell-aw.oxee.net/api` through the platform Nginx proxy. Login remains on the configured Oxee origin. Set `SHELLANYWHERE_PROVIDER_URL` to override the full provider API base for development or another deployment, for example `https://shell-aw.localhost/api`. Nginx forwards `/api/...` unchanged to the provider's Docker network alias `shell-aw`. Run the provider's `docker-compose.dev.yaml` alongside the platform, with a `shell-anywhere` bucket available in its configured S3 storage. The app uses msar for provider requests with Authorization and saves rotated refresh sessions. Sync metadata in `vault.json` is scoped to the signed-in account and server.

The app binds a temporary listener to `127.0.0.1` on an available port. Oxee stores that port in the pending request and returns a one-time authorization code to it after login/signup. Shell Anywhere accepts the callback only at `/oauth/<rid>` for its active request and exchanges the code with that request id. The server stores the code hash in `OAuthRequest.code` and deletes the request after a successful exchange. A separate `state` parameter is not used; rejected callbacks and failed exchanges leave the listener available for the valid callback. The listener closes on completion, cancellation, or after ten minutes. Refresh tokens stay in the operating system credential store under `shell-everywhere-oxee`, independently of SSH passwords and the browser session. They are never passed to the React UI or written to host configuration files.

The browser stays on Oxee and sends the callback in the background. The local listener permits CORS only from the configured Oxee origin and acknowledges success with the account identity after saving the session. It never returns refresh tokens to the browser. Oxee then shows an animated check, the connected account, and a message that the tab can be closed. If the local app cannot be reached or browser permission is denied, the page offers a retry without repeating login/signup. Some browsers request permission to connect to the local app.

**Sign out** removes the saved local login and attempts to revoke its server session when Oxee is reachable. For a local platform deployment, set `SHELLANYWHERE_OXEE_URL` before launching the app. The current development default is `https://localhost`; use `https://oxee.net` for production. Overrides must use HTTPS, except loopback HTTP addresses for development. Certificate validation is skipped for `localhost` and loopback IP addresses only, including authenticated msar requests, so self-signed development certificates work without installing them in Windows. Remote HTTPS addresses use normal certificate validation. The platform users/sessions services and frontend must be deployed together for this flow.

Authenticated API requests use the msar Go client. It captures issued access tokens, sends the `Authorization` header, manages refresh cookies, and supplies the Shell Anywhere platform header. Each restored account or revocation operation has an isolated cookie jar seeded from the system credential store; rotated refresh cookies are saved back there. Anonymous request creation and the one-time code exchange use ordinary HTTP without automatic retries.

## Development

Requirements: Go 1.25+, Node.js 24+, GNU Make, Wails v2.17, and the [Wails v2 platform dependencies](https://v2.wails.io/docs/gettingstarted/installation/). macOS builds require Xcode Command Line Tools

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm --prefix frontend install
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
Local development and local installer builds keep the localhost default

No custom secret is required for these builds: the workflow uses GitHub's built-in token with `contents: write` to upload the packages. Apple notarization would require separate Apple signing credentials
To rebuild an existing release, rerun its workflow from the Actions tab; assets with the same names are replaced
The workflow does not commit version changes or push source code

The bottom-left footer shows the actual build version and checks the latest stable GitHub release once when the app starts
It shows **New version available** only when that release is newer, otherwise **Up to date**
If no release exists or the request fails, the footer shows that state instead of claiming the app is current
The result is cached for the app session, including failed checks; there is no periodic update check
GitHub builds embed the full release tag, including prerelease suffixes; local builds use the product version from `wails.json`

The React UI lives in `frontend/src`. `App.tsx` manages host and session state, while `components/` contains the dialogs, navigation, host list, cards, tabs, and session view. Shared TypeScript types are in `types.ts`, and the terminal implementation is in `TerminalPane.tsx`.

Choose **Separate credentials**, a saved identity, or **Group default** when adding or editing a host. Separate password credentials are saved automatically; leave the password field blank while editing to keep them. When using a group without a default identity, the app asks for a password on connection. Identities are encrypted and synced alongside hosts, groups, and trusted server keys. On first connection, verify the displayed host-key fingerprint through a trusted channel before selecting **Trust & connect**
