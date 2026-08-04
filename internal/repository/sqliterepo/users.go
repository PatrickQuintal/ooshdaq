package sqliterepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PatrickQuintal/ooshdaq/internal/domain"
	"github.com/PatrickQuintal/ooshdaq/internal/repository"
)

var _ repository.UserStore = (*Store)(nil)
var _ repository.UserActivityWriter = (*Store)(nil)

func (store *Store) GetUser(ctx context.Context, id string) (domain.User, error) {
	if err := validateUserID(id); err != nil {
		return domain.User{}, invalidArgument("get user", err)
	}
	var user domain.User
	err := store.database.QueryRowContext(
		ctx,
		"SELECT id, display_name FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, notFound("get user", "user", id)
	}
	if err != nil {
		return domain.User{}, classify("get user", err)
	}
	return user, nil
}

func (store *Store) UpsertUser(ctx context.Context, user domain.User) error {
	if err := user.Validate(); err != nil {
		return invalidArgument("upsert user", err)
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	now := store.now().UTC().Format(time.RFC3339Nano)
	_, err := store.database.ExecContext(
		ctx,
		`INSERT INTO users(id, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			display_name = excluded.display_name,
			updated_at = excluded.updated_at`,
		user.ID,
		user.DisplayName,
		now,
		now,
	)
	return classify("upsert user", err)
}

func (store *Store) TouchUserLastSeen(ctx context.Context, id string) error {
	if err := validateUserID(id); err != nil {
		return invalidArgument("touch user last seen", err)
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()

	now := store.now().UTC()
	result, err := store.database.ExecContext(
		ctx,
		`UPDATE users
		 SET last_seen_at = ?
		 WHERE id = ?
			AND (
				last_seen_at IS NULL
				OR unixepoch(last_seen_at) <= unixepoch(?)
			)`,
		now.Format(time.RFC3339Nano),
		id,
		now.Add(-repository.LastSeenTouchInterval).Format(time.RFC3339Nano),
	)
	if err != nil {
		return classify("touch user last seen", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return classify("read touched user count", err)
	}
	if changed != 0 {
		return nil
	}
	var exists int
	err = store.database.QueryRowContext(
		ctx,
		"SELECT 1 FROM users WHERE id = ?",
		id,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("touch user last seen", "user", id)
	}
	return classify("check touched user", err)
}

func validateUserID(id string) error {
	candidate := domain.User{ID: id, DisplayName: "Member"}
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("invalid user id: %w", err)
	}
	return nil
}
