# wol — Wake-on-LAN utility (`sdk/wol`)

`wol` sends Wake-on-LAN magic packets to power on hosts by MAC address.

- **Module path:** `github.com/sfc-gh-eraigosa/dotfiles/sdk/wol`
- **Binary:** `wol` (installed to `~/opt/bin/wol` by `install.sh`)
- **External install:** `go install github.com/sfc-gh-eraigosa/dotfiles/sdk/wol@<tag>` (tags: `sdk/wol/vX.Y.Z`)
- **Build:** `bash sdk/wol/build.sh` (injects version via `-ldflags -X .../sdk/wol/cmd.*`)
- **Usage:** `wol <MAC>` (run `wol --help` for options).
