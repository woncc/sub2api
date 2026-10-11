package service

import (
	"errors"
	"fmt"
)

// UserOSSClientMessage is the fixed client-facing text for a user object-storage
// failure. Underlying SDK errors stay in logs and must not be copied into the response.
const UserOSSClientMessage = "Failed to store output in your object storage"

// UserOSSStorageError is a user object-storage bind or offload failure.
// The upstream call may already have succeeded. Handlers must not record it
// as an account schedule failure and must not fail over to another account.
// Create responses stay unwritten: returning the upstream task id would let
// clients poll a platform URL and bypass the bound user store (#5).
type UserOSSStorageError struct {
	Op  string
	Err error
}

func (e *UserOSSStorageError) Error() string {
	if e == nil {
		return "user object storage error"
	}
	op := e.Op
	if op == "" {
		op = "store"
	}
	if e.Err == nil {
		return "user object storage " + op + " failed"
	}
	return fmt.Sprintf("user object storage %s: %v", op, e.Err)
}

func (e *UserOSSStorageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsUserOSSStorageError reports whether err is a user object-storage failure.
func IsUserOSSStorageError(err error) bool {
	var target *UserOSSStorageError
	return errors.As(err, &target)
}

func userOSSStorageError(op string, err error) error {
	if err == nil {
		return nil
	}
	if IsUserOSSStorageError(err) {
		return err
	}
	return &UserOSSStorageError{Op: op, Err: err}
}
