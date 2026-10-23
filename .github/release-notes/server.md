The hisar-server image `ghcr.io/tosunlab/hisar-server:<version>` for linux/amd64 and linux/arm64. [deploy/README.md](https://github.com/tosunlab/hisar/blob/main/deploy/README.md) says how to run and upgrade it.

Before deployment, download `release-evidence.json` and set `commit` to the full independently reviewed source commit SHA.
Verify the evidence and then its immutable image index digest with a recent GitHub CLI:

```sh
gh attestation verify release-evidence.json --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
digest=$(jq -r .image.digest release-evidence.json)
gh attestation verify "oci://ghcr.io/tosunlab/hisar-server@$digest" --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release-server.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

Authenticate to GHCR when required. Deploy `ghcr.io/tosunlab/hisar-server@$digest` after verification, not a mutable tag.
The attestation covers the multiarchitecture image index. It does not protect against a compromised authorized builder.

Vault key changes now use bounded staging and atomic activation. Update the apps and CLI before removing vault members or deleting a vault while moving its items. Updated callers require a server with staged transitions and do not fall back to the old aggregate route.

Desktop versions through 0.1.12 and CLI versions through 0.1.6 can still read rotated vaults but cannot change their items, names, grants or deletion state without an explicit key generation. The server returns an instruction to update Hisar. Generation omission remains supported only for never-rotated nonorganization vaults.
