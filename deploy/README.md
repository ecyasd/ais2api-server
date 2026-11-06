# Server distribution and promotion

The supported server target is **Linux amd64**. This repository is an independent
distribution based on upstream AIS2 v0.3.1 (`80847a7665c442a238918c957963217773f36291`).
See [product boundary](../docs/server.md). The upstream Go module name and native
account/state formats are retained intentionally.

## Build and release

`Server CI` runs frontend type, lint, format and production build checks; all
upstream Go unit tests and vet; dedicated tool/account/telemetry contracts; backup
recovery checks; a static Linux binary build; isolated process restart/auth/import
acceptance; and the browser-free Docker image build. All synthetic tests use
separate temporary directories. The process smoke test points its outbound proxy
at a closed loopback port and never starts generation.

`server-v*` tags trigger the same checks and publish an immutable candidate release
containing binary, image archive, SHA-256 checksums and commit provenance. Tags
must point at the reviewed server commit. Release publication does not change a
running service. Upstream desktop workflows remain in source but are disabled in
this fork. Windows/macOS desktop packages are outside this server release.

`Promote fixed server release` is the controlled delivery entry point after
acceptance. Dispatch it from `main` with the reviewed release tag. It uses the
existing GitHub OIDC role and one-node SSM document; it requires no long-lived AWS
key. Repository variables are `AWS_DEPLOY_ROLE_ARN`, `AWS_DEPLOY_REGION`,
`AWS_DEPLOY_DOCUMENT` and `AWS_DEPLOY_NODE`. The delivery template reuses those
existing resources and replaces the allowed repository identity with this fork.

The root-owned `host_deploy.py` is installed separately as
`/usr/local/sbin/ais2api-server-deploy`. It only accepts a tag, commit and archive
checksum from the dedicated document, and verifies the fixed repository's current
reviewed `main`, release provenance, image platform, health and independent auth.
It stops the current service before starting the replacement. A failed upgrade
switches the image back while retaining the latest account/config/statistics data.
The stable Compose path is `/srv/ais2api-server/compose.server.yml`, its private
image selection is `/etc/ais2api-server/image.env`, and the existing protected
`aistudio-api.service` unit controls the new server. Its traffic start gate and
the host traffic timer remain enabled. Release packages cannot overwrite the
root updater, service unit, Compose policy or mutable data.

Load the verified archive with `docker load`, then set `SERVER_IMAGE` to the exact
tested tag/digest, `SERVER_DATA` to a dedicated persistent directory, and use
`docker compose -f deploy/compose.server.yml up -d`. The management endpoint is
bound to loopback for the existing authenticated HTTPS gateway. Stop the currently
running API before starting a candidate. Keep only one product service running;
use a separate data directory for synthetic checks, then migrate before cutover.
The Linux Compose deployment uses host networking with an explicit loopback bind,
so starting or replacing the service does not create bridge/veth interfaces that
invalidate host traffic accounting. Keep the host's existing traffic protection
enabled. Verify the network policy before any candidate start or replacement.

Place `.env` **inside the persistent data directory**, using `server.env.example`
as a template. Supply separate administrator and model credentials. A missing
management password prevents startup; empty model keys deny model access. Do not
put mutable model keys in container environment variables, as environment values
override the management page. `AUTO_START=false` preserves explicit owner control
over account activation. Imported accounts are saved disabled, without network
verification; enabling an account or starting generation is a separate operation.
Cookie import uses AIS2's native account recognition, multiple file selection,
account checkboxes and batch task list. Raw Cookie discovery contacts Google only
after the owner supplies import data; it does not start a browser or call a model.
Complete native exports keep their identity and environment metadata. Existing
accounts can be selected to replace credentials without deleting the account.

## Data inventory and consistent recovery

Inventory these before migration: `.env`, native `auth/<account>/` directories,
source-PC pairing/renewal material, native `runtime/requests.db`, the independent
timing table and account metadata, plus the old service's retained statistics.
Keep private data and backups outside Git and release artifacts.

Prefer original complete exports or native AIS2 state. Simplified legacy Cookie
name/value maps have already lost domain/path/expiry/security attributes; the
Cookie-header importer marks these as partial. Do not label them lossless. The
AIS2 import API accepts a complete `storage_state` envelope with locale, timezone
and auth-user; the UI supports pasted input and multiple files. Full source
extensions and pairing renewal material remain native. The server does not renew
a revoked Google login merely by refreshing a WAA signature. Origin-PC pairing
helpers run on the source Windows machine and push replacement credentials.

Stop the candidate before backup so account files share one consistency point:

```sh
docker compose -f deploy/compose.server.yml stop api
python3 deploy/data.py backup --source /path/to/data --destination /path/to/private-backup --service-stopped
python3 deploy/data.py verify --source /path/to/private-backup
python3 deploy/data.py restore --source /path/to/private-backup --destination /path/to/empty-recovery-check
docker compose -f deploy/compose.server.yml start api
```

The backup uses SQLite's snapshot API and bounded-memory file copying, records
integrity metadata privately, and never prints credential values. Restore only
targets a separate empty directory. Verify account count/configuration, newest
Cookie state, statistics and DB integrity in an isolated recovery instance.

**Code rollback** switches the image only and continues mounting the latest
data directory. It does not invoke restore or replace current Cookies. Data
restoration is a separate recovery operation for corruption/incompatibility.

## Final acceptance and cutover

Before changing the HTTPS gateway or removing any legacy repository:

- Verify candidate health, independent auth, bounded metadata and no browser
  processes/preview listener; import, restart, upgrade and rollback preserve data.
- Connect a separate SillyTavern profile using Chat Completion / Custom OpenAI,
  base URL ending in `/v1`, a current model key and Function Calling enabled.
- Check PG and `-build`: system/history/continuation, ordinary incremental text
  with tool declarations, complete tool-call/result/continuation/signature round
  trip, cancellation, failures and terminal markers.
- With owner-authorized real requests, compare cold and warm runs using the same
  underlying model and actual thinking level. Use request IDs and persisted phase
  timings; no timing or speed-up claims may be inferred from synthetic responses.
- Verify the gateway can return to the previous service and that rollback only
  switches code. Keep the previous image while validating the new service.
- Switch only after the acceptance evidence is complete. Remove the exact legacy
  local/GitHub repositories only after final acceptance, while retaining all
  required live data and recovery artifacts outside those repositories.

Existing native usage history stays intact. New diagnostics retain at most 10,000
requests for 7 days; live logs and write queues are bounded. Diagnostics never
persist prompt/body/thought/signature/Cookie values.
