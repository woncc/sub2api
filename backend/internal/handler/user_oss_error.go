package handler

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// userOSSStorageOp returns the storage operation name for logs.
// The wrapped SDK error is omitted because it can echo credentials.
func userOSSStorageOp(err error) string {
	var storageErr *service.UserOSSStorageError
	if errors.As(err, &storageErr) && storageErr != nil && storageErr.Op != "" {
		return storageErr.Op
	}
	return "store"
}
