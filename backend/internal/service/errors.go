package service

import "errors"

var (
	ErrInvalidInput         = errors.New("invalid input")
	ErrDuplicateName        = errors.New("name already registered")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrTaskCompleted        = errors.New("task is already completed")
	ErrTaskNotFound         = errors.New("task not found")
	ErrFileTooLarge         = errors.New("file too large")
	ErrTaskLocked           = errors.New("another task is already active")
	ErrDraftConflict        = errors.New("draft version conflict")
	ErrDraftVersionNotFound = errors.New("draft version not found")
)
