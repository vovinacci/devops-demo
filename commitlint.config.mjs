// PR titles are linted as Conventional Commits (RFC-0001 D12):
// squash-merge repo, titles become the commit history release-please reads.
export default {
  extends: ['@commitlint/config-conventional'],
};
