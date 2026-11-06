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
