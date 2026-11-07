# Server worker retention

The server keeps one prepared account by default and may expand to four active
workers. The idle reaper closes **surplus** workers after 30 minutes without
use. It never reduces the warm pool below `WARM_WORKER_LIMIT`, never deletes
an account or its cookies, and retains the existing busy-worker/lease checks.
Capacity pressure, authentication failure, explicit stop or a reduced active
limit can still replace/close a worker before the idle timeout.

`WORKER_IDLE_TIMEOUT=30m` overrides this retention. It accepts Go durations
of at least one second; missing or invalid values use 30 minutes. This is a
**process environment** setting, not a management-form or `/data/.env` key.
Do not put it in the application-managed `.env`: unknown keys are removed
when settings are saved. Compose passes it into the container explicitly.
For the fixed host deployment, set it in `/etc/ais2api-server/image.env`
(the Compose interpolation environment), then recreate the service container.
For a directly executed binary, export it before starting the process.
The reaper checks at most every 30 seconds; expiry does not imply execution
at an exact wall-clock instant.

Recommended application settings in persistent `/data/.env`:

```dotenv
AUTO_START=true
WAA_BACKEND=spidermonkey
WARM_WORKER_LIMIT=1
MAX_ACTIVE_WORKERS=4
WARM_STARTUP_CONCURRENCY=4
```

Existing saved settings are not overwritten by an image upgrade. Apply the
above non-secret settings via management or the private host configuration,
preserving administrator credentials, API keys and all account directories.
Update the root-owned host updater separately to include PR #6 fixes.
The final warm account is retained, including after 30 idle minutes.
PG and Build still use the local SpiderMonkey path, not a remote signer.
