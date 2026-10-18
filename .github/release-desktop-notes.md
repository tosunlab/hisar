**macOS:** open `Hisar-<version>.dmg` and drag Hisar into Applications. The app is not notarized yet: on first launch, right-click Hisar → Open.

**Linux (amd64, arm64):** `sudo apt install ./Hisar-<version>-linux-<arch>.deb` on Ubuntu 24.04+ or Debian 13+, or `sudo dnf install ./Hisar-<version>-linux-<arch>.rpm` on Fedora 40+. It needs a Secret Service (GNOME Keyring or KWallet) for the Secret Key. `hisar` comes with it at `/usr/bin/hisar`. On other distributions with GTK 4 and WebKitGTK 6.0 (Arch, openSUSE and others), `chmod +x Hisar-<version>-linux-<arch>.AppImage` and run it; it copies `hisar` to `~/.local/share/hisar/bin`, and Settings → Command-line tool links it onto your PATH.

Check downloads against the `SHA256SUMS-*` files: `sha256sum -c --ignore-missing SHA256SUMS-linux-amd64`.
