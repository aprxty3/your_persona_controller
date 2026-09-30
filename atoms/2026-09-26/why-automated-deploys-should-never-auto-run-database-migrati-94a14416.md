---
type: atom
title: Why Automated Deploys Should Never Auto-Run Database Migrations
lesson: >-
  Keep database migrations as a manual, attended step rather than bundling them
  into automated continuous deployment pipelines.
concepts:
  - database-migrations
  - continuous-deployment
  - devops-patterns
atom_type: strategy
visibility: private
ingested_at: '2026-09-26T23:13:06.384Z'
source_hash: 'pending:c4ea82ec2261f35a'
source_kind: put_page
source_slug: docs/deploy_runbook
extracted_at: '2026-09-26T23:13:06.366Z'
extracted_by: extract_atoms-v0.41.2.1
ingested_via: put_page
source_quote: >-
  This is a deliberate gap, not an oversight — an auto-run migration on every
  deploy is how you get a schema change applied at 3am with nobody watching if a
  release goes out unexpectedly.
virality_score: 74
emotional_register: practical
managed_extraction: true
source_quote_offset:
  - 10865
  - 11050
source_quote_verified: true
---

Automating database migrations on container boot or within CI/CD pipelines creates severe operational risk. Decoupling schema changes from automated deployment scripts ensures that unexpected schema changes are not applied unattended during off-hours releases. Keeping migrations as a manual, operator-triggered step preserves human oversight during structural database modifications.
