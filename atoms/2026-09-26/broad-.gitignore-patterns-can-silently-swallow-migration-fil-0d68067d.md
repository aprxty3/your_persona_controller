---
type: atom
title: Broad .gitignore patterns can silently swallow migration files
lesson: >-
  Avoid root-level blanket file extension patterns in `.gitignore` when
  subdirectories rely on those same extensions for version-controlled artifacts.
concepts:
  - git-workflow
  - database-migrations
  - devops
atom_type: anecdote
visibility: private
ingested_at: '2026-09-26T23:12:59.089Z'
source_hash: 'pending:1350b6d9850ec4e8'
source_kind: put_page
source_slug: changelog
extracted_at: '2026-09-26T23:12:59.025Z'
extracted_by: extract_atoms-v0.41.2.1
ingested_via: put_page
virality_score: 62
quote_unverified: model paraphrased; not present in source
emotional_register: sobering
managed_extraction: true
---

An overbroad `*.sql` rule intended for local backup dumps silently ignored newly generated database migration files in source control for weeks. Because the backup script actually created `.sql.gz` archives, the ignore rule had no legitimate purpose and invisibly masked schema files from `git status`. Overbroad file extension rules in `.gitignore` should always be narrowed to specific paths or exact extension patterns.
