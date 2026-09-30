---
type: atom
title: Closing Audit Gaps for Dashboard-Only Cloud Infrastructure
lesson: >-
  Mandate verification artifacts in pull requests for cloud infrastructure
  settings that exist only in web dashboards.
concepts:
  - infrastructure-management
  - auditability
  - operational-hygiene
atom_type: insight
visibility: private
ingested_at: '2026-09-26T23:13:06.384Z'
source_hash: 'pending:c4ea82ec2261f35a'
source_kind: put_page
source_slug: docs/deploy_runbook
extracted_at: '2026-09-26T23:13:06.376Z'
extracted_by: extract_atoms-v0.41.2.1
ingested_via: put_page
virality_score: 62
quote_unverified: model paraphrased; not present in source
emotional_register: practical
managed_extraction: true
---

Settings configured solely inside cloud provider web dashboards leave no git trail and are easily missed during migrations or audits. When infrastructure components like lifecycle rules cannot be defined in code, requiring exported configurations or screenshots attached directly to the pull request provides necessary accountability. Without code-managed infrastructure, PR verification checklists act as a crucial paper trail.
