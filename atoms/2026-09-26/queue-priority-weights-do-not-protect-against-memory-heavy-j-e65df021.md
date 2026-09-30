---
type: atom
title: Queue priority weights do not protect against memory-heavy job spikes
lesson: >-
  Isolate resource-intensive background tasks into dedicated worker pools with
  hard concurrency limits rather than relying on shared-pool priority weights.
concepts:
  - background-jobs
  - resource-allocation
  - system-reliability
atom_type: insight
visibility: private
ingested_at: '2026-09-26T23:12:59.089Z'
source_hash: 'pending:1350b6d9850ec4e8'
source_kind: put_page
source_slug: changelog
extracted_at: '2026-09-26T23:12:59.057Z'
extracted_by: extract_atoms-v0.41.2.1
ingested_via: put_page
virality_score: 58
quote_unverified: model paraphrased; not present in source
emotional_register: practical
managed_extraction: true
---

Background task workers that configure queue priorities via weights instead of hard per-queue worker caps risk out-of-memory crashes when general queues go idle. If a resource-heavy task like PDF generation shares the same worker pool, it can saturate all available workers whenever higher-priority queues are empty. Isolating resource-heavy jobs onto dedicated worker instances with strict concurrency caps prevents memory exhaustion.
