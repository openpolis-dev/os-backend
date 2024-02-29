package service

import "errors"

var (
	ErrInvalidInviteCode = errors.New("invalid invite code")
	ErrAlreadyInvited    = errors.New("already invited")
)
