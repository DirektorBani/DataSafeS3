package storage

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotEmpty = errors.New("bucket not empty")
)
