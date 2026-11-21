#!/usr/bin/env python3
"""Rank Go compile errors in a transpiled package: missing swiftrt functions, then other errors by shape."""
import collections, re, subprocess, sys

pkg = sys.argv[1]
out = subprocess.run(["go", "run", "./cmd/gocheck", "./out/" + pkg], capture_output=True, text=True).stdout
lines = [l for l in out.splitlines() if re.match(r"^\S+\.go:\d+:\d+: ", l) and f"/out/{pkg}/" in l]
missing_rt = collections.Counter()
missing_dep = collections.Counter()
shapes = collections.Counter()
examples = {}
for l in lines:
    msg = re.sub(r"^\S+\.go:\d+:\d+: ", "", l)
    m = re.search(r"undefined: swiftrt\.(\w+)", msg)
    if m:
        missing_rt[m.group(1)] += 1
        continue
    m = re.search(r"undefined: (\w+)\.(\w+)", msg)
    if m:
        missing_dep[m.group(1) + "." + m.group(2)] += 1
        continue
    shape = re.sub(r'"[^"]*"', '"…"', msg)
    shape = re.sub(r"\b[A-Z]\w*\b", "T", shape)
    shape = re.sub(r"\b\d+\b", "N", shape)
    shape = re.sub(r"\(value of (struct |interface |)type [^)]*\)", "(value of type T)", shape)
    shape = re.sub(r"\btype [\w.*\[\]]+", "type T", shape)
    shape = re.sub(r"[a-z_]\w*\.[a-z_]\w*", "x.y", shape)
    shapes[shape] += 1
    examples.setdefault(shape, l)
total = len(lines)
print(f"{pkg}: {total} errors, {sum(missing_rt.values())} missing-swiftrt ({len(missing_rt)} distinct), {sum(missing_dep.values())} missing-dep")
print("\n== missing swiftrt (top 60)")
for k, v in missing_rt.most_common(60):
    print(f"{v:6d} {k}")
print("\n== missing dependency symbols (top 20)")
for k, v in missing_dep.most_common(20):
    print(f"{v:6d} {k}")
print("\n== error shapes (top 40)")
for k, v in shapes.most_common(40):
    print(f"{v:6d} {k}\n         {examples[k][:220]}")
