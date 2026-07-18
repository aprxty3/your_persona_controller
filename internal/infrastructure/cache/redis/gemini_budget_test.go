package redis

import (
	"testing"
	"time"
)

// The budget day must roll over at WIB midnight, not UTC midnight — the same
// boundary users experience for their monthly quota (PRD 9.1). 2026-07-17
// 18:30 UTC is already 2026-07-18 01:30 in WIB.
func TestBudgetKey_UsesWIBCalendarDay(t *testing.T) {
	utcEvening := time.Date(2026, 7, 17, 18, 30, 0, 0, time.UTC)
	if got, want := budgetKey(utcEvening), "gemini:budget:2026-07-18"; got != want {
		t.Errorf("budgetKey(%v) = %q, want %q", utcEvening, got, want)
	}

	utcMorning := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	if got, want := budgetKey(utcMorning), "gemini:budget:2026-07-17"; got != want {
		t.Errorf("budgetKey(%v) = %q, want %q", utcMorning, got, want)
	}
}
