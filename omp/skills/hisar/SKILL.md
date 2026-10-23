---
name: hisar
description: Pass Hisar secret references to approved commands without copying values into chat. Commands receive real secrets; output masking is not containment. Read before using a Hisar tool or running any hisar command, or when a task needs a credential.
---

# Hisar

Hisar keeps the user's secrets. This plugin connects omp to Hisar's MCP server, whose tools omp names `mcp__hisar_list_vaults`, `mcp__hisar_search_items` and `mcp__hisar_run_with_secrets`. omp may offer them as tool devices instead: write the JSON arguments to `xd://mcp__hisar_run_with_secrets` and so on. List and search return metadata and references, not secret values. A command receives the real values in its environment or on its standard input. Hisar masks recognized exact values in output, but values shorter than 4 bytes, transformed output and other disclosure channels are not protected. Each use opens an approval window in the Hisar app, and the user decides there.

Never print a secret. The plugin blocks `hisar read`, `hisar get`, `hisar export` and `hisar run --no-masking`.

Masking is not containment. Run only the intended command, and never use output, a file or a network request to reveal a granted secret. `--no-reveal` removes direct value-returning MCP tools, not the command's ability to disclose values. The [command runner](../../../cli/mcpserver/run.go) and [masker](../../../cli/internal/masking/masking.go) define these limits.

## Find the reference

A reference is `hisar://<vault>/<item>/<field>`. Call `search_items` with a title, username, URL or vault name. It returns each field's label with its reference, so copy the reference rather than building it. `list_vaults` lists the vaults the agent can see.

Without the tools, `hisar vaults` lists the vaults and `hisar ls -v Agents` the items in one. A reference built by hand percent-encodes spaces and slashes in a name, so the item `Deploy token` becomes `Deploy%20token`. The field is `password`, `username`, `code` for the one-time code, or the name of a custom field.

## Run a command with secrets

Call `run_with_secrets`. Give exactly one of:

- `command`, the program and its arguments, which run without a shell: `["npm", "run", "deploy"]`.
- `shell`, a string `/bin/sh -c` runs, for pipes, `&&` and redirects: `"npm ci && npm run deploy"`.

Then give the secrets in one or more of:

- `env`, each variable mapped to a reference: `{"TOKEN": "hisar://Agents/Deploy%20token/password"}`.
- `envFiles`, paths of `.env` files whose values are references, as `hisar run --env-file` reads them. A later file wins, and `env` wins over the files.
- `stdin`, literal text or one whole reference, for a command that reads a secret from its input: `docker login --password-stdin -u deploy registry.example.com` with `stdin` set to the password's reference.

`workingDirectory` defaults to the project. The tool returns the exit code and the masked output when the command ends, up to 64 KiB of each stream.

## Run it in the shell instead

`run_with_secrets` gives the command no terminal, and omp shows nothing of its output until it ends. For a command whose live output you need to act on, one that asks for input, or one that needs the shell's own environment (variables set earlier, an activated virtualenv), run `hisar run` in `bash`:

```sh
TOKEN=hisar://Agents/Deploy%20token/password hisar run -- ./deploy.sh
hisar run --env-file .env -- npm run dev
```

`--env-file` repeats, and a later file wins.

## Wait for the approval

The user approves in the Hisar app or on their phone, which can take two minutes. `run_with_secrets` waits for the approval without a time limit in omp, and its `timeoutSeconds` (default 120, at most 3600) counts from when the command starts. For `hisar run` in `bash`, set the tool's `timeout` to at least 180 seconds plus the time the command itself needs.

## When a reference is not found

The agent sees only the vaults the user turned on for coding agents. A vault or item that is "not found" may be in a vault that is not turned on. Tell the user so, and offer `/hisar`, which opens Hisar where they turn vaults on. Do not guess other names or retry with other vaults.

## When the user denies a use, or Hisar is locked

A denied use stays denied: tell the user, and do not retry the same request. A locked Hisar asks the user to unlock it in the approval window, so run the command anyway. `hisar run` starts a Hisar that is not running. When Hisar is signed out, ask the user to sign in to the Hisar app.
