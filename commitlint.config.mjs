// PR titles are linted as Conventional Commits (ADR-0009):
// squash-merge repo, titles become the commit history release-please reads.
export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    // Subjects legitimately start with acronyms here (SLO, API, CI, ADR);
    // the case heuristic misreads them as sentence-case. Type/scope casing
    // is still enforced by the other rules.
    'subject-case': [0],
  },
};
