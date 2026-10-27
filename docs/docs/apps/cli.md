---
sidebar_position: 2
sidebar_label: Command-line tool
---

# Hisar command-line tool

`hisar` lists, reads and edits your items from a terminal or a script (`hisar --help`), runs commands with your secrets and masks them in the output, and serves them to coding agents over MCP. Design: [Design §9.4](../design.md#94-cli).

## Install

**On a Mac with the Hisar app**, the app includes it: Settings → Developer tools → **Install command-line tool**. `hisar` then works
through the running app, which asks you to approve each terminal with Touch ID; `hisar status` shows whether it uses the app.
Details: [Desktop app](./desktop.md#command-line-tool).

**Keep the app running.** SSH clients, `git` and commit signing reach Hisar through `~/.hisar/agent.sock`, and they cannot start the
app themselves: while Hisar is not running they fail at once with "Connection refused" or "Permission denied (publickey)". So the app
opens at login (Settings → General → **Open Hisar at login**, on by default) and keeps running in the menu bar when you close its window;
⌘Q closes the window too, and only **Quit Hisar Completely** in the menu bar quits it. `hisar` itself starts the app when it is installed but not running, waits up to 10 seconds
for it, and otherwise says "Hisar isn't running. Open the Hisar app and try again."; `HISAR_NO_LAUNCH=1` turns that off for scripts and CI.

**Anywhere else** (Linux, Windows, servers, or without the app), download the standalone `hisar` from the published GitHub Release of a `cli-<version>` tag
(github.com/tosunlab/hisar → Releases): `hisar-<version>-<os>-<arch>.tar.gz` for Linux and macOS,
`.zip` for Windows, and `SHA256SUMS`. Check the download before you run it, then put `hisar` on your `PATH`:

For releases with `release-evidence.json`, also [verify the artifact attestation](../development/releasing.md#verify-release-artifacts) before extraction.
The checksum file detects corruption but is not independent proof of who built the archive.

```sh
sha256sum -c --ignore-missing SHA256SUMS          # macOS: shasum -a 256 -c --ignore-missing SHA256SUMS
tar -xzf hisar-0.2.1-linux-amd64.tar.gz && sudo install hisar /usr/local/bin/
hisar --version
hisar login                                       # its own sign-in: server, email, Secret Key, master password
```

On Windows, `Get-FileHash hisar-0.2.1-windows-amd64.zip -Algorithm SHA256` must match the line in `SHA256SUMS`. The standalone
`hisar` keeps its session in `state.json` and asks for the master password for each command; `hisar --standalone` (or
`HISAR_STANDALONE=1`) uses this mode even when the Mac app is running.

## Secrets for commands: `hisar run` and `hisar read`

Name a secret with a reference instead of copying it: `hisar://<vault>/<item>/<field>`. Vault and item are names (case does not
matter) or IDs; the field is `password`, `username`, `notes`, `url`, `otp` (the current one-time code), `public key`,
`fingerprint` or a custom field's label. SSH private keys never leave Hisar (use its SSH agent), so `private key` references are refused. Characters such as `/`, `%` and spaces are percent-encoded (`AWS%20prod`). In the
Hisar app, the terminal button next to a field's copy button copies its reference.

Keep references, not values, in your `.env`:

```sh
# .env — safe to commit; it holds no secrets
DATABASE_URL=postgres://localhost:5432/app
DATABASE_PASSWORD=hisar://Work/Postgres%20staging/password
GITHUB_TOKEN=hisar://Personal/GitHub/API%20token
AWS_SECRET_ACCESS_KEY="hisar://Work/AWS prod/secret access key"
```

```sh
hisar run --env-file .env -- npm test
hisar run --env-file .env -- docker compose up
GITHUB_TOKEN=hisar://Personal/GitHub/API%20token hisar run -- gh release list
hisar read hisar://Personal/GitHub/otp            # one value on stdout, for scripts
```

`hisar run` resolves every reference in one request, so the app asks once (with Touch ID, or on your smart device) and shows the command
and the secrets it reads. The command gets the real values in its environment; Hisar does not write a resolved `.env` file.
Output masking replaces recognized exact values with `<concealed by hisar>` to reduce accidental disclosure. Values shorter than
4 bytes and transformed output are not masked. That needs pipes, so use `--no-masking` for commands that need the terminal itself (colours,
prompts, full-screen tools). Several `--env-file` options are applied in order; variables already in your environment win unless they
are references themselves. Signals are passed on, and `hisar run` exits with the command's exit code. Approving covers the terminal
session: other `hisar` commands in it don't ask again until 10 minutes after the last one, or until Hisar locks.

**With coding agents.** Give the agent the `.env` with references and let it run `hisar run --env-file .env -- <command>`. The
`.env` it reads holds references instead of values. Masking reduces accidental disclosure in output and transcripts, but cannot
prevent the command from revealing a granted secret. The command itself has the values,
and an agent that can run `hisar read` in an approved terminal can read them. An approval covers every vault, so approve an agent's
terminal only when it may use them. Details: [Design](../design.md) §9.4.1. For agents that speak MCP, `hisar mcp`
(below) is narrower: it sees only the vaults you choose and asks per command.

## Discover objects before selecting them

`hisar vaults` lists vault IDs, types, and your roles without revealing item values.
Use `hisar ls -v <vault-ID>` to list item metadata and IDs. `--tag <tag>` filters that list.
Unique names still work, but duplicate names or titles now fail with candidate IDs instead of selecting the first match.
An ID always selects its own vault or item, even when another one is named that ID. This holds wherever a name is accepted, including
`hisar share-link`, a server's SSH key, and the vault choices of `hisar import <.env>` and `hisar vault delete`, which list each
vault's ID, type and your role.
Use `hisar get -v <vault-ID> <item-ID> -f <field>` when you intend to read one value.
Raw field output stays byte-preserving.

For MCP, call `list_vaults`, then `search_items` with the intended vault ID; an ID searches that vault only.
Use the returned item IDs and ready-made field references rather than constructing a reference from ambiguous names.
These discovery tools return metadata, not secret values.

For file imports, run `hisar import <file> --dry-run` to review source groups and destination access.
Unmapped groups default to Personal. Map a shared group explicitly with
`--map '<source-group-ID>=<destination-vault-ID>'`, repeated for each group you want to redirect.
Review the printed members before continuing. `-y` applies the same defaults and explicit mappings without questions.

## Use Hisar with coding agents

`hisar mcp` is an [MCP](https://modelcontextprotocol.io) server on stdio that lets coding agents find secret references and pass
values to approved commands. Command output is masked, not contained. It always works through the Hisar app, which must be running
with Settings → Developer tools → "Let the command-line tool use Hisar" on.

1. In Hisar, open Settings → Developer tools → Coding agents and turn on "Available to agents" for the vaults an agent may use. Every vault starts
   off, and the setting is per device. Agents cannot see the other vaults at all.
2. Connect your agent. Claude Code, Codex and omp connect with one click in the same section (below), and so does Hermes
   ([Sign in to websites](#sign-in-to-websites-computer-use-agents)). Cursor, in
   `~/.cursor/mcp.json` or a project's `.cursor/mcp.json`:

   ```json
   { "mcpServers": { "hisar": { "command": "hisar", "args": ["mcp", "--no-reveal"] } } }
   ```

   Any other MCP client: a stdio server with the command `hisar` and the arguments `mcp --no-reveal`. `--no-reveal` leaves out
   `get_secret` and `get_one_time_code`, removing direct value-returning tools. It does not prevent an approved command from
   disclosing values through its output or other channels. Drop it only for an agent you let read values directly.

The agent gets these tools:

| Tool | What it does | Asks you |
|---|---|---|
| `list_vaults` | the vaults available to agents | once per agent connection |
| `search_items` | items by title, username, URL or vault: metadata, field labels and `hisar://` references, never values or notes | once per agent connection |
| `run_with_secrets` | runs a command, as an argument list or a `/bin/sh -c` command line, with secrets from `env` (variable → `hisar://` reference), `envFiles` (as `hisar run --env-file`) and `stdin` (text or one reference); through `direnv exec` when the directory has an allowed `.envrc`; output masked, at most 64 KiB per stream and streamed as progress to clients that ask, timeout up to 1 hour | every call, showing the command with the env files' plain values (`env NAME=value …`) and its stdin, the directory and secrets, unless you allowed those secrets for longer |
| `sign_in` | signs in to the website in a browser window (`pid` and `window_id` from cua-driver's `list_windows`) with the login saved for it; Hisar for Mac enters and submits it, and the agent gets the outcome, never the password or code | every call, showing the website, the browser and the login, unless you allowed that login on that website for longer |
| `get_one_time_code` | the current one-time code of an item | every call, unless you allowed it for longer |
| `get_secret` | reveals one value to the agent | every call, unless you allowed that field for longer; another field of the item asks again. `private key` is always refused |

Every dialog needs Touch ID, your master password or your phone; a click alone never approves an agent. A dialog that names secrets
has a menu for how long the agent may keep exactly what it names: **Only this time**, **For 1 hour**, **For 1 day** or **Always**.
For omp, Claude Code, Codex and Hermes, a longer choice covers that agent's program across sessions and Hisar restarts only
when Hisar can identify the program and use its Keychain-backed grant store. Otherwise only the first two choices appear,
and **For 1 hour** ends with the agent session or connection.
Settings → Developer tools → Coding agents lists each one with its expiry and a revoke button. Signing out ends them; locking does
not, but a locked Hisar asks you to unlock it first.

The first dialog names the agent (from its MCP client name) and the app it runs in. Denying fails the tool with "The user denied
access in Hisar.", which the agent can relay to you. The agent's connection approval ends when it disconnects or Hisar locks.
A request uses exactly the items the dialog named: if a reference names another item by the time it runs (an item was renamed,
moved or removed), Hisar hands out nothing and the request is denied. A request still running when Hisar locks, signs out or the
agent's session ends is denied too, and nothing it approved is remembered.

Limits: a command run with secrets has the real values and can write them to a file, send them elsewhere or print them encoded,
which masking does not catch, so approve only commands you would run yourself; allowing secrets for longer lets the agent run any
command with them for that time. A value from `get_secret` is in the agent's transcript and logs. Details:
[Design](../design.md) §9.4.2.

### Claude Code

In Hisar, open Settings → Developer tools → Coding agents and click **Connect** next to Claude Code. Hisar finds `claude` on your
PATH, in `~/.local/bin`, `/opt/homebrew/bin`, `/usr/local/bin` or `~/.claude/local`, and adds the `hisar` inside Hisar.app to
your user settings (`~/.claude.json`, or `$CLAUDE_CONFIG_DIR/.claude.json`):

```sh
claude mcp add --scope user --transport stdio hisar -- /Applications/Hisar.app/Contents/Helpers/hisar mcp --no-reveal
```

That command is also the way to connect by hand. New Claude Code sessions load Hisar; restart the ones already running.
**Disconnect** runs `claude mcp remove hisar --scope user`. If your settings already have a server named `hisar` that Hisar did not
add, or Hisar's with a setting you added or changed, the card says "Claude Code already has a server named hisar. Remove it to
connect Hisar." and Hisar never changes it.
Commands Claude Code runs in its shell get the coding-agent rules, as omp's do (below).

### Codex

Click **Connect** next to Codex in the same section. Hisar finds `codex` on your PATH, in `/opt/homebrew/bin`, `/usr/local/bin`,
`~/.npm-global/bin` or `~/.local/bin`, and adds this table to `~/.codex/config.toml` (or `$CODEX_HOME/config.toml`), leaving the
rest of the file as it was:

```toml
[mcp_servers.hisar]
command = "/Applications/Hisar.app/Contents/Helpers/hisar"
args = ["mcp", "--no-reveal"]
startup_timeout_sec = 15
tool_timeout_sec = 4200
```

To connect by hand, add the same table: `codex mcp add` cannot set the timeouts, and Codex's default of 60 seconds per tool call
would end a `run_with_secrets` call while its approval is still open. New Codex sessions load Hisar; restart the ones already
running. **Disconnect** runs `codex mcp remove hisar`. A `hisar` server that Hisar did not add, or this table with a key you added
or changed, is left alone, as for Claude Code.
Codex's sandbox blocks Unix sockets for the commands it runs, so `hisar run` in Codex's shell cannot reach the app; Codex uses
your secrets through `run_with_secrets`, which runs outside the sandbox.

### omp and Tern

Hisar comes with a plugin for omp, the coding agent that also runs in Tern's panes. Connecting it gives omp both the plugin and
Hisar's MCP server as `hisar mcp --no-reveal`. omp passes references to commands, Hisar masks recognized values in their output,
and its status line shows whether Hisar is locked. The command still receives the secrets.

1. In Hisar, open Settings → Developer tools → Coding agents and turn on "Available to agents" for the vaults omp may use, as for
   `hisar mcp` above.
2. Next to omp in the same section, click **Connect**. Hisar finds omp on your PATH, in `~/bin`, in Tern
   (`~/Library/Application Support/Tern/omp/bin`) or in `~/.bun/bin`, and runs `omp plugin link` with the plugin inside Hisar.app.
   The plugin starts the MCP server with the `hisar` inside Hisar.app. New omp sessions load Hisar; restart the ones already
   running. **Disconnect** runs `omp plugin uninstall @tryhisar/omp-plugin`, which removes the MCP server too.
   On Linux, link the plugin from a checkout of this repository: `omp plugin link <checkout>/omp`.
3. Start omp. Its status line says "Hisar locked" or "Hisar unlocked", and `/hisar` shows Hisar's state. The agent gets
   `list_vaults`, `search_items`, `run_with_secrets` and `sign_in`, never `get_secret` or `get_one_time_code`. omp shows nothing of a
   `run_with_secrets` command's output until it ends, so for a long command, one that asks for input or one that needs the shell's
   environment, the agent runs `hisar run --env-file .env -- <command>` in its shell instead.

Every `hisar` command omp runs, in its shell or as `hisar mcp`, gets the coding-agent rules
([Design](../design.md) §9.4.4). It sees only the vaults available to agents. `hisar run` asks for the secrets it uses,
unless a use grant covers them. `hisar read` and `hisar get` ask unless a reveal grant covers them. Changes to items are refused.
The approval names omp and its host app ("omp in Tern wants to use 1 secret"). Connection approvals end when omp exits or Hisar
locks. Session-scoped secret grants end when omp exits, but survive lock. Program-scoped saved grants also survive sessions and
Hisar restarts. ssh and git inside omp get only the SSH keys in vaults available to agents. The host app, such as Tern, becomes an
agent host. Its other processes without a terminal ask on every connection, and it can no longer be a launcher.

## Sign in to websites (computer-use agents)

An agent that drives your Mac through [cua-driver](https://cua.ai/docs/cua-driver) can ask Hisar to sign in to a website instead
of supplying a password in tool arguments. It calls `sign_in` on `hisar mcp`, and Hisar for Mac fills the browser window itself.
It works in Brave, Google Chrome and Microsoft Edge. The tool returns no credential value, but page scripts and captures can
still expose entered values.

1. The agent opens the site's sign-in page and calls `sign_in` with the window's `pid` and `window_id` from cua-driver's
   `list_windows`.
2. Hisar reads the page's address from the browser's accessibility tree, never from the agent, and finds the logins saved for
   that website in the vaults available to agents.
3. Hisar asks you, naming the agent, the website, the browser and the login. You approve with Touch ID, your master password or
   your phone.
4. Hisar enters the username, password and one-time code and submits each step of the sign-in (an email page, then a password
   page, then a code page) while the page stays on that exact website. It then attempts to clear remaining entries and reports
   the cleanup evidence.
5. The agent gets the outcome and the login's title and username. It never gets the password or the code.

### Turn on Website sign-in

In Hisar, open Settings → Developer tools → Coding agents and turn on **Let agents sign in to websites in your browser** under
Website sign-in. It is off by default. Turning it on asks macOS to give Hisar the Accessibility permission. Until macOS does, the
row says "Accessibility permission needed", and **Open System Settings** opens Privacy & Security → Accessibility, where you turn
on Hisar. cua-driver needs its own permissions as well; its [quickstart](https://cua.ai/docs/cua-driver/quickstart) lists them.

Hisar signs in only with logins in vaults whose "Available to agents" switch is on (above), and only on a website the login is
saved for: the same host, or, with Settings → Browser → **Only fill on the exact host** off, a host under the same domain (a login
for `google.com` also fills on `accounts.google.com`). The page must be https, or http on `localhost` or a loopback address.

### Hermes

Hermes' computer-use tool runs cua-driver for it.

1. Turn on computer use in Hermes: `hermes tools enable computer_use`. If it does not work, `hermes computer-use doctor` says why.
2. In Hisar, open Settings → Developer tools → Coding agents and click **Connect** next to Hermes. Hisar finds `hermes` on your
   PATH, in `~/.local/bin`, `~/.hermes/hermes-agent/.hermes/bin`, `/opt/homebrew/bin` or `/usr/local/bin`, asks it with
   `hermes config path` which `config.yaml` it reads (with the `HERMES_HOME` of your login shell), and adds this entry under
   `mcp_servers`, leaving the rest of the file as it was:

   ```yaml
   mcp_servers:
     hisar:
       command: "/Applications/Hisar.app/Contents/Helpers/hisar"
       args: ["mcp", "--no-reveal"]
       timeout: 4200
       connect_timeout: 60
   ```

   Hermes cancels a tool call at its `timeout`, so the entry leaves room for your approval and a `run_with_secrets` command of up
   to an hour. When Hisar serves another socket than `~/.hisar/cli.sock`, the entry also sets `env: {HISAR_CLI_SOCKET: …}`.
   **Disconnect** removes the entry. A `hisar` server that Hisar did not add is left alone, as for Claude Code.
3. Start a new session with `hermes chat`, or run `/reload-mcp` in one that is running. A session started with `-t` loads Hisar
   only when its list names `hisar`, `all` or `*`, such as `hermes -t computer_use,hisar chat`.

Ask Hermes to use the native `computer_use` window controls to open the sign-in page and call Hisar's `sign_in`, not a CDP
`browser` or `browser_exec` tool. Hisar refuses debuggable browsers.

Hermes is unsigned Python. Hisar recognizes its installer's launcher, and eligible saved grants cover that launcher's Python
running that launcher's code ([Desktop app](./desktop.md#coding-agent-approvals-and-website-sign-in)).

### Claude Code and Codex

1. Install cua-driver as its [quickstart](https://cua.ai/docs/cua-driver/quickstart) says.
2. Run `cua-driver mcp-config --client claude` (or `--client codex`). It prints the command that adds cua-driver to that agent;
   run the printed command.
3. In Hisar, click **Connect** next to Claude Code or Codex in Settings → Developer tools → Coding agents (above).
4. Start a new session, and ask the agent to sign in through cua-driver's native window controls, not a CDP browser tool.

### Any other MCP client

Give the client cua-driver's MCP server (`cua-driver mcp`) and Hisar's (`hisar mcp --no-reveal`, above) side by side. Use native
window controls, not a CDP browser tool. Set the tool timeout to at least 5 minutes. The dialog normally waits 60 seconds, or
2 minutes when the request is also sent to a phone. `hisar mcp` waits up to 5 minutes for the app. An agent outside a recognized
harness gets only **Only this time** and **For 1 hour**, and its grants end with its connection.

### The approval

The dialog says "Hermes in Terminal wants to sign in to https://tosunlab.com". It shows the website and the browser
("https://tosunlab.com in Brave Browser"), the login's vault, title and username, and a duration menu. **Only this time** is the default.
**For 1 hour**, **For 1 day** and **Always** are available when Hisar can identify the harness's program and save its grants in
the Keychain-backed store. Otherwise only **Only this time** and **For 1 hour** appear, and the hour ends with the session or
connection.

When several logins match, you pick one on the Mac. A click never approves. Touch ID's prompt names exactly what you allow
(`allow "Hermes in Terminal" to sign in to https://tosunlab.com as user@example.com for 1 day`). Changing the login or duration starts a
new prompt. Without Touch ID the dialog asks for your master password. Your phone can approve a sign-in with one login, only for
this time.

- **Only this time** covers that one call. The agent's next `sign_in`, even at the same website, asks again.
- A longer choice covers only the chosen login at that exact website (scheme, host and port). Later sign-ins there need no
  dialog. Eligible saved grants cover the harness's program across sessions and Hisar restarts.
  [Desktop app](./desktop.md#coding-agent-approvals-and-website-sign-in) explains program identity and storage.
- Settings → Developer tools → Coding agents → **Allowed without asking** lists each grant ("Sign in to https://tosunlab.com as
  Personal › Tosun Lab") with the agent that holds it, its expiry and **Revoke**. Signing out ends them all. Locking keeps them, but a
  locked Hisar asks you to unlock it before it signs in.
- `cleanup-failed` ends the grant for that login and website, so the next call asks you. If Hisar is locked or cannot save the
  removal, it drops all saved grants instead. A storage failure is reported as a tool error, not hidden.
- While Hisar is locked, the dialog unlocks it first ("Hisar shows the login saved for this website after you unlock it"). Hisar
  then reads the window again, refuses if it shows another website, and asks once more if several logins match.

### What the agent sees

The tool result has the outcome, cleanup evidence (`cleanup`), the website and browser Hisar read, and the login's ID, vault,
title and username. It also lists the parts Hisar entered on each page and a message for the model. Hisar never sends a password,
a one-time code or an authenticator secret to `hisar mcp`. A refusal is a tool error whose message says what to do next.

### Limits

- `sign_in` returns no password or one-time code. A screenshot, recording or accessibility read can capture a one-time code
  while the page retains it. Hisar enters passwords only into fields macOS reports as secure text fields.
  Browsers normally mask these with dots, which reveal the password's length. Page scripts and show-password controls can
  reveal the password.
- Hisar uses a 15-second deadline after each submit and a separate five-second cleanup timeout. A browser Accessibility call
  can finish after those deadlines. These are not limits on how long a value remains on the page.
- Hisar does not pause the agent while `sign_in` runs. Another tool call can take a screenshot or click the page's show-password
  button before cleanup.
- Hisar refuses a browser started with remote debugging (`--remote-debugging-port`, `--remote-debugging-pipe`, or a profile
  whose `DevToolsActivePort` port accepts connections), since any program could then read the page. A browser that a tool drives
  over the DevTools protocol is one.
- `fields-removed` means the fields left the accessibility tree but their document stayed. Hisar cannot confirm the page erased
  their values. `cleanup-failed` means clearance could not be confirmed. Either can leave values present after return with no
  Hisar-enforced retention limit. Page-script copies, unrecognized save-password prompts and captures also have no deletion limit.
- Hisar fills a username, a password and a one-time code from the login's authenticator secret. A code sent by SMS or email,
  a passkey, an account chooser or a CAPTCHA is for the agent or for you.
- Out of scope: an agent that operates Hisar's own window while Hisar is unlocked, and other programs that run as you, which can
  read the browser's pages or take screenshots themselves.
- Safari, Arc and other browsers are not supported. Hisar signs in one window at a time; a second `sign_in` waits for the first.

See [Design](../design.md) §9.4.5 for the transaction and threat model.

### Troubleshooting

The table quotes the messages the agent can relay. Braced placeholders stand for substituted values. `{id}` uses Go's quoted
string format, including escapes when needed.

| The agent gets | What to do |
|---|---|
| "Signing in to websites works only in Hisar for Mac." | Use Hisar for Mac; the Linux app does not sign in to websites. |
| "This Hisar app can't sign in to websites. Ask the user to update Hisar." | Update the Hisar app. |
| `Website sign-in is off in Hisar. Ask the user to turn on Hisar → Settings → Developer tools → Coding agents → Website sign-in.` | Turn on Website sign-in (above). |
| `Hisar can't fill browser windows yet: macOS hasn't given it the Accessibility permission. Ask the user to open Hisar → Settings → Developer tools → Coding agents → Website sign-in and allow it.` | Turn on Hisar in System Settings → Privacy & Security → Accessibility. |
| `pass the pid and window_id of the browser window from list_windows` | Supply a positive process ID and a nonzero window ID from `list_windows`. |
| "Process {pid} has no window {window_id}. Get the current pid and window_id from list_windows." | The window closed or the IDs are old; the agent lists the windows again. |
| `Window {window_id} is on another Space (or minimized), where Hisar can't read the page. Bring it to the current Space with focus_app or bring_to_front, then call sign_in again.` | Bring the window to the current Space, then call again. |
| `Hisar couldn't read window {window_id} in full: the browser didn't answer in time, or the page is too large. Wait until the page has loaded and call sign_in again.` | Wait until the page has loaded, then call again. |
| "Window {window_id} belongs to {app}, not a browser Hisar signs in with. Hisar supports Brave Browser, Google Chrome and Microsoft Edge." | Use a window of one of those browsers. |
| `{browser} was started with remote debugging, so any program on this Mac could read what Hisar types into its pages. Hisar won't sign in there. Use a browser window that was started normally.` | Quit the browser and start it normally, or use another browser window. |
| `The window shows {address}, not a website. Open the site's sign-in page and call sign_in again.` | Open the site's sign-in page first. |
| `The window shows a page that is not a website. Open the site's sign-in page and call sign_in again.` | Open the site's sign-in page first. |
| "The page is {website}. Hisar signs in only on https pages." | Use the site's https address. |
| `The sign-in form comes from {form_origin} inside a page of {top_origin}. Open {form_origin} itself and call sign_in again.` | Open the framed sign-in page on its own. |
| `Hisar found no username, password or one-time code field on {website}. Open the sign-in page, wait until its form shows, and call sign_in again.` | Wait until the form shows. On a page that is already signed in, nothing is left to do. |
| "This page has more than one password field, as sign-up and change-password forms do. Hisar fills sign-in forms only." | You fill that form. |
| "The password field on {website} is not a real password input, so the password would show on screen. Hisar won't fill it." | You sign in on that site. |
| `The page on {website} asks for {what}, which Hisar doesn't fill with this login. Handle it, then call sign_in again if a username, password or one-time code field shows.` | The login lacks what the first page asks for. Handle that page. |
| `No login in the vaults available to agents is saved for {website}. Ask the user to add one in Hisar or make its vault available to agents. Don't ask for the password.` | Add the website to a login, or make its vault available to agents. |
| `No login available to agents has the ID "{id}". Find the login with search_items, or leave out login.` | Find the login with `search_items` or leave out `login`. |
| `“{title}” is saved for {websites}, but the page is {website}. Hisar only signs in on a login's saved websites.` | The named login is not saved for this website. |
| `The window changed while the user was approving: it now shows {page}. Hisar entered nothing. Call sign_in again if the sign-in page is back.` | Return to the sign-in page, then call again. |
| `“{title}” changed while the user was approving, or its vault is no longer available to agents. Hisar entered nothing. Call sign_in again.` | Call again. The dialog shows the login as it is now. |
| `The sign-in request ended before Hisar entered anything. Call sign_in again if the agent still needs it.` | Call again if the agent still needs it. |
| `Hisar couldn't enter the login into the page, and cleared what it had entered. Reload the sign-in page and call sign_in again.` | Reload the sign-in page, then call again. |
| "The sign-in was interrupted (Hisar locked, the agent session ended or the request was cancelled). Hisar cleared what it had entered." | Unlock Hisar or restart the agent, then call again. |
| `The user denied access in Hisar.` | You denied it. The agent does not retry. |
| `Hisar stopped the request because it locked or signed out, or the agent session ended.` | Unlock or sign in to Hisar, or start a new agent session. This is not a user denial. |
| `The user did not answer Hisar's approval dialog in time.` | Answer before the dialog's deadline, normally 60 seconds, or two minutes when the request is also sent to a phone. |
| "Hisar is locked. Ask the user to unlock the Hisar app." | Unlock Hisar, then call again. |
| "Hisar is not signed in. Ask the user to sign in to the Hisar app." | Sign in to the Hisar app. |
| `Hisar is not available: {error}. Ask the user to open the Hisar app and to keep Settings → Developer tools → “Let the command-line tool use Hisar” on.` | Open Hisar and turn on the named setting. |
| `The Hisar app does not let the command-line tool use it. Ask the user to turn on Settings → Developer tools → “Let the command-line tool use Hisar”.` | Turn on the named setting. |
| `{cleanup_message} Hisar could not save the grant revocation: {error}. Stop, and tell the user to check Hisar and the browser window.` | Cleanup failed and revocation hit a storage error. Stop and check both windows. `{cleanup_message}` is the full cleanup-failed message. |

### For agent and driver authors

`sign_in` is a tool of `hisar mcp`, with or without `--no-reveal`. Hisar for Mac answers it; the app announces the
`agent-sign-in` feature, and `hisar mcp` refuses the call on an app without it.

**Input.**

| Field | Type | Meaning |
|---|---|---|
| `pid` | integer, required | The browser process that owns the window, as cua-driver's `list_windows` reports it. |
| `window_id` | integer, required | The macOS window ID (`CGWindowID`) of the browser window. |
| `login` | string, optional | The item ID from `search_items`, for a site where the user has several accounts. Without it, Hisar offers every login saved for the website. |

Call it while the page shows its sign-in form. Do not type into the form first. Hisar replaces what the fields hold.

**Result.** The tool result's text is `message`, and its structured content is:

```json
{
  "outcome": "submitted",
  "cleanup": "page-left",
  "website": "https://tosunlab.com",
  "browser": "Brave Browser",
  "login": {
    "id": "019a3f4e-7b2c-7d10-9a51-2f6c8e4d1b07",
    "vaultId": "019a3f4e-1c05-7e2a-8f3b-6d0a9c2e5b11",
    "vaultName": "Personal",
    "title": "Tosun Lab",
    "username": "user@example.com"
  },
  "steps": [
    { "filled": ["username"], "submitted": true },
    { "filled": ["password"], "submitted": true }
  ],
  "message": "Hisar signed in to https://tosunlab.com as user@example.com and submitted it. Take a screenshot to see the page."
}
```

`website` is the origin Hisar read from the browser. `steps` lists each page Hisar submitted and the parts it entered on it:
`username`, `password` and `one-time code`.

| `outcome` | Meaning | What the agent does next |
|---|---|---|
| `submitted` | Hisar submitted a password or a one-time code, and the form left the page. | Take a screenshot to see where the site went. It may still ask for something Hisar does not fill. |
| `not-accepted` | The site kept its sign-in form after Hisar submitted a password or code, or asked for one again. The password or code may be wrong. | Tell the user. Do not retry. |
| `stopped` | After at least one submitted page, the page asks for something Hisar does not fill, or moved to another website. `message` says which ("the page moved to https://accounts.example"). | Handle the page, then call `sign_in` again when a username, password or one-time code field shows. Another website asks the user again. |
| `cleanup-failed` | Hisar could not confirm that what it entered is off the page. `cleanup` is `unverified`. | Stop, and tell the user to check the browser window. Do not call `sign_in` again. |

| `cleanup` | Meaning |
|---|---|
| `nothing-entered` | Hisar entered nothing. |
| `page-left` | Every field Hisar filled left with its document: the page navigated, or the window closed. |
| `fields-cleared` | Hisar cleared every field it filled that is still on the page, and each reads back empty. The others left with their documents. |
| `fields-removed` | The page removed a field Hisar filled but kept its document. Accessibility cannot show whether the page erased the value. |
| `unverified` | A clear failed, the browser's offer to save the password stayed open, or the window could not be read again or belongs to another process. Only with `cleanup-failed`. |

**Errors.** A refusal is a tool error (`isError`) with a message listed under Troubleshooting. Hisar refuses before entry, or
after cleanup reports `page-left` or `fields-cleared`. With weaker evidence, the outcome becomes `cleanup-failed` instead.
`fields-cleared` means a clear followed by zero read-back, not an independent check of the DOM. If grant revocation also fails
to persist, the tool returns an error with the cleanup warning and storage failure. Retry only when the message says to call
again. A user denial and `not-accepted` are final.

**Timing.** The call returns after approval and the sign-in, and sends no progress notifications. The dialog normally waits
60 seconds, or two minutes when the request is also sent to a phone. `hisar mcp` waits up to five minutes for the app.
Hisar uses a 15-second deadline after each submit and a separate five-second cleanup timeout. A browser Accessibility call can
finish after those deadlines. Give the tool call a timeout of at least five minutes.

**Repeating a call.** Each call reads the window anew and starts at the step the page shows; it carries nothing over from an
earlier call. A call on a page without a sign-in form enters nothing and refuses. A call that a grant covers runs without a
dialog only when the grant covers exactly one of the logins saved for the website; otherwise it asks. Hisar runs one sign-in at
a time on the Mac: a second call can be approved while the first runs, and its sign-in starts when the first one ends.
