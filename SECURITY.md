# Security

ArtifactKit processes untrusted local artifacts, so parser safety is part of its public contract.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting for this repository. Do not open a public issue for a suspected vulnerability.

Include the affected format, the smallest artifact that reproduces the issue, configured limits, ArtifactKit version, and expected versus observed behavior. Remove confidential document content where possible.

## Parser guarantees

- ArtifactKit never executes document macros, scripts, formulas, or embedded programs.
- Input, text, node count, nesting, archive entry, expansion, and compression-ratio limits are enabled by default.
- Archive paths are normalized and must not escape their logical root.
- Parsing stays local unless an embedding application explicitly moves returned data elsewhere.

No parser should weaken these guarantees. New parsers must include malformed-input and limit tests.

