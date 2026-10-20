# Hisar command-line tool

`hisar` lists, reads and edits your items from a terminal or a script (`hisar --help`), runs commands with your secrets and masks them in the output, and serves them to coding agents over MCP. Design: [docs/DESIGN.md §9.4](../docs/DESIGN.md#94-cli).

## Install

**On a Mac with the Hisar app**, the app includes it: Settings → Developer tools → **Install command-line tool**. `hisar` then works
through the running app, which asks you to approve each terminal with Touch ID; `hisar status` shows whether it uses the app.
Details: [desktop/README.md](../desktop/README.md#command-line-tool).

**Keep the app running.** SSH clients, `git` and commit signing reach Hisar through `~/.hisar/agent.sock`, and they cannot start the
app themselves: while Hisar is not running they fail at once with "Connection refused" or "Permission denied (publickey)". So the app
opens at login (Settings → General → **Open Hisar at login**, on by default) and keeps running in the menu bar when you close its window;
⌘Q or **Quit Hisar** in the menu bar quits it. `hisar` itself starts the app when it is installed but not running, waits up to 10 seconds
for it, and otherwise says "Hisar isn't running. Open the Hisar app and try again."; `HISAR_NO_LAUNCH=1` turns that off for scripts and CI.

**Anywhere else** (Linux, Windows, servers, or without the app), download the standalone `hisar` from the published GitHub Release of a `cli-<version>` tag
(github.com/tosunlab/hisar → Releases): `hisar-<version>-<os>-<arch>.tar.gz` for Linux and macOS,
`.zip` for Windows, and `SHA256SUMS`. Check the download before you run it, then put `hisar` on your `PATH`:

For releases with `release-evidence.json`, also [verify the artifact attestation](../docs/RELEASING.md#verify-release-artifacts) before extraction.
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
terminal only when it may use them. Details: [docs/DESIGN.md](../docs/DESIGN.md) §9.4.1. For agents that speak MCP, `hisar mcp`
(below) is narrower: it sees only the vaults you choose and asks per command.

## Use Hisar with coding agents

`hisar mcp` is an [MCP](https://modelcontextprotocol.io) server on stdio that lets coding agents find secret references and pass
values to approved commands. Command output is masked, not contained. It always works through the Hisar app, which must be running
with Settings → Developer tools → "Let the command-line tool use Hisar" on.

1. In Hisar, open Settings → Developer tools → Coding agents and turn on "Available to agents" for the vaults an agent may use. Every vault starts
   off, and the setting is per device. Agents cannot see the other vaults at all.
2. Connect your agent. Claude Code, Codex and omp connect with one click in the same section (below). Cursor, in
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
| `run_with_secrets` | runs a command, as an argument list or a `/bin/sh -c` command line, with secrets from `env` (variable → `hisar://` reference), `envFiles` (as `hisar run --env-file`) and `stdin` (text or one reference); through `direnv exec` when the directory has an allowed `.envrc`; output masked, at most 64 KiB per stream and streamed as progress to clients that ask, timeout up to 1 hour | every call, showing the command with the env files' plain values (`env NAME=value …`) and its stdin, the directory and secrets; "Allow these secrets for this agent for 1 hour" |
| `get_one_time_code` | the current one-time code of an item | every call; can be allowed for 1 hour |
| `get_secret` | reveals one value to the agent | every call, for that item; "Allow every field of “&lt;item&gt;” for this agent for 1 hour". `private key` is always refused |

The first dialog names the agent (from its MCP client name) and the app it runs in. Denying fails the tool with "The user denied
access in Hisar.", which the agent can relay to you. Approvals and allowed secrets end when the agent disconnects or Hisar locks.

Limits: a command run with secrets has the real values and can write them to a file, send them elsewhere or print them encoded,
which masking does not catch, so approve only commands you would run yourself; allowing secrets for an hour lets the agent run any
command with them for that hour. A value from `get_secret` is in the agent's transcript and logs. Details:
[docs/DESIGN.md](../docs/DESIGN.md) §9.4.2.

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
   `list_vaults`, `search_items` and `run_with_secrets`, never `get_secret` or `get_one_time_code`. omp shows nothing of a
   `run_with_secrets` command's output until it ends, so for a long command, one that asks for input or one that needs the shell's
   environment, the agent runs `hisar run --env-file .env -- <command>` in its shell instead.

Every `hisar` command omp runs, in its shell or as `hisar mcp`, gets the coding-agent rules
([docs/DESIGN.md](../docs/DESIGN.md) §9.4.4): it sees only the vaults available to agents, `hisar run` asks once per secret for each omp
session, `hisar read` and `hisar get` ask per item, and changes to items are refused. The approval names omp and the app it runs in
("omp in Tern wants to use 1 secret"). Approvals end when that omp exits or Hisar locks. ssh and git inside omp get only the SSH
keys in vaults available to agents. The app omp runs in, such as Tern, becomes an agent host: its other processes without a
terminal then ask on every connection, and it can no longer be a launcher.
