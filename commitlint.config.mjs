// PR titles are linted as Conventional Commits (ADR-0009):
// squash-merge repo, titles become the commit history release-please reads.
export default {
  extends: ['@commitlint/config-conventional'],
};
