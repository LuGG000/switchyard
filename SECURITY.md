# Security policy

Please report vulnerabilities privately via GitHub's "Report a vulnerability"
feature on this repository instead of opening a public issue.

switchyard must never read, copy, log or transmit credentials or tokens.
Reports of any violation of this rule are treated as security issues. CI
enforces part of it: it fails when non-test Go code refers to `credentials.json`,
and when a credential-like file is tracked.

## Supported versions

Only the latest release gets security fixes. Update with `switchyard update --install`.
The project is in early development; there are no long-term support branches.

## What to expect

switchyard is maintained by one person. A report is acknowledged within about a week,
and a confirmed problem is fixed in a new release as soon as is practical; the report
is credited if you wish. Please give a reasonable time to fix before you publish details.

Releases are signed: `checksums.txt` carries a cosign signature, see "Verify a download"
in the README.
