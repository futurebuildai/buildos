package store

import "errors"

// ErrNotFound is returned by store methods that target a single row when
// no row matched. The service layer should map this to HTTP 404; do not
// leak pgx.ErrNoRows above the store boundary.
var ErrNotFound = errors.New("store: row not found")
