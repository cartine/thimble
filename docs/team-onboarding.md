# Adding a new operator or deploy host

Thimble access is granted per **namespace** (one application +
environment pair). Granting access means adding a new **recipient** —
an age public key — to the namespace's manifest and re-encrypting the
**bundle** to it. This page walks through the three routine flows:
onboarding a human operator, onboarding a deploy host, and offboarding.

Throughout, `web-api production` is the example namespace. Substitute
your own application and environment.

## Onboarding a new operator

### 1. The new operator generates an identity

On their own machine — the private identity never leaves it:

```sh
age-keygen -o ~/.config/thimble/identity.txt
chmod 0600 ~/.config/thimble/identity.txt
export THIMBLE_AGE_IDENTITY=~/.config/thimble/identity.txt
```

Add the `export` line to their shell profile so every `thimble`
invocation finds the identity. Thimble refuses group- or
world-readable identity files, so the `chmod 0600` is not optional.

### 2. The operator shares their PUBLIC recipient

```sh
age-keygen -y ~/.config/thimble/identity.txt
# → age1qwj0...
```

The `age1...` string is the **recipient** — the public half. It is
safe to share, but share it **out of band** (a call, a signed message,
an in-person read-back), not just a chat paste. The maintainer must
verify the string really belongs to the new operator before granting
access; a wrong recipient string grants plaintext access to whoever
holds the matching private key.

Never send the identity file itself. If a private identity ever moves
between machines or people, treat it as compromised.

### 3. The maintainer adds the recipient

What happens next depends on whether the namespace has a quorum
policy (`secrets/recipients.signed.toml` present).

#### No policy file (single-maintainer namespaces)

One command, done:

```sh
thimble recipient add web-api production age1qwj0...
```

The bundle is re-encrypted so the new recipient can decrypt all
current values.

(For a brand-new namespace with fewer than 2 recipients, use
`thimble recipient add --bootstrap` — the chicken-and-egg escape
that skips the gate while there is no quorum to consult.)

#### With a quorum policy (`recipients.signed.toml` present)

Every add must be approved by M of the operators listed in the policy
file. The maintainer runs the **same** `recipient add` command
**twice** — the first run prepares challenges, the second commits once
enough signatures exist. In between, each approving operator runs
`recipient sign-add` once.

Worked example with a 2-of-3 policy (alice, bob, carol), where alice
is the maintainer adding dana:

```sh
# alice — run 1: prepare. Writes .pending-recipient-adds/ with one
# encrypted challenge per policy operator.
thimble recipient add web-api production age1dana...

# alice shares the pending directory (short-lived branch, rsync, ...).

# bob — sign. Decrypting the challenge with his private identity is
# the proof of key possession; the signature lands in the same dir.
THIMBLE_AGE_IDENTITY=~/.config/thimble/identity.txt \
  thimble recipient sign-add web-api production age1dana...

# carol — same command on her machine. (2 of 3 reached.)
thimble recipient sign-add web-api production age1dana...

# alice — run 2: commit. Verifies the signatures, mutates the
# manifest, re-encrypts the bundle, cleans up the pending directory.
thimble recipient add web-api production age1dana...
```

If the second run reports the quorum is short, it names exactly which
operators have not signed yet — collect the missing signatures and
re-run; no re-prepare needed. If another mutation re-encrypted the
bundle in the meantime, the pending signatures are invalidated and
the maintainer starts again from the prepare step.

### 4. Verify

```sh
thimble recipient list web-api production
thimble doctor
```

`recipient list` should show the new recipient with its thumbprint.
The new operator should then confirm they can actually decrypt:

```sh
thimble list web-api production        # key names — works without identity
thimble verify web-api production      # bundle SHA + recipient list
thimble exec web-api production -- env # proves decryption end to end
```

## Onboarding a new deploy host

A deploy host is just another recipient, plus (optionally) a **peer**
entry if it should participate in rsync sync.

### 1. Host identity and recipient grant

On the host:

```sh
age-keygen -o /etc/thimble/identity.txt
chmod 0600 /etc/thimble/identity.txt
age-keygen -y /etc/thimble/identity.txt   # → the host's recipient
```

Collect the `age1...` recipient out of band, then grant it exactly as
for an operator (quorum flow included, if a policy is present):

```sh
thimble recipient add web-api production age1host...
```

### 2. Wire up peer sync (optional)

If the host is a leader that should receive the on-mutate rsync
broadcast, register it on the existing leaders:

```sh
thimble peer add deploy-1 deploy@deploy-1.internal
thimble peer list
```

On the new host itself, bootstrap the store from an existing peer:

```sh
thimble peer join deploy@leader-1.internal
# or, if the host has a stale/partial secrets/ dir to discard:
thimble peer join --replace deploy@leader-1.internal
```

### 3. Verify connectivity and health

```sh
thimble peer ping deploy-1     # active ssh+rsync probe
thimble peer status            # passive view of .peer-state.json
thimble doctor                 # includes the peers health check
```

`thimble peer ping --quiet` is the cron-friendly form for ongoing
monitoring.

## Offboarding

Remove the departing recipient and rotate in one atomic step:

```sh
thimble recipient remove --rotate web-api production age1old...
```

Then push the re-encrypted store to your store host or let the peer
broadcast handle it.

Know exactly what `--rotate` does and does not cover:

- **Covered**: every value whose origin is `provision` — the
  high-entropy random tokens created via
  `thimble provision | thimble set --origin=provision ...` — is
  regenerated automatically.
- **Not covered**: operator-supplied values (database URLs, third-party
  API keys). These are listed as "manual rotate needed" — rotate them
  at their source, then re-run `thimble set` for each. Use
  `--rotate-randoms-only` to suppress those hints in scripts.

Removal without `--rotate` only prevents the former recipient from
decrypting **future** bundles. It cannot un-share anything they
already decrypted, so treat every value they could read as exposed
until rotated.

Finish with:

```sh
thimble recipient list web-api production
thimble audit --limit 20 web-api production
```

The audit log entry ties the removal (and, for quorum adds, the signer
thumbprints) to the operation — review it the way you would review a
recipient-only diff.
