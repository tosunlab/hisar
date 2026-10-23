The docs site image `ghcr.io/tosunlab/hisar-docs:<version>` for linux/amd64 and linux/arm64: the static Docusaurus build served by nginx on port 8080, for docs.tryhisar.com.

Before deployment, download `release-evidence.json` and set `commit` to the full independently reviewed source commit SHA.
Verify the evidence and then its immutable image index digest with a recent GitHub CLI:

```sh
gh attestation verify release-evidence.json --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
digest=$(jq -r .image.digest release-evidence.json)
gh attestation verify "oci://ghcr.io/tosunlab/hisar-docs@$digest" --repo tosunlab/hisar \
  --signer-workflow tosunlab/hisar/.github/workflows/release-docs.yml \
  --signer-digest "$commit" --source-digest "$commit" --deny-self-hosted-runners
```

Deploy `ghcr.io/tosunlab/hisar-docs@$digest` after verification, not a mutable tag.
