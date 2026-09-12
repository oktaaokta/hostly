package domain

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrClosed       = errors.New("venue is closed")
	ErrDuplicate    = errors.New("already in the queue")
	ErrNotWaiting   = errors.New("party is not waiting")
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalid      = errors.New("invalid input")
)
