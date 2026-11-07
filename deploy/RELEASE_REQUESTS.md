# Reviewed release requests

Changing `release-request.json` in a reviewed PR explicitly requests a fixed
release from the merged main commit. The existing full Server CI workflow runs
again against that exact merged commit before publishing. Set `deploy` to true
only when the operator has approved production deployment; this dispatches the
existing `server-deploy.yml` private promotion workflow. False publishes only.
Tag-push releases remain supported and do not automatically deploy.

Standalone CI and release verification use separate workflow concurrency groups,
so the two checks triggered by one reviewed main push cannot cancel each other.

The flow refuses a changed main head and does not overwrite a release belonging
to another commit. A rerun may reuse a complete release for the same commit.
No administrator password, Cookie, remote terminal command or cloud credential
is placed in the manifest. Existing AWS OIDC/SSM permissions remain unchanged.
The private updater is still code-only: it does not silently rewrite application
settings. Apply the non-secret runtime settings through the authorized management
interface or private host before verifying the final active configuration.
