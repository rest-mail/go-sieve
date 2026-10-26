"""RFC verification, adapted from go-spf at 7d291c08d310b9c62afc287e75a534a39150fafb.

Cross-package coverage and execution-bounded fuzzing preserve proofs after the
package split. The 95% gate and input validation are unchanged.
"""

import json
import os
import re
import subprocess
from decimal import Decimal


def local_package(value, recursive=False):
    if not isinstance(value, str):
        raise TypeError("package must be a string")
    if value == ".":
        return value
    if recursive and value == "./...":
        return value
    path = value[2:] if value.startswith("./") else ""
    if recursive and path.endswith("/..."):
        path = path[:-4]
    if not path or any(
        part in (".", "..") or not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]*", part)
        for part in path.split("/")
    ):
        raise ValueError("expected a local package, without flags or traversal")
    return value


def nonempty_list(raw):
    value = json.loads(raw)
    if not isinstance(value, list) or not value:
        raise ValueError("expected a nonempty JSON array")
    return value


def configuration(env):
    packages = [
        local_package(p, recursive=True) for p in nonempty_list(env["RFC_PACKAGES"])
    ]
    dirs = [local_package(p) for p in nonempty_list(env["RFC_CHECKLIST_DIRS"])]
    targets = nonempty_list(env["RFC_FUZZ_TARGETS"])
    seen = set()
    for item in targets:
        if not isinstance(item, dict) or set(item) != {"package", "target"}:
            raise ValueError("fuzz target needs exactly package and target")
        local_package(item["package"])
        target = item["target"]
        if not isinstance(target, str) or not re.fullmatch(
            r"Fuzz[A-Za-z0-9_]+", target
        ):
            raise ValueError("expected an exact Go fuzz function name")
        key = (item["package"], target)
        if key in seen:
            raise ValueError("duplicate fuzz target")
        seen.add(key)
    return packages, dirs, targets


def go(*args, capture=False):
    return subprocess.run(
        ["go", *args],
        check=True,
        text=True,
        stdout=subprocess.PIPE if capture else None,
    ).stdout


def coverage_total(report):
    totals = [line for line in report.splitlines() if line.startswith("total:")]
    if len(totals) != 1:
        raise ValueError("expected exactly one coverage total")
    match = re.fullmatch(
        r"total:\s+\(statements\)\s+([0-9]+(?:\.[0-9]+)?)%\s*", totals[0]
    )
    if not match:
        raise ValueError("malformed coverage total")
    total = Decimal(match[1])
    if not Decimal(95) <= total <= Decimal(100):
        raise ValueError(f"statement coverage {total}% is outside 95..100%")
    print(f"Statement coverage: {total}%", flush=True)


def verify(env):
    packages, dirs, targets = configuration(env)
    # Refuse misspelled or absent targets before doing the expensive proof.
    for item in targets:
        listed = go("test", "-list=^Fuzz", item["package"], capture=True).splitlines()
        if item["target"] not in listed:
            raise ValueError(f"fuzz target not found: {item}")
    go(
        "test",
        "-race",
        "-count=1",
        "-covermode=atomic",
        "-coverpkg=./...",
        "-coverprofile=coverage.out",
        *packages,
    )
    coverage_total(go("tool", "cover", "-func=coverage.out", capture=True))
    go("run", "./internal/checklist", "-file", "CHECKLIST.md", *dirs)
    for item in targets:
        go(
            "test",
            "-run=^$",
            f"-fuzz=^{item['target']}$",
            "-fuzztime=500000x",
            item["package"],
        )


if __name__ == "__main__":
    verify(os.environ)
