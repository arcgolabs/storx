// Package storx defines shared errors and common primitives for storx modules.
package storx

import "errors"

var (
	ErrNotFound       = errors.New("storx: not found")
	ErrClosed         = errors.New("storx: closed")
	ErrInvalidKey     = errors.New("storx: invalid key")
	ErrInvalidValue   = errors.New("storx: invalid value")
	ErrCodec          = errors.New("storx: codec error")
	ErrKeyCodec       = errors.New("storx: key codec error")
	ErrBucketNotFound = errors.New("storx: bucket not found")
)
