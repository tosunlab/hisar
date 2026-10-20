The standalone `hisar` command-line tool for Linux, macOS and Windows. Download the archive for your platform and `SHA256SUMS`, check it with `sha256sum -c --ignore-missing SHA256SUMS` (`shasum -a 256 -c --ignore-missing SHA256SUMS` on macOS), and put `hisar` on your PATH. With the Hisar app on a Mac, install `hisar` from Settings → Command-line tool instead.

Before extracting or running the archive, verify its keyless artifact attestation with a recent GitHub CLI.
Set `commit` to the full independently reviewed source commit SHA and `file` to the downloaded archive:

```sh
gh attestation verify "$file" --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

Use the same command for `SHA256SUMS` and `release-evidence.json`. Evidence binds exact-SHA tests and delivered file digests.
Checksums alone do not authenticate downloads. Attestations do not protect against a compromised authorized builder.
