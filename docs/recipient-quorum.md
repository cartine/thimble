# Recipient quorum protocol (K-36)

This document specifies the on-disk and runtime protocol for the
quorum-signed recipient list shipped in K-36. The goal is a
structural fix — not process — for the social-engineering attack
where a single compromised maintainer adds an unauthorized recipient
in a one-line manifest diff and gains plaintext access to every
future write.

## File layout

```
secrets/
  recipients.signed.toml          # quorum policy (optional, opt-in)
  thimble.json                    # plaintext manifest
  .pending-recipient-adds/        # in-flight quorum gate (transient)
    meta.json                     # canonical-message inputs
    <op-thumbprint>.challenge     # one per policy operator
    <op-thumbprint>.sig           # one per signing operator
  .thimble-audit.log              # append-only audit ledger (K-27)
  <app>/<env>.env.age             # ciphertext bundles
```

The `recipients.signed.toml` file is the activation switch. When
absent, recipient adds work exactly as before K-36 (single-operator
add, no gate). When present, every `recipient add` must be either
`--bootstrap` (valid only at <2 recipients) or accompanied by M
signatures from the listed operators.

## Policy file (`recipients.signed.toml`)

Hand-edited TOML subset, parsed by `internal/quorum/policy.go`:

```toml
[policy]
quorum_m = 2  # signatures required; quorum_n is implicit = len(operators)

[[operators]]
name = "alice"
recipient = "age1..."

[[operators]]
name = "bob"
recipient = "age1..."

[[operators]]
name = "recovery-offline"
recipient = "age1..."
```

The parser is intentionally narrow: only `[policy]` and
`[[operators]]` tables, only the keys shown above, no escapes inside
quoted strings, no nested tables. Anything outside this subset
fails loudly with line numbers — the policy file is small and
hand-edited, so loose parsing would be a footgun.

Validation enforces:

- `quorum_m >= 1`
- `quorum_m <= len(operators)`
- Operator names are unique and non-empty
- Operator recipients are unique and pass `store.ValidateRecipient`

## Canonical signed message

Every signature is over a single ASCII line:

```
thimble-recipient-add:<app>:<env>:<new-recipient>:<bundle-sha-at-prepare>:<nonce-hex>
```

The fields:

- `<app>` and `<env>`: scope the addition to one namespace.
- `<new-recipient>`: exact recipient string the maintainer wants to
  add. Replay-resistant by content: a signature over recipient X
  cannot be repurposed to add Y.
- `<bundle-sha-at-prepare>`: the bundle's `BundleSHA256` at prepare
  time. Once `recipient add` commits, the bundle is re-encrypted and
  this SHA changes, invalidating any unspent signatures.
- `<nonce-hex>`: 16 random bytes (32 hex chars) generated at prepare
  time and persisted in `meta.json`. Defense-in-depth against
  structural collisions; in practice the bundle SHA already gives
  per-prepare uniqueness.

Bytes are compared with `==` after a single trailing-newline trim.
No JSON, no whitespace tolerance, no base64.

## Three-phase protocol

The maintainer runs `thimble recipient add` twice with the same
arguments — the first run writes challenges, the second commits.
Operators run `thimble recipient sign-add` once each in between.

### Phase 1 — Prepare (maintainer)

```
$ thimble recipient add svc prod age1new...
```

Triggered when `recipients.signed.toml` is present and there is no
pending `meta.json`. The store layer:

1. Validates the new recipient and the policy.
2. Records `bundle_sha_at_prepare = current bundle's SHA-256`.
3. Generates a 16-byte random nonce.
4. For each operator in the policy:
   - Computes thumbprint = `sha256(operator.recipient)[:16] hex`.
   - Encrypts the canonical message *to the operator alone* (one
     `-r` recipient line, the operator's age public key).
   - Writes ciphertext to
     `.pending-recipient-adds/<thumbprint>.challenge`.
5. Writes `meta.json` containing all of the above plus the
   `verifier_recipient` (the maintainer's public recipient,
   parsed from `THIMBLE_AGE_IDENTITY`).

The maintainer commits the pending directory to a short-lived
branch or shares it out of band; operators consume the file
addressed to them.

### Phase 2 — Sign (each operator)

```
$ THIMBLE_AGE_IDENTITY=~/.config/thimble/identity.txt \
    thimble recipient sign-add svc prod age1new...
```

The CLI:

1. Reads the operator's public recipient from their identity file.
2. Looks up the operator's policy entry by recipient (refuses if
   the recipient is not in `recipients.signed.toml`).
3. Computes their thumbprint, locates
   `.pending-recipient-adds/<thumb>.challenge`.
4. **Decrypts the challenge with their private identity** — this is
   the cryptographic proof of key possession. age decryption fails
   if the operator does not hold the private key matching the
   challenge's recipient header.
5. Validates the decrypted plaintext against the canonical message
   reconstructed from `meta.json`.
6. Re-encrypts the same plaintext to the verifier's recipient and
   writes `.pending-recipient-adds/<thumb>.sig`.

Re-encryption to the verifier is what lets the maintainer later
decrypt the signature with their own identity and confirm the
canonical bytes match.

### Phase 3 — Commit (maintainer)

```
$ thimble recipient add svc prod age1new...
```

Same command, run a second time. The store layer sees `meta.json`
exists, switches to the verify path:

1. Re-reads the policy and meta.
2. Refuses if the bundle SHA has changed since prepare (catches a
   concurrent re-encrypt from another mutation).
3. Walks `.pending-recipient-adds/*.sig`. For each:
   - Looks up the policy operator by thumbprint (skips unknown).
   - Decrypts with the maintainer's identity (skips on failure).
   - Compares plaintext to the canonical message (skips on mismatch).
4. Counts distinct, valid operators. If `< quorum_m`, fails with a
   message naming the operators still missing.
5. On success, mutates the manifest, re-encrypts the bundle, and
   appends an audit entry with the signer thumbprints and
   `bootstrap=false`.
6. Removes the entire `.pending-recipient-adds/` directory.

## Forgery analysis

To forge an `<op>.sig` that passes verification, an attacker needs
either:

- The operator's private age key (so they can decrypt the challenge
  and produce a re-encryption whose plaintext matches the canonical
  message), or
- The maintainer's private age key (so they can decrypt their own
  forged ciphertext and edit it pre-commit — but the verifier only
  trusts the per-operator challenges they themselves generated, so
  this attack also requires the operator's key to fabricate a
  challenge ciphertext that decrypts cleanly).

In both cases, possession of an operator's private key is the
requirement — which is what "M of N must approve" means.

## Bootstrap

When the namespace has 0 or 1 recipients, `--bootstrap` is allowed
and skips the gate:

```
$ thimble recipient add --bootstrap svc prod age1bob...
```

This exists because the gate is meaningful only when ≥2 recipients
already share the bundle (otherwise the "M operators" set is empty
or trivially captured). The audit entry records `bootstrap=true` so
reviewers can grep for adds that bypassed the gate.

## Audit log

The `recipient_add` audit event for a quorum-gated add carries:

- `op = "recipient_add"`
- `subject = <new-recipient-thumbprint>`
- `signers = [<thumb1>, <thumb2>, ...]` (sorted by policy order)
- `bootstrap = false`

The signers list is the auditor's primary anchor: it ties a
plaintext-access grant to the operators whose signatures were
collected, without leaking any recipient string.

## Failure modes & operator UX

- **Missing meta.json on `sign-add`**: maintainer hasn't run prepare
  yet. Error tells the operator to ask the maintainer to run
  `thimble recipient add` first.
- **Recipient mismatch between meta and CLI args**: the maintainer
  prepared X but typed Y on commit. Error names both.
- **Non-listed operator runs sign-add**: refusal includes the
  operator's thumbprint so the maintainer can pinpoint who.
- **Stale signatures after a concurrent re-encrypt**: bundle SHA
  check fires; meta is invalidated; maintainer must re-prepare.
- **Quorum short**: error names every operator who hasn't signed yet
  (`bob or carol or recovery-offline`).

The pending directory is left intact on every failure path, so the
maintainer can collect the missing signatures and retry without
re-preparing — except when stale-SHA forces a re-prepare.
