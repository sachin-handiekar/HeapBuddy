# Security Policy

## Reporting a Vulnerability

Please **do not** report security vulnerabilities through public GitHub issues.

Instead, report them privately via GitHub's
[private vulnerability reporting](https://github.com/sachin-handiekar/HeapBuddy/security/advisories/new).
This creates a confidential channel with the maintainers.

When reporting, please include:

- A description of the vulnerability and its impact
- Steps to reproduce (a minimal proof of concept if possible)
- The HeapBuddy version (`heapbuddy --version`) and your OS/Go version

You can expect an initial acknowledgement within **5 business days** and a more detailed
response indicating the next steps within **10 business days**.

## A Note on Heap Dumps

HeapBuddy reads JVM heap dumps (`.hprof` files). **Heap dumps frequently contain sensitive
data** — passwords, tokens, personal data, and other secrets that were resident in memory.

- Never attach a real production `.hprof` to a public issue or pull request.
- When sharing a reproduction, use a synthetic dump (see `sample-hprof/`) or scrub
  sensitive values first.
- HeapBuddy processes dumps locally and does not transmit data anywhere; treat the
  generated HTML/JSON reports with the same care as the original dump.

## Supported Versions

This project is pre-1.0. Security fixes are applied to the latest released version and
`main`. Please upgrade to the latest release before reporting an issue.
