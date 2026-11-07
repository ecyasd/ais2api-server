# AIS2 API Server

An independent SillyTavern-oriented server distribution of AIStudio2API.
The upstream module path and directory layout are retained for future merges.

Base: **v0.3.1**, commit `80847a7665c442a238918c957963217773f36291`.
Upstream: <https://github.com/Mag1cFall/AIStudio2API>.

The server distribution uses its own Linux amd64 checks and fixed build identity
`v0.3.1+server.<commit>`, with `managedBy=ais2api-server`. Run the server image from
`deploy/Dockerfile.server`; mount the whole `/data` directory persistently. The
upstream Dockerfile and desktop release workflows are retained as upstream source,
and are not the server distribution's release entry points.

Upstream upgrades are deliberate version selections on an `upgrade/*` integration
branch. Merge the selected tag, run upstream checks and the server contracts,
build the image from the tested commit, and promote that exact artifact. Never
track upstream `main` automatically in production.

Implementation and migration acceptance are separate gates. A successful offline
test suite does not imply live Google or SillyTavern acceptance.

The shared policy is `internal/product/server.json`. It controls management
navigation, public model APIs, prompt/state databases, preview listeners,
runtime engine. Worker resources are ordinary runtime configuration, with defaults
of 1 warm worker, 4 active workers and 4 concurrent initializations. Process
environment variables override `.env`, and explicit saved values are preserved
across reloads and upgrades. The defaults are not product limits: valid values
such as 2 or 6 remain configurable. Four startup slots allow concurrent on-demand
initialization; the default warm target is still one account. Disabling the
preview listener also disables its port-conflict rule.

Account import follows the pinned upstream implementation unchanged. The server
profile does not rewrite Cookie input, replace account-add actions with pairing,
change native import defaults or hide native browser-discovery routes.

Both PG and the `-build` code assistant use the upstream SpiderMonkey runtime for
WAA scripts and the Go HTTP transport for generation. SpiderMonkey is an embedded
JavaScript engine, not a running Firefox/Camoufox browser. This browserless path
is inherited from AIS2 and enforced by the server profile. Explicit account login
is separate: the image includes the Linux libraries and virtual display needed
by the native, on-demand Camoufox login. Installing those dependencies does not
start a browser during service startup or generation.

OpenAI Chat Completions is the primary contract. Gemini generation, streaming,
model listing and counting remain available. Ordinary models use PG; `-build`
selects the independent code assistant. Tools are a first-version requirement:
declaration membership, mode, single-call requests, complete JSON, strict schema,
call/result IDs and thought signatures are checked in offline contracts.
OpenAI custom tools are rejected at request entry; the server's multi-turn tool
contract uses standard `function` tools. Completed Build tool handoffs terminate
the client turn without waiting for the upstream response to close, and cached
history must preserve each tool result's association with its call.

For a fixed thinking level in SillyTavern's Custom OpenAI connection, use its
additional request body, for example `reasoning_effort: low`. This also makes
PG/Build comparisons explicit: use the same underlying model and the same actual
thinking value. Do not infer the forwarded value from a client-side control that
may only apply to the client's recognized OpenAI models.

The native request ledger and usage aggregation remain upstream. A bounded
`server_request_timings` extension table links timings by request ID, separates
client-visible text from first semantic event, and records runtime attempts plus
internal upstream generation rounds. Late background cleanup updates use revisions
so older snapshots cannot replace terminal metadata. Diagnostic errors use fixed
summaries; raw upstream evidence is not persisted.

See [deployment, migration and final acceptance](../deploy/README.md).
