#!/usr/bin/env python3
"""
Generator: read draft markdown in tasks/knots/, create real knots via `kno new`,
then build the execution_plan with waves and steps.

Usage: python3 tasks/build_plan.py [--dry-run]
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
KNOTS_DIR = ROOT / "tasks" / "knots"
ID_MAP_PATH = ROOT / "tasks" / "id-map.json"


def section(text: str, name: str) -> str | None:
    pat = re.compile(rf"^## {re.escape(name)}\n(.*?)(?=^## |\Z)", re.M | re.S)
    m = pat.search(text)
    return m.group(1).strip() if m else None


def first_field(text: str, label: str) -> str | None:
    pat = re.compile(rf"^- {re.escape(label)}:\s*(.+)$", re.M)
    m = pat.search(text)
    return m.group(1).strip() if m else None


def parse_knot_md(path: Path) -> dict:
    raw = path.read_text()
    title_match = re.match(r"^#\s+(K-\d+)\s+—\s+(.+)$", raw.split("\n", 1)[0])
    if not title_match:
        raise ValueError(f"could not parse title in {path}")
    knot_id, title = title_match.group(1), title_match.group(2).strip()

    wave_step = first_field(raw, "Wave / Step") or ""
    effort = first_field(raw, "Effort") or ""
    risk = first_field(raw, "Risk") or ""
    deps = first_field(raw, "Deps") or "—"
    files = first_field(raw, "Files") or ""

    goal = section(raw, "Goal") or ""
    acceptance = section(raw, "Acceptance") or ""
    notes = section(raw, "Notes") or ""

    # Description: Goal + Verification + Constraints (per knots-create skill).
    desc_parts = [f"Goal:\n{goal}"]

    # Verification: derive from acceptance bullets, keep them as numbered.
    verif_lines = []
    n = 0
    for line in acceptance.splitlines():
        stripped = line.lstrip()
        if stripped.startswith("- "):
            n += 1
            verif_lines.append(f"{n}. {stripped[2:]}")
        elif stripped and verif_lines:
            # continuation line
            verif_lines[-1] += " " + stripped
    if verif_lines:
        desc_parts.append("Verification:\n" + "\n".join(verif_lines))

    constraints = []
    constraints.append(f"Knot ref: {knot_id}; wave/step {wave_step}; effort {effort}; risk {risk}")
    if files:
        constraints.append(f"Files: {files}")
    if deps and deps != "—":
        constraints.append(f"Depends on: {deps}")
    if notes:
        constraints.append(f"Notes: {notes}")
    desc_parts.append("Constraints:\n" + "\n".join(f"- {c}" for c in constraints))

    description = "\n\n".join(desc_parts)

    # Acceptance criteria (numbered) for --acceptance flag
    crit_lines = []
    n = 0
    for line in acceptance.splitlines():
        stripped = line.lstrip()
        if stripped.startswith("- "):
            n += 1
            crit_lines.append(f"{n}. {stripped[2:]}")
        elif stripped and crit_lines:
            crit_lines[-1] += " " + stripped
    accept_arg = "\n".join(crit_lines)

    return {
        "ref": knot_id,
        "title": title,
        "wave_step": wave_step,
        "description": description,
        "acceptance": accept_arg,
        "deps": deps,
        "tags": [f"wave-{wave_step.split('.')[0]}", f"risk-{risk}"],
    }


def run(cmd: list[str], dry: bool = False) -> str:
    if dry:
        print("DRY:", " ".join(cmd[:3]) + " ...")
        return "dry-skip"
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"FAIL: {' '.join(cmd[:3])}", file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        sys.exit(2)
    return result.stdout.strip()


def parse_created(out: str) -> str:
    # Output: "created <short-id> [STATE] <title>"
    m = re.match(r"created\s+(\S+)\s+\[", out)
    if not m:
        raise ValueError(f"unparseable kno output: {out!r}")
    return m.group(1)


# Wave/step structure with knot refs.
PLAN = [
    {"wave": 1, "name": "Foundation Alignment",
     "objective": "Source-shape and vocabulary baseline before any other work begins",
     "steps": [
         ["K-01"],
         ["K-02"],
         ["K-03"],
         ["K-04"],
     ]},
    {"wave": 2, "name": "Legal & Trust Scaffolding",
     "objective": "Repo passes basic procurement smell-test; trust enablers in place",
     "steps": [
         ["K-05", "K-06", "K-07", "K-08", "K-09", "K-11"],
     ]},
    {"wave": 3, "name": "Code Structure",
     "objective": "main.go split into auditable packages with clear trust boundaries",
     "steps": [
         ["K-12"],
     ]},
    {"wave": 4, "name": "CI / Quality Gates",
     "objective": "PRs run vet, test, lint, vuln, and real-age integration",
     "steps": [
         ["K-13", "K-14", "K-15", "K-16", "K-17"],
     ]},
    {"wave": 5, "name": "Runtime Security Hardening",
     "objective": "Close the runtime threat-model items: age trust, identity hygiene, locks, audit, ops",
     "steps": [
         ["K-18", "K-19", "K-20", "K-21", "K-23", "K-24", "K-25", "K-26", "K-28"],
         ["K-22", "K-27", "K-29"],
     ]},
    {"wave": 6, "name": "Web UI Hardening",
     "objective": "Close web-surface items: cookies, host allowlist, cache, rotation, plaintext policy, scope",
     "steps": [
         ["K-30", "K-31", "K-32", "K-34", "K-35"],
         ["K-33"],
     ]},
    {"wave": 7, "name": "Recipient Governance",
     "objective": "Recipient adds gated by quorum; rotate flow on removal",
     "steps": [
         ["K-36"],
         ["K-37"],
     ]},
    {"wave": 8, "name": "Release & Install Hardening",
     "objective": "Signed, attested, multi-channel installs",
     "steps": [
         ["K-38", "K-40"],
         ["K-39", "K-41"],
         ["K-42", "K-43", "K-44", "K-45"],
     ]},
    {"wave": 9, "name": "DevX Polish",
     "objective": "README/Make/automation tell the story",
     "steps": [
         ["K-10", "K-46", "K-47"],
         ["K-48"],
     ]},
]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--repo", default=str(ROOT))
    ap.add_argument("--reuse-map", action="store_true",
                    help="Skip knot creation; reuse tasks/id-map.json (for re-running plan wiring).")
    args = ap.parse_args()

    md_files = sorted(KNOTS_DIR.glob("K-*.md"))
    if len(md_files) != 48:
        print(f"warning: expected 48 knot files, found {len(md_files)}", file=sys.stderr)

    knots = [parse_knot_md(p) for p in md_files]
    by_ref = {k["ref"]: k for k in knots}

    id_map: dict[str, str] = {}
    if args.reuse_map and ID_MAP_PATH.exists():
        id_map = json.loads(ID_MAP_PATH.read_text())
        print(f"reusing id-map with {len(id_map)} entries", file=sys.stderr)

    # 1. Create the parent execution_plan knot first.
    plan_id_key = "_PLAN"
    if plan_id_key not in id_map:
        out = run([
            "knots", "-C", args.repo, "new", "Thimble hardening plan",
            "--type", "execution_plan",
            "--objective", "Take Thimble from v0.0 hobby to a small-team-trustable secrets manager: source shape, vocabulary, legal/trust scaffolding, code restructure, CI gates, runtime hardening, web UI hardening, recipient governance, release & install hardening, DevX polish.",
            "-d", "Parent execution_plan for the 48-knot Thimble hardening rollout. See tasks/knot-plan.md for the human-readable map and tasks/knots/K-NN-*.md for individual knot drafts.",
        ], dry=args.dry_run)
        plan_id = parse_created(out) if not args.dry_run else "dry-plan"
        id_map[plan_id_key] = plan_id
        print(f"plan: {plan_id}")
    else:
        plan_id = id_map[plan_id_key]
        print(f"plan: {plan_id} (reused)")

    # 2. Create each work knot, recording short id.
    for k in knots:
        ref = k["ref"]
        if ref in id_map:
            print(f"  {ref}: {id_map[ref]} (reused)")
            continue
        cmd = [
            "knots", "-C", args.repo, "new", k["title"],
            "-d", k["description"],
        ]
        if k["acceptance"]:
            cmd += ["--acceptance", k["acceptance"]]
        for tag in k["tags"]:
            cmd += ["-t", tag]
        out = run(cmd, dry=args.dry_run)
        short = parse_created(out) if not args.dry_run else f"dry-{ref}"
        id_map[ref] = short
        print(f"  {ref}: {short}  {k['title']}")
        # Persist incrementally so a crash mid-batch is recoverable.
        if not args.dry_run:
            ID_MAP_PATH.write_text(json.dumps(id_map, indent=2))

    # 3. Add waves.
    for w in PLAN:
        run([
            "knots", "-C", args.repo, "plan", "wave", "add", plan_id,
            "--name", f"Wave {w['wave']}: {w['name']}",
            "--objective", w["objective"],
        ], dry=args.dry_run)

    # 4. Add steps within each wave.
    for w in PLAN:
        for step_refs in w["steps"]:
            knot_ids = ",".join(id_map[r] for r in step_refs)
            run([
                "knots", "-C", args.repo, "plan", "step", "add", plan_id,
                "--wave", str(w["wave"]),
                "--knot-ids", knot_ids,
            ], dry=args.dry_run)

    if not args.dry_run:
        ID_MAP_PATH.write_text(json.dumps(id_map, indent=2))
    print(f"\ndone. plan={plan_id}, {len(knots)} child knots wired.")


if __name__ == "__main__":
    main()
