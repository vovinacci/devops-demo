#!/usr/bin/env node
// Parity gate, JS side (RFC-0001 D5, ADR-0003, Hard rule 8): recomputes
// the golden grid with loadprofile/shape.js and compares every row
// against the Go-generated goldens (Go is canonical -- see
// loadprofile/README.md). A relative tolerance is used, not exact
// equality: Math.sin/exp differ by a few ULP between V8 and Go's math
// package on the same input, and that noise must not fail the gate.
//
// Usage: node loadprofile/parity/compare.mjs
// Exit code: 0 if every row of both goldens matches within tolerance,
// 1 otherwise (first mismatches printed).

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

import { rate } from "../shape.js";

const RELATIVE_TOLERANCE = 1e-9;
const MAX_MISMATCHES_PRINTED = 20;
const GOLDEN_REF_UNIX = 1767571200; // 2026-01-05T00:00:00Z, see loadprofile/README.md

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..", "..");

function loadProfile() {
  const data = readFileSync(path.join(repoRoot, "loadprofile", "profile.json"), "utf8");
  return JSON.parse(data);
}

function loadGolden(file) {
  const text = readFileSync(path.join(repoRoot, "loadprofile", "parity", file), "utf8");
  const lines = text.trim().split("\n");
  const [header, ...rows] = lines;
  if (header !== "unix_ts,rate") {
    throw new Error(`${file}: unexpected header ${header}`);
  }
  return rows.map((line) => {
    const [ts, r] = line.split(",");
    return { unixSeconds: Number(ts), rate: Number(r) };
  });
}

function withinTolerance(a, b) {
  if (a === b) return true;
  const denom = Math.max(Math.abs(a), Math.abs(b), 1e-12);
  return Math.abs(a - b) / denom <= RELATIVE_TOLERANCE;
}

function checkGolden(profile, file, scale) {
  const golden = loadGolden(file);
  const mismatches = [];
  for (const { unixSeconds, rate: wantRate } of golden) {
    const gotRate = rate(unixSeconds, profile, { scale, refUnixSeconds: GOLDEN_REF_UNIX });
    if (!withinTolerance(gotRate, wantRate)) {
      mismatches.push({ unixSeconds, wantRate, gotRate });
    }
  }
  return { file, rowCount: golden.length, mismatches };
}

function main() {
  const profile = loadProfile();
  const results = [
    checkGolden(profile, "golden-scale1.csv", 1),
    checkGolden(profile, "golden-scale24.csv", 24),
  ];

  let ok = true;
  let totalRows = 0;
  for (const { file, rowCount, mismatches } of results) {
    totalRows += rowCount;
    if (mismatches.length === 0) {
      console.log(`OK: ${file} -- ${rowCount} rows matched within ${RELATIVE_TOLERANCE} relative tolerance`);
      continue;
    }
    ok = false;
    console.error(`MISMATCH: ${file} -- ${mismatches.length}/${rowCount} rows outside tolerance`);
    for (const m of mismatches.slice(0, MAX_MISMATCHES_PRINTED)) {
      console.error(`  unix_ts=${m.unixSeconds} golden=${m.wantRate} js=${m.gotRate}`);
    }
    if (mismatches.length > MAX_MISMATCHES_PRINTED) {
      console.error(`  ... and ${mismatches.length - MAX_MISMATCHES_PRINTED} more`);
    }
  }

  if (!ok) {
    process.exit(1);
  }
  console.log(`parity OK: ${totalRows} rows compared across ${results.length} golden files`);
}

main();
