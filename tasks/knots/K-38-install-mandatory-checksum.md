# K-38 — install.sh: mandatory checksum verification

- Wave / Step: 7.1
- Effort: S
- Risk: high (current behavior is silently unsafe)
- Deps: —
- Files: scripts/install.sh

## Goal

Today's [install.sh:34](scripts/install.sh:34) silently skips checksum
verification if the checksums file fails to download:

```sh
if curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"; then
  expected="..."
  ...
fi
```

A transient network blip — or an active MITM at the CDN edge that fails one
request and serves another — bypasses verification. This must be mandatory.

## Acceptance

- `install.sh` aborts with a clear error if `checksums.txt` cannot be
  downloaded.
- `install.sh` aborts if the checksum line for the asset is missing.
- `install.sh` aborts on mismatch (already does, leave that path).
- New flag `THIMBLE_INSTALL_NO_VERIFY=1` exists for emergency reinstalls but
  prints a multi-line warning before proceeding.
- README "Install" section gains a one-line note that verification is
  mandatory by default.

## Notes

This is the highest-leverage fix in Wave 7. Land it first.
