package application

// MemberMonthlyQuota is the number of assessments a Member may complete per
// calendar month (Asia/Jakarta). Shared between the submit usecase (which
// enforces it) and the dashboard usecase (which derives remaining quota from
// it) so the two never drift out of sync — see PRD Section 5.1's quota table.
const MemberMonthlyQuota = 3
