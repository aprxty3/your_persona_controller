## 2026-08-25 - Cache SJT OptionTraitMap parsing
**Learning:** Repeatedly parsing static JSON from domain entities (like Question OptionTraitMap) inside loops (like scoring iterations) is a significant bottleneck, especially when the number of unique JSON strings is small but they are evaluated frequently.
**Action:** Introduce a localized cache (e.g. sync.Map) keyed by the raw JSON string to memoize the parsed output, reducing CPU overhead dramatically without touching the underlying domain architecture.
