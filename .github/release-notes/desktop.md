**macOS:** open `Hisar-<version>.dmg` and drag Hisar into Applications.

**Linux (amd64, arm64):** `sudo apt install ./Hisar-<version>-linux-<arch>.deb` on Ubuntu 24.04+ or Debian 13+, or `sudo dnf install ./Hisar-<version>-linux-<arch>.rpm` on Fedora 40+. It needs a Secret Service (GNOME Keyring or KWallet) for the Secret Key. `hisar` comes with it at `/usr/bin/hisar`. On other distributions with GTK 4 and WebKitGTK 6.0 (Arch, openSUSE and others), `chmod +x Hisar-<version>-linux-<arch>.AppImage` and run it; it copies `hisar` to `~/.local/share/hisar/bin`, and Settings → Command-line tool links it onto your PATH. Or install the Flatpak, which brings GTK and WebKitGTK with the GNOME runtime from Flathub: `flatpak install --user Hisar-<version>-linux-<arch>.flatpak`, then `flatpak run com.tryhisar.hisar`.

**omp and Tern:** connect omp in Settings → Developer tools → Coding agents. Every `hisar` command omp runs follows the coding-agent rules, so if you already use `hisar` from omp, expect more approvals: one per secret for each omp session. ssh and git inside omp get only the SSH keys in vaults available to agents, so move a key omp needs, such as your GitHub key, into one of them. The app omp runs in, such as Tern, becomes an agent host: its other processes without a terminal ask on every connection, and it can no longer be a launcher.

**Claude Code and Codex:** connect them in Settings → Developer tools → Coding agents. Hisar adds its MCP server, `hisar mcp --no-reveal`, to Claude Code's user settings or to Codex's `config.toml` with a tool timeout long enough for approvals, and never changes a server named `hisar` that you added yourself. Commands they run in their shells follow the coding-agent rules, as omp's do.

Check downloads against the `SHA256SUMS-*` files: `sha256sum -c --ignore-missing SHA256SUMS-linux-amd64`.

Before installing, verify each downloaded DMG, zip, Linux package, AppImage, or Flatpak with a recent GitHub CLI.
Set `commit` to the full independently reviewed source commit SHA and `file` to the downloaded file:

```sh
gh attestation verify "$file" --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

Verify checksum files, `update.json`, and `release-evidence.json` the same way. The update asset is attested before its
publication rename, with identical bytes. macOS signing and notarization remain separate checks.
Attestations identify workflow and source, not protection against a compromised authorized builder.
