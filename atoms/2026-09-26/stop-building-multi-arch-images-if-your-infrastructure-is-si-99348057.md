---
type: atom
title: Stop building multi-arch images if your infrastructure is single-arch
lesson: >-
  Target only your production architecture on native CI runners instead of
  paying emulation overhead for unused platform targets.
concepts:
  - ci-cd
  - containerization
  - cloud-infrastructure
atom_type: strategy
visibility: private
ingested_at: '2026-09-26T23:12:59.089Z'
source_hash: 'pending:1350b6d9850ec4e8'
source_kind: put_page
source_slug: changelog
extracted_at: '2026-09-26T23:12:59.075Z'
extracted_by: extract_atoms-v0.41.2.1
ingested_via: put_page
virality_score: 65
quote_unverified: model paraphrased; not present in source
emotional_register: practical
managed_extraction: true
---

Multi-architecture container builds often incur massive QEMU emulation overhead on standard CI runners without providing runtime value. Building unused x86 images while emulating ARM64 for an ARM-only deployment host significantly slowed pipeline execution. Switching to a native single-arch ARM runner dropped build times from up to seven minutes down to under two minutes.
