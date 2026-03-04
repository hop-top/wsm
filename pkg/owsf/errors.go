package owsf

import "errors"

var (
	ErrSpaceExists     = errors.New("space already exists")
	ErrParentNotFound  = errors.New("parent space not found")
	ErrParentImmutable = errors.New("parent-child immutable")
	ErrWriterNotFound  = errors.New("writer not registered")
	ErrMultiParent     = errors.New("multi-parent lineage prohibited")
	ErrCycleDetected   = errors.New("cycle detected in space tree")
)
