// Package demo contains fictional local-auth identities. It is deliberately
// separate from reviewed production catalog manifests.
package demo

import "github.com/ooshdaq/ooshdaq/internal/domain"

var users = []domain.User{
	{ID: "mock-alex", DisplayName: "Alex Example"},
	{ID: "mock-bailey", DisplayName: "Bailey Example"},
	{ID: "mock-casey", DisplayName: "Casey Example"},
	{ID: "mock-devon", DisplayName: "Devon Example"},
}

// Users returns a defensive copy of the fictional local allowlist.
func Users() []domain.User {
	return append([]domain.User(nil), users...)
}
