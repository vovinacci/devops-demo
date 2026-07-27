# ADR-0019: make as the task runner, logic in scripts/

- Status: Accepted
- Date: 2026-07-27

## Context

Every operation in this repo is reached through one entry point: `make up`,
`make smoke`, `make test-backend`, `make lint-infra`. CI calls the same
targets, which is what makes "CI runs what you run" (engineering-principles
Section 6) checkable rather than aspirational.

That choice was never written down. It was inherited from the baseline
(RFC-0000) and had no ADR, so a reasonable reviewer asking "why not Taskfile?"
had nothing to read. Two things needed deciding: which runner, and how much
logic belongs inside it.

The second question had drifted. `make test-backend` was 32 lines of shell
inside a recipe -- backslash continuations, `$$` escaping, a `while` loop, an
exit code captured and re-raised around a cleanup step. `make clean` was 28.
Recipe shell is the worst place to write that: it is unlintable (shellcheck
does not see it), untestable in isolation, and written in a dialect that is
neither shell nor make but a quoting compromise between them.

## Decision

**Keep make**, and move any recipe with real control flow or meaningful length
into `scripts/`, invoked as a one-line target.

`test-backend`, `test-docker`, and `clean` moved to `scripts/*.sh`. The
Makefile keeps its role as the index: a target line, its `##` help text, and
a call. `COMPOSE` stays defined once in the Makefile and is passed to scripts
through the environment, so the compose invocation has a single definition.

The line is drawn at **control flow, not line count**. `smoke` and `smoke-full`
stay inline despite their length: they are flat sequences of compose commands
whose bulk is comments explaining the CI contract, and `.github/workflows/`
points at them as "runs exactly this". Extracting them would add a hop without
removing complexity. A branch, a loop, or a trap is the signal to extract.

Why make over the alternatives:

- It is already installed everywhere -- students, CI runners, and containers.
  For a teaching repo, a prerequisite that is not there yet is a prerequisite
  some fraction of the class will fight instead of learning from.
- Students meet make in the wild far more than any newer runner, so the
  syntax is transferable knowledge rather than repo-local trivia.
- The features that would justify switching -- typed variables, dependency
  graphs, cross-platform shell -- are not load-bearing here now that the
  logic lives in scripts. What remains in the Makefile is target names,
  ordering, and help text, which every runner does equally well.

## Alternatives

- **Taskfile (go-task): rejected, but genuinely close.** Better than make on
  the axes it targets: YAML instead of tab-sensitive recipes, real variables,
  `deps` without `.PHONY` bookkeeping, built-in `--list` instead of the awk
  incantation this Makefile uses for `make help`, and no `$$`-escaping tax.
  Rejected on two grounds, neither of them "make is better": it is another
  required install in a repo whose prerequisites are already a documented
  hurdle (`make doctor` exists because of that), and the migration would
  touch every workflow, every doc, and every exercise for no behavioural
  gain. If the Makefile grows back toward orchestration logic rather than
  staying an index, this decision is worth reopening -- that is the trigger
  to watch, and this ADR should be superseded rather than quietly ignored.
- **Shell scripts alone, no runner:** rejected -- discoverability collapses.
  `make help` listing 54 targets is a real onboarding surface; a directory of
  scripts is not.
- **npm scripts:** rejected -- would put a JavaScript toolchain on the
  critical path of a five-runtime repo where most services are not JS.

## Consequences

- Easier: extracted logic is linted by shellcheck and formatted by shfmt in
  the existing hooks, which recipe shell never was. The Makefile shrank from
  446 to 373 lines and reads as a table of contents.
- Easier: a script can be run directly when debugging, without make's
  variable expansion in the way.
- Harder: one more indirection -- reading `make test-backend` now means
  opening a second file. The `##` help text and a header comment in each
  script are the mitigation.
- Harder: scripts depend on `COMPOSE` being exported by the caller. Each
  fails loudly (`${COMPOSE:?...}`) rather than silently running a bare
  `docker compose` against the wrong project.
