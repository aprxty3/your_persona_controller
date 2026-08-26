## 2026-08-26 - Pre-parsing JSON in Repository Layer for Hot Paths
**Learning:** `json.Unmarshal` within iterative scoring loops (`ComputeScores` / `scoreSJTAnswer`) is a significant performance bottleneck due to repeated parsing and allocations.
**Action:** When a domain entity contains JSON strings that are frequently read but rarely modified, parse them once during database hydration in the repository layer and store the result in the entity struct.
