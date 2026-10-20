The Hisar browser extension <version> for Chrome and other Chromium browsers. Unzip `hisar-extension-<version>.zip`, open `chrome://extensions`, turn on Developer mode, remove the old Hisar extension and choose Load unpacked. Pairing with the Mac app is kept, because the extension ID is fixed.

Before unpacking, verify the zip with a recent GitHub CLI. Set `commit` to the full independently reviewed source commit SHA:

```sh
gh attestation verify "hisar-extension-<version>.zip" --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

Verify `release-evidence.json` with the same flags. Attestations identify workflow and source, not protection against
a compromised authorized builder.
