---
name: hisar
description: Use the user's secrets from Hisar (passwords, API tokens, keys) in a command without seeing them. Read before running any hisar command or when a task needs a credential.
---

# Hisar

Hisar keeps the user's secrets. `hisar run` resolves `hisar://` references in a command's environment, starts the command with the values, and masks them in its output. The values never enter the transcript. Each use opens an approval window in the Hisar app, and the user decides there.

Never print a secret. The plugin blocks `hisar read`, `hisar get`, `hisar export` and `hisar run --no-masking`.

## Find the reference

A reference is `hisar://<vault>/<item>/<field>`. Percent-encode spaces and slashes in a name, so the item `Deploy token` becomes `Deploy%20token`. The field is `password`, `username`, `code` for the one-time code, or the name of a custom field.

```sh
hisar vaults        # the vaults the agent can see
hisar ls -v Agents  # the items in one vault: title, username, URL, type
```

## Run a command with a secret

Set each variable to a reference, then put the command after `--`:

```sh
TOKEN=hisar://Agents/Deploy%20token/password hisar run -- ./deploy.sh
```

When the project has a `.env` file whose values are references, pass it instead:

```sh
hisar run --env-file .env -- npm run deploy
```

`--env-file` repeats, and a later file wins.

## Wait for the approval

The user approves in the Hisar app or on their phone, which can take two minutes. Set the `bash` tool's `timeout` to at least 180 seconds plus the time the command itself needs.

## When a reference is not found

The agent sees only the vaults the user turned on for coding agents. A vault or item that is "not found" may be in a vault that is not turned on. Tell the user so, and offer `/hisar`, which opens Hisar where they turn vaults on. Do not guess other names or retry with other vaults.

## When Hisar is locked or not running

Run the command anyway. A locked Hisar asks the user to unlock it in the approval window, and `hisar run` starts a Hisar that is not running. When Hisar is signed out, ask the user to sign in to the Hisar app.
