# Security policy

This is an educational project; it ships no production service and offers
no support SLA. Still, security reports are welcome -- they are usually
teachable moments.

## Reporting a vulnerability

- Preferred: GitHub private vulnerability reporting ("Report a
  vulnerability" under the Security tab), so the report stays private
  until fixed.
- Do not open a public issue for anything exploitable.

## Scope notes

- The compose stack is designed for local use: default credentials
  (Grafana admin/admin, Postgres app/app) are intentional and documented.
  Reports about them are out of scope unless they leak beyond localhost.
- Dependency and image CVEs are handled automatically (Renovate, Trivy in
  CI, weekly rescans); reports adding signal beyond those gates are
  appreciated.
