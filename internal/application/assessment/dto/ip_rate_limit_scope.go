package dto

// IPRateLimitScope namespaces per-IP request counters for
// assessment.IPRateLimiter — lives in this leaf dto package rather than
// assessment itself for the same reason SubmitResponse does (see that file's
// doc comment): a mockery-generated mock referencing this type in the
// IPRateLimiter interface would otherwise have to import assessment back,
// hitting Go's "import cycle not allowed in test" the moment assessment's
// own in-package tests import the mock.
type IPRateLimitScope string
