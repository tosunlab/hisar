---
sidebar_position: 3.5
sidebar_label: Agent website sign-in
title: Let computer-use agents sign in to websites with Hisar
description: Set up Hermes, Claude Code, Codex, or another MCP client to request website sign-ins through Hisar for Mac or Linux.
---

# Let computer-use agents sign in to websites with Hisar

A computer-use agent can ask Hisar for Mac or Linux to sign in to a website instead of putting a password in its tool arguments. You approve the website and login. Hisar fills and submits the form in your browser, then returns the outcome and login metadata, not the password or one-time code.

Use this with Hermes, Claude Code, Codex, or any MCP client that can control native windows through [cua-driver](https://cua.ai/docs/cua-driver) on the Mac, or through the desktop's window tools on Linux. The agent needs both Hisar's MCP server and computer-use tools.

> **The browser still receives the credentials.**
>
> The `sign_in` tool returns no password or one-time code. That does not prevent page scripts, screenshots, recordings, or accessibility reads from exposing values entered into the browser. Hisar attempts cleanup, but does not guarantee deletion. Read [the limits](#limits) before you enable it.

![Hisar asks to let Hermes in Terminal sign in to tosunlab.com in Google Chrome with the Tosun Lab login from the Agents vault.](/img/agent-sign-in/01-approval-sign-in.png)

The screenshots on this page use the desktop frontend preview and sample accounts.

## How it works

1. The agent opens the website's sign-in form in a supported browser. It gets the browser window's `pid` and `window_id` from cua-driver's `list_windows` on the Mac, or the browser's process ID and X11 window ID on Linux.
2. The agent calls Hisar's `sign_in` tool with those IDs. Hisar reads the website address from the browser's accessibility tree, not from an address supplied by the agent.
3. Hisar finds matching logins in the vaults available to agents. Unless an existing approval covers the login and website, Hisar asks you to approve.
4. Hisar rechecks the window, website, and login. It enters the username, password, and authenticator code as each step asks for them, and submits each step while the website stays the same.
5. Hisar attempts to clear remaining entries. The agent gets the outcome, cleanup evidence, and login metadata so it can continue or tell you what stopped the sign-in.

## Set up Hisar

1. Open Hisar and unlock it.
2. Open **Settings → Developer tools → Coding agents**.
3. Under **Vaults for coding agents**, turn on **Available to agents** for the vault that contains the login. Other vaults stay invisible to Hisar's agent tools. A separate vault for agent accounts lets you choose which accounts to expose.
4. Under **Website sign-in**, turn on **Let agents sign in to websites in your browser**. It is off by default.
5. On the Mac, if Hisar says **Accessibility permission needed**, click **Open System Settings**. Turn on Hisar in **Privacy & Security → Accessibility**. On Linux, start the browser as [Linux](#linux) shows.

![Vaults for coding agents. The Agents vault is available to agents, while Personal and Team stay invisible.](/img/agent-sign-in/10-settings-agent-vaults.png)

Save the site's address in the login's website list. Matching uses your **Settings → Browser → Only fill on the exact host** choice. With that switch off, a login can match sibling hosts under the same registrable domain. With it on, the host must match exactly. Matching also checks the scheme and port. The page must use HTTPS, except for HTTP on `localhost` or a loopback address.

cua-driver needs its own macOS permissions. Follow its [quickstart](https://cua.ai/docs/cua-driver/quickstart) for installation and permission setup.

## Connect your agent

### Hermes

1. Enable Hermes' computer-use tools.

   ```sh
   hermes tools enable computer_use
   ```

2. In Hisar's **Coding agents** settings, click **Connect** next to **Hermes**. Hisar adds its MCP server to the configuration that `hermes config path` reports.
3. Start a new session with `hermes chat`. In a running session, use `/reload-mcp` instead.

![Hermes is connected to Hisar. Its card explains how to reload MCP and include Hisar when starting a session with -t.](/img/agent-sign-in/08-settings-hermes-connected.png)

If you start Hermes with `-t`, the list must name `hisar`, `all`, or `*` to load Hisar. For example, this loads computer use and Hisar together.

```sh
hermes -t computer_use,hisar chat
```

Ask Hermes to open the sign-in page with native `computer_use` window controls and call Hisar's `sign_in`. Do not use a CDP `browser` or `browser_exec` tool for this flow. Hisar refuses browsers started with remote debugging.

If computer use is unavailable, run `hermes computer-use doctor` and follow its diagnostics.

### Claude Code and Codex

1. Install cua-driver through its [quickstart](https://cua.ai/docs/cua-driver/quickstart).
2. Run the registration helper for your agent.

   ```sh
   cua-driver mcp-config --client claude
   ```

   For Codex, use `cua-driver mcp-config --client codex`. Run the registration command that the helper prints.
3. In Hisar's **Coding agents** settings, click **Connect** next to **Claude Code** or **Codex**.
4. Start a new agent session. Ask it to open the sign-in page through cua-driver's native window controls, then call Hisar's `sign_in`.

Hisar's **Connect** button registers Hisar, not cua-driver. You need both registrations.

### Other MCP clients

Register `cua-driver mcp` and `hisar mcp --no-reveal` as separate MCP servers. Use native window controls rather than a CDP browser tool. Give the Hisar tool call a timeout of at least five minutes so it can wait for the approval and sign-in.

See the [command-line guide](./apps/cli.md) for MCP configuration examples.

## Review the approval

The dialog names the agent, where it runs, the website, and the browser. It shows the login's vault, title, and username. Check these before you approve with Touch ID, your master password, or a verified answer from your phone.

If several logins match, choose one on the Mac. Selecting a login does not approve it. The phone can approve a sign-in with one login, and its approval is for this time only.

![The approval offers two saved Tosun Lab logins, with user@example.com and ops@example.com as sample usernames.](/img/agent-sign-in/03-approval-pick-login.png)

Without Touch ID, Hisar asks for your master password. You can also choose **or use your master password** in a Touch ID dialog. Enter it in Hisar, never in the agent's chat.

![The sign-in approval requires the master password when Touch ID is unavailable.](/img/agent-sign-in/04-approval-master-password.png)

### Choose how long to allow it

**Only this time** is the default. It covers that one call. The next `sign_in` asks again, even on the same website.

A longer approval covers only the chosen login at that exact website's scheme, host, and port. It does not approve another login or another website.

![The open duration menu offers Only this time, For 1 hour, For 1 day, and Always.](/img/agent-sign-in/02-approval-duration-menu.png)

For omp, Claude Code, Codex, and Hermes, **For 1 hour**, **For 1 day**, and **Always** can cover the same agent program across sessions and Hisar restarts. Hisar must identify the program and be able to save the approval in its Keychain-backed store. The approval belongs to that program, not to every process with the same agent name. For an interpreted agent, the identity also includes its script path or inline code.

Otherwise, the menu offers only **Only this time** and **For 1 hour**. The hour ends sooner if the agent session or connection ends. Changing the duration or login while Touch ID waits starts a new prompt for that choice.

### Revoke an approval

Open **Settings → Developer tools → Coding agents → Allowed without asking**. Each row names what was allowed, who holds the approval, and when it expires. Click **Revoke** to make that agent ask again.

![Website sign-in is enabled with Accessibility allowed. Allowed without asking lists a Tosun Lab sign-in for Hermes with an expiry and a Revoke button.](/img/agent-sign-in/06-settings-website-sign-in.png)

Signing out of Hisar ends these approvals. Locking keeps them, but Hisar must unlock before it can sign in. A `cleanup-failed` result ends the approval for that login and website. If Hisar cannot remove a saved approval while locked or cannot save the removal, it drops all saved approvals instead. Storage failures are reported.

**Running agents** lists sessions that have used Hisar. Revoking a session ends its approval and what you allowed only for that session. Approvals that cover the program across sessions stay until they expire, you revoke them under **Allowed without asking**, or you sign out.

![Running agents lists Hermes in Terminal for the infra project, with a remembered Tosun Lab sign-in and a Revoke button.](/img/agent-sign-in/11-settings-running-agents.png)

## Supported browsers

Hisar for Mac supports **Brave Browser**, **Google Chrome**, and **Microsoft Edge**. Tested with Brave, Chrome 156 and Edge 156 on macOS 26.

Safari, Arc, Firefox, and other browsers are not supported. On the Mac, the browser window must be on the current Space and not minimized. Use a browser started normally, without remote debugging. Hisar runs one sign-in at a time.

### Linux

Hisar for Linux supports **Google Chrome**, **Chromium**, **Brave**, and **Microsoft Edge** installed by their packages, on X11 or under XWayland in a Wayland session. Tested with Debian's Chromium 156 on X11. A Flatpak or Snap browser, and a browser window that runs natively on Wayland, are not supported.

Start the browser with accessibility on, or Hisar cannot read its pages:

```sh
ACCESSIBILITY_ENABLED=1 google-chrome --force-renderer-accessibility
```

Use `chromium`, `brave-browser`, or `microsoft-edge` in place of `google-chrome`. The browser window must be the active window: Hisar types the login into the focused field and checks before every key that the window, field, and website are still the same. Do not type or switch windows while it signs in. **While Hisar types, any X11 program on your display can read the keys.** A password with a character your keyboard layout types only with AltGr or a dead key is refused before Hisar types anything.

## What the agent gets back

The tool returns the website and browser Hisar read, the login's ID, vault, title, and username, and the parts entered on each step. It also returns an outcome and cleanup evidence. It does not return the password, one-time code, or authenticator secret.

- `submitted` means Hisar submitted the last step and the form left the page. The agent checks where the site went. It is not proof that every site-specific sign-in step is complete.
- `not-accepted` means the site kept its sign-in form or asked for the password or code again. The agent tells you and does not retry.
- `stopped` means the page needs something Hisar does not fill, or moved to another website. Handle that step before another call.
- `cleanup-failed` means Hisar could not confirm cleanup. The agent must stop and tell you to check the browser window.

### Commands that need secrets

Website sign-in differs from `run_with_secrets`, which gives real secret values to a command. Unless an existing use approval covers the secrets, Hisar asks you to approve the command, working directory, and secrets. A longer use approval lets the agent run other commands with those same secrets until it ends.

![Hermes asks to run a database migration with two secrets. Hisar shows the command, working directory, and secret names before approval.](/img/agent-sign-in/05-approval-run-with-secrets.png)

See the [command-line guide](./apps/cli.md) for commands that use secrets.

## Limits

Hisar keeps credential values out of the `sign_in` response. It does not isolate those values from the website or from an agent that can observe and control your computer.

- Page scripts and show-password controls can reveal an entered password. Hisar enters passwords only into fields that macOS reports as secure text fields, or AT-SPI as password fields on Linux. Browsers normally mask them with dots, which can reveal the password's length.
- Screenshots, recordings, and accessibility reads can capture a one-time code while the page retains it. Hisar does not pause the agent's other tools during sign-in.
- Cleanup evidence is not a deletion guarantee. `fields-cleared` means the fields read back empty through Accessibility. `fields-removed` means the fields left the accessibility tree while their document stayed, not that the page erased their values. `cleanup-failed` means cleanup could not be confirmed.
- Retained fields, page-script copies, captures, browser-owned storage, and unrecognized save-password prompts have no Hisar-enforced deletion limit. The 15-second deadline after each submit and separate five-second cleanup timeout are not credential-retention limits. Browser Accessibility calls can finish after those deadlines.
- Hisar fills a username, password, and a code from the login's authenticator secret. You or the agent must handle SMS or email codes, passkeys, account choosers, and CAPTCHAs.
- Hisar refuses forms inside cross-origin frames, sign-up and change-password forms with multiple password fields, and browsers started with remote debugging.
- An agent that operates Hisar's own unlocked window, and other programs running as you, are outside this protection.

Chromium can temporarily expose no page through Accessibility. Bring a background or covered browser window to the front on the current Space before calling `sign_in`. If a pre-entry refusal still says the window shows no website, call once more. On `cleanup-failed`, stop and tell the user.

Read the website sign-in section of the [security model and design](./design.md) for the transaction checks and threat model.

## Troubleshooting

| What happens | What to do |
|---|---|
| Hisar says Website sign-in is off. | Turn on **Website sign-in** in **Settings → Developer tools → Coding agents**. |
| Hisar needs Accessibility permission. | Turn on Hisar in **System Settings → Privacy & Security → Accessibility**, then return to Hisar. |
| Linux: the browser was started without accessibility. | Quit it and start it with `ACCESSIBILITY_ENABLED=1` and `--force-renderer-accessibility` ([Linux](#linux)). |
| Linux: Hisar needs X11. | Use an X11 session, or run the browser under XWayland. |
| Linux: the window is not the active window. | Bring it to the front, then request sign-in again. |
| Hisar finds no login for the website. | Check the login's saved website and make its vault **Available to agents**. Do not paste the password into chat. |
| Hermes has no Hisar tools. | Click **Connect**, then start `hermes chat` or use `/reload-mcp`. A `-t` list must include `hisar`, `all`, or `*`. |
| Claude Code or Codex has no window controls. | Register cua-driver with its `mcp-config` helper. Hisar's **Connect** button does not register it. |
| Hisar refuses a debuggable browser. | Quit that browser and start it normally, or use another normally started supported browser. Use native window tools, not CDP. |
| The window is on another Space or minimized. | Bring it to the current Space and restore it. Ask the agent to list windows again if the IDs are stale. |
| The page or login changed during approval. | Return to the sign-in page, then request sign-in again. Hisar rechecks the destination and login. |
| The page needs an account choice, CAPTCHA, SMS code, or email code. | Handle that step. Call `sign_in` again only when a supported sign-in field shows. |
| The site does not accept the login. | Check the saved login yourself. The agent must not retry `not-accepted`. |
| Cleanup fails. | Stop the agent and inspect the browser window. Do not repeat `sign_in` to dismiss the warning. |
| Hisar is unavailable or asks for an update. | Open or update Hisar. Keep **Settings → Developer tools → Let the command-line tool use Hisar** on. |

The [command-line guide](./apps/cli.md) lists the exact refusal messages.

## For agent and driver authors

Call `sign_in` on `hisar mcp` with `pid` and `window_id` from cua-driver's `list_windows` on the Mac, or the X11 window ID and its `_NET_WM_PID` on Linux. The optional `login` field takes an item ID from `search_items`. Call while the sign-in form shows, without typing credentials into it first.

Wait for the result before deciding what to do next. Inspect both `outcome` and `cleanup`. Treat a denial and `not-accepted` as final. Stop on `cleanup-failed`. Never ask the user to paste a password into chat.

The [command-line guide](./apps/cli.md) has the full input and result contract, timing, cleanup definitions, and retry rules.
