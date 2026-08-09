# Incident Review

ArtifactKit parsed the local report without sending it to a remote service.

## Findings

The payment retry worker duplicated three jobs after a lease expired.

## Action items

Add an idempotency key and preserve the source location for every finding.

