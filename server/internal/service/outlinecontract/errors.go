// Package outlinecontract defines the interface and DTOs that decouple
// service/story (consumer) from service/outline (implementation).
// This package contains NO implementations, NO IO, and depends only on stdlib.
// Plan 11A §7.5 N5: prevents bidirectional dependency between story↔outline.
package outlinecontract

import "errors"

// ErrOutlineExpired is returned when the outline_id no longer maps to a
// valid cached outline (TTL elapsed, Redis flushed, or already accepted/refreshed/expired).
// Spec §6.5: maps to HTTP 410 outline_expired.
var ErrOutlineExpired = errors.New("outline: expired or not found in cache")

// ErrOutlineForbidden is returned when (user_id, child_id, outline_id) triple
// ownership check fails. Spec §6.5: maps to HTTP 403 forbidden.
var ErrOutlineForbidden = errors.New("outline: ownership mismatch")

// ErrOutlineNotFound is returned for refresh path when outline_id doesn't
// exist at all. Spec §6.5: maps to HTTP 404 not_found.
var ErrOutlineNotFound = errors.New("outline: not found")
