package store

import "errors"

// ErrNotFound is returned by store methods that target a single row when
// no row matched. The service layer should map this to HTTP 404; do not
// leak pgx.ErrNoRows above the store boundary.
var ErrNotFound = errors.New("store: row not found")

// ErrPhotoLimit is returned when a photo-append would push a daily log's
// photo_asset_ids past the per-log cap. The service layer maps it to a 400
// VALIDATION_ERROR.
var ErrPhotoLimit = errors.New("store: photo limit exceeded")
