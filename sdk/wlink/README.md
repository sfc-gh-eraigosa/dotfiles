# wlink — WSL link (tunnel + resolver) management

**wlink = WSL link** — the link between this WSL box and the private network its fleet lives
on: the tunnel carrying it, the resolver that makes its names resolvable, and whether it is
currently usable.

Reads as a pair with [`fleet`](../fleet/README.md):

```
fleet status     # who my hosts are, and are they in sync
wlink status     # can I reach them by name from here
```

> Full design, spec, and plan: [`docs/mbo/`](../../docs/mbo/index.md), slug `wlink`
> (issue [#245](https://github.com/sfc-gh-eraigosa/dotfiles/issues/245)).

## `status --json` — the published contract

`gsl` renders this in the status line and CI can gate on it, so the shape is pinned by test
against the spec's worked example. All values below are illustrative.

```json
{
  "wsl": true,
  "link": "degraded",
  "tunnel": { "state": "up", "interface": "wg-lab", "handshake_age_seconds": null },
  "pinned": { "resolver": "10.10.0.1", "since": "2026-08-24T21:40:11Z", "managed": true },
  "candidates": [
    { "server": "10.10.0.1", "reachable": true, "fleet_resolved": 3, "recursive": true }
  ],
  "fleet": { "total": 3, "resolved": 3, "excluded_by_hosts_file": ["selfhost"] },
  "drift": null
}
```

| Field | Meaning |
| :-- | :-- |
| `link` | `ok` \| `degraded` — the one-word verdict; drives the exit code so consumers read one field instead of re-deriving the rules |
| `tunnel.handshake_age_seconds` | **Always `null` today** — wlink does not yet read the WireGuard handshake clock. Documented as a field so the shape is stable; populate it before relying on it. |
| `tunnel.state` | `up` \| `not-ready` \| `down` \| `unknown`. **`not-ready` is distinct from `down`**: Windows publishes a VPN adapter and its DNS server the moment you click connect, seconds before the handshake completes, and the old network is already unroutable by then |
| `pinned` | `null` when nothing is pinned. `managed: false` means `resolv.conf` names this resolver but wlink did not write it — someone else's pin, which wlink will not claim or silently take over |
| `candidates[].reachable` vs `fleet_resolved` | Deliberately separate: a resolver that answers but does not know the fleet is a *different* situation from one that says nothing, and only the first distinguishes "wrong tunnel" from "tunnel not ready" |
| `fleet.excluded_by_hosts_file` | Names `/etc/hosts` already answers. `nsswitch` is `files dns`, so no resolver is ever asked for them — they are not probed and do not count against the score |
| `drift` | `null` when the managed files match what wlink wrote |

Empty collections are `[]`, never `null`, so `.length` is always meaningful. Absences
(`pinned`, `drift`) *are* `null` — "not pinned" and "pinned to nothing" must not look alike.
