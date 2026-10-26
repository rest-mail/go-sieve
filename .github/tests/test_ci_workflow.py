"""Offline fixtures for the local RFC verification workflow.

The proof uses the shared input contract with cross-package coverage.
These tests check what this repository passes it and that the
required Build & Test verdict fails closed. The workflow is read as text so the
job needs nothing beyond the runner's Python.

The local proof helper is loaded without running Go or its suite.
"""

import contextlib
import importlib.util
import io
import json
import os
import re
import subprocess
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
CALLER = (ROOT / ".github/workflows/ci.yml").read_text()
PIN = "7d291c08d310b9c62afc287e75a534a39150fafb"
PROOF = ROOT / ".github/scripts/rfc_verification.py"


def jobs():
    body = CALLER.split("\njobs:\n", 1)[1]
    return re.findall(r"^  ([A-Za-z0-9_-]+):\s*$", body, re.MULTILINE)


def job(name):
    body = CALLER.split("\njobs:\n", 1)[1]
    return re.split(r"^  [A-Za-z0-9_-]+:\s*$", body, flags=re.MULTILINE)[
        jobs().index(name) + 1
    ]


def quoted_input(name):
    match = re.search(
        r"^          "
        + re.escape(
            {"packages": "RFC_PACKAGES", "checklist-dirs": "RFC_CHECKLIST_DIRS"}[name]
        )
        + r": '([^']*)'\s*$",
        job("rfc"),
        re.MULTILINE,
    )
    return json.loads(match[1])


def folded_input(name):
    lines = job("rfc").splitlines()
    start = lines.index("          RFC_FUZZ_TARGETS: >-")
    block = []
    for line in lines[start + 1 :]:
        if not line.startswith("            "):
            break
        block.append(line.strip())
    return json.loads(" ".join(block))


def inputs():
    return {
        "RFC_PACKAGES": json.dumps(quoted_input("packages")),
        "RFC_CHECKLIST_DIRS": json.dumps(quoted_input("checklist-dirs")),
        "RFC_FUZZ_TARGETS": json.dumps(folded_input("fuzz-targets")),
    }


def repository_fuzz_targets():
    found = []
    for path in sorted(ROOT.rglob("*_test.go")):
        relative = path.relative_to(ROOT)
        if relative.parts[0].startswith(".") or "testdata" in relative.parts:
            continue
        package = (
            "." if relative.parent == Path(".") else "./" + relative.parent.as_posix()
        )
        for target in re.findall(
            r"^func (Fuzz\w*)\(\w+ \*testing\.F\)", path.read_text(), re.MULTILINE
        ):
            found.append({"package": package, "target": target})
    return found


class CallerFixtures(unittest.TestCase):
    def test_readme_coverage_badge_names_the_enforced_minimum(self):
        self.assertIn(
            "[![Coverage gate: >=95%]"
            "(https://img.shields.io/badge/coverage%20gate-%3E%3D95%25-brightgreen)]"
            "(COVERAGE.md)",
            (ROOT / "README.md").read_text(),
        )

    def test_local_proof_preserves_shared_contract(self):
        self.assertIn(PIN, PROOF.read_text())
        self.assertIn("    name: Verify RFC / RFC proof\n", job("rfc"))
        self.assertIn("name: Upload fuzz crashers", job("rfc"))
        self.assertIn("run: python3 -B .github/scripts/rfc_verification.py", job("rfc"))
        self.assertIn('go-version: "1.25.10"', job("rfc"))
        self.assertIn("version: v2.11.4", job("rfc"))
        self.assertIn('"-coverpkg=./..."', PROOF.read_text())
        self.assertNotIn("secrets:", CALLER)
        self.assertEqual(
            re.findall(r"^permissions:\n((?:  .*\n)+)", CALLER, re.MULTILINE),
            ["  contents: read\n"],
        )
        self.assertEqual(CALLER.count("permissions:"), 1)

    def test_events_are_unchanged(self):
        self.assertIn(
            "on:\n  push:\n    branches: [main]\n  pull_request:\n    branches: [main]\n",
            CALLER,
        )

    def test_packages_and_checklist_dirs(self):
        self.assertEqual(quoted_input("packages"), ["./..."])
        self.assertEqual(
            quoted_input("checklist-dirs"),
            ["./pkg/sieve", "./pkg/sieve/internal/grammar", "./pkg/sieve/rfctest"],
        )

    def test_fuzz_targets_are_exactly_the_repository_targets(self):
        targets = folded_input("fuzz-targets")
        self.assertEqual(len(targets), 7)

        def key(item):
            return item["package"], item["target"]

        self.assertEqual(
            sorted(targets, key=key), sorted(repository_fuzz_targets(), key=key)
        )

    def test_required_context_needs_every_other_job(self):
        self.assertEqual(jobs(), ["rfc", "fixtures", "build"])
        build = job("build")
        self.assertIn("    name: Build & Test\n", build)
        self.assertIn("    needs: [rfc, fixtures]\n", build)
        self.assertIn("    if: ${{ always() }}\n", build)
        self.assertIn("RFC_RESULT: ${{ needs.rfc.result }}", build)
        self.assertIn("FIXTURE_RESULT: ${{ needs.fixtures.result }}", build)
        self.assertNotIn("continue-on-error", CALLER)

    def test_required_verdict_fails_closed(self):
        command = next(
            line.strip().removeprefix("run: ")
            for line in job("build").splitlines()
            if line.strip().startswith("run: test ")
        )
        outcomes = ["success", "failure", "cancelled", "skipped", "", None]
        for rfc in outcomes:
            for fixtures in outcomes:
                env = {
                    k: v
                    for k, v in os.environ.items()
                    if k not in ("RFC_RESULT", "FIXTURE_RESULT")
                }
                env.update(
                    {
                        k: v
                        for k, v in [("RFC_RESULT", rfc), ("FIXTURE_RESULT", fixtures)]
                        if v is not None
                    }
                )
                result = subprocess.run(["sh", "-c", command], env=env, check=False)
                with self.subTest(rfc=rfc, fixtures=fixtures):
                    self.assertEqual(
                        result.returncode == 0, rfc == fixtures == "success"
                    )


class ProofContractFixtures(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location("rfc_proof_fixture", PROOF)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        cls.proof = vars(module)

    def test_caller_passes_only_declared_inputs(self):
        self.assertEqual(
            set(inputs()), {"RFC_PACKAGES", "RFC_CHECKLIST_DIRS", "RFC_FUZZ_TARGETS"}
        )
        self.proof["configuration"](inputs())

    def test_readme_coverage_badge_matches_the_producer_gate(self):
        # Exercise the pinned workflow's own helper, so a changed gate cannot
        # leave the README's minimum stale. No coverage or race suite runs.
        with contextlib.redirect_stdout(io.StringIO()):
            self.proof["coverage_total"]("total: (statements) 95.0%\n")
        with self.assertRaises(ValueError):
            self.proof["coverage_total"]("total: (statements) 94.99%\n")

    def test_producer_accepts_inputs_and_runs_every_proof(self):
        calls = []

        def go(*args, **kwargs):
            calls.append(args)
            if args[:2] == ("test", "-list=^Fuzz"):
                return (
                    "\n".join(item["target"] for item in repository_fuzz_targets())
                    + "\n"
                )
            if args[:2] == ("tool", "cover"):
                return "total: (statements) 100.0%\n"

        with patch.dict(self.proof, go=go), contextlib.redirect_stdout(io.StringIO()):
            self.proof["verify"](inputs())
        targets = folded_input("fuzz-targets")
        self.assertEqual(
            calls,
            [
                *[("test", "-list=^Fuzz", item["package"]) for item in targets],
                (
                    "test",
                    "-race",
                    "-count=1",
                    "-covermode=atomic",
                    "-coverpkg=./...",
                    "-coverprofile=coverage.out",
                    "./...",
                ),
                ("tool", "cover", "-func=coverage.out"),
                (
                    "run",
                    "./internal/checklist",
                    "-file",
                    "CHECKLIST.md",
                    *quoted_input("checklist-dirs"),
                ),
                *[
                    (
                        "test",
                        "-run=^$",
                        f"-fuzz=^{item['target']}$",
                        "-fuzztime=500000x",
                        item["package"],
                    )
                    for item in targets
                ],
            ],
        )


if __name__ == "__main__":
    unittest.main()
