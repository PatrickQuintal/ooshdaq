package sqliterepo

import (
	"context"
	"errors"

	"github.com/PatrickQuintal/ooshdaq/internal/repository"
	sqlite "modernc.org/sqlite"
)

func classify(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var sqliteError *sqlite.Error
	if !errors.As(err, &sqliteError) {
		return repository.NewFailure(repository.FailureUnavailable, operation, err)
	}

	var kind repository.FailureKind
	switch sqliteError.Code() & 0xff {
	case 5, 6:
		kind = repository.FailureBusy
	case 8:
		kind = repository.FailureReadOnly
	case 10, 14, 15:
		kind = repository.FailureUnavailable
	case 11, 26:
		kind = repository.FailureCorrupt
	case 13:
		kind = repository.FailureFull
	case 19:
		kind = repository.FailureConflict
	default:
		kind = repository.FailureUnavailable
	}
	return repository.NewFailure(kind, operation, err)
}

func notFound(operation, resource, id string) error {
	return repository.NewFailure(
		repository.FailureNotFound,
		operation,
		errors.New(resource+" "+id+" does not exist"),
	)
}

func invalidArgument(operation string, err error) error {
	return repository.NewFailure(repository.FailureInvalidArgument, operation, err)
}
