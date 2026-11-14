# Evidence — retiring `ssh-key-sync.sh` (spec §6 parity gate)

Date: 2026-11-14. Host aliases are redacted (`nano`, `pi`, `gig`, `spark`, `pi2`) and
the login name is `<user>`.

## Parity: `fleet keys list` vs `ssh-key-sync.sh --list`

Both run on the same workstation, against the same `~/.ssh/config` (5 `#fleet` hosts,
no unmarked hosts), immediately one after the other:

```console
$ fleet keys list
warning: pi2: reading authorized_keys: exit status 255
id_ed25519:              nano, pi, gig, spark

$ ~/git/dotfiles/src/ssh-key-sync/ssh-key-sync.sh --list
id_ed25519: local, nano, pi, gig, spark
```

Same key, same authorizing hosts. The two differences are the script's defects, not
gaps in `fleet`:

- `pi2` had just dropped off the network (`No route to host`). `fleet` **names** the
  host it could not read; the script's `2>/dev/null` silently left it out (design §3,
  defect 4).
- The script adds a `local` column (this machine's own `authorized_keys`). `fleet keys
  list` is scoped to fleet hosts by design; the workstation is itself a fleet host here,
  so its row is the `spark` entry.

## Why retirement could not wait: the first-run incident

The same day, a machine (`pi2`) ran `fleet` for the first time. Onboarding found no key
pair, generated `~/.ssh/id_ed25519` (comment `fleet`), created an empty `~/.ssh/config`,
and the scan filled it with five hosts. Every host takes **keys only**, and nothing had
authorized the new key, so the machine got `Permission denied (publickey)` everywhere —
including to itself. The only advice on screen was "press A" (`ssh-copy-id`), which
needs a password none of the hosts accept. The 0-byte `config.bak-*` the scan left
(a backup of the empty file onboarding had created seconds earlier) looked like the old
config had been lost.

It was repaired by hand from a machine that still had access: read the new public key,
back up and append it on all five hosts, verify from the new machine. That procedure is
now `fleet keys enroll`, and the keys-only refusal names it everywhere `A`/`ssh-copy-id`
used to be suggested. The empty-backup skip and first-run self-authorization ship in
the same change.

## The defects being retired (re-confirmed against the script on 2026-11-14)

| Line | Defect | Re-confirmed |
| :-- | :-- | :-- |
| 63 | `scp "$P" "$P.pub"` copies the private key to every host | by reading |
| 18–27 | `--prune` overwrites every `authorized_keys` with this machine's `*.pub` only, no backup; joins keys with `$"\n"`, which bash writes as a literal backslash-n, so the result is one invalid line | `bash -c 'M+="a"$"\n"; …' \| od -c` → `a \ n b \ n` |
| 8 | `--delete` consumes the flag and falls through to generate+distribute | by reading |
| 43, 63 | `2>/dev/null` / `\|\| true` hide per-host failures | the parity run above |
