# Security Policy

## Supported Versions

uniterm follows a rolling-release model. Only the latest published release and the `main` branch receive security fixes.

| Version / Branch | Supported |
|------------------|-----------|
| Latest release    | Yes       |
| `main`           | Yes       |
| Older releases   | No        |

## Reporting a Vulnerability

**Please do not file a public GitHub Issue for security vulnerabilities.**

Report privately via GitHub's [private vulnerability reporting](https://github.com/ys-ll/uniterm/security/advisories/new):

- **Scope**: any issue that could compromise confidentiality, integrity, or availability of a user's terminal sessions, stored credentials, AI provider keys, sync data, or the host system.
- **What to include**:
  - Description of the vulnerability and impact
  - Steps to reproduce or a proof-of-concept
  - Affected version / commit SHA
  - Your environment (OS, install method, protocol if relevant)

You should receive an acknowledgement within 3 business days. We aim to ship a fix or mitigation within 30 days of confirmation.

## Coordinated Disclosure

- We follow responsible-disclosure norms: please give us a reasonable window before publishing details.
- We are happy to credit reporters in the release notes and SECURITY advisory on request.
- If you are running uniterm in a sensitive environment, watch the [GitHub Security Advisories page](https://github.com/ys-ll/uniterm/security/advisories) for updates.

## Cryptography Notice

uniterm bundles the Go `crypto/*` standard library and upstream Go modules for SSH/TLS. It does **not** ship its own custom cryptographic primitives. Out-of-date Go runtimes can carry known CVEs in `crypto/*` — always build with a current Go release (see `go.mod`).

When users store connection credentials, AI provider keys, or sync secrets, uniterm uses the OS-provided secret store (Keychain on macOS, Credential Manager on Windows, libsecret on Linux) via `github.com/zalando/go-keyring`. Plaintext credentials are never written to disk.

## Out of Scope

The following are **not** considered uniterm vulnerabilities:

- Issues in upstream libraries (please report to the upstream maintainer).
- Self-XSS / social-engineering scenarios requiring the user to paste attacker-controlled content.
- Denial-of-service scenarios that require local code execution already on the user's machine.

## Acknowledgements

Thanks to the security researchers and users who report issues responsibly — your reports make uniterm safer for everyone.