package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/WangYihang/Platypus/internal/user"
)

// SystemUserID is the fixed id of the "system" pseudo-user that owns
// server-initiated rows whose creator isn't a real admin (mesh CA,
// seed-time project, future automation). The username is the same
// literal; the password_hash is empty so bcrypt.Compare can never
// succeed and the account is unreachable through /login.
const SystemUserID = "system"

// DefaultProjectID is the fixed project row id that the mesh config's
// `project_id: "default"` resolves to. Using a literal rather than a
// UUID lets cfg.Mesh.ProjectID point at a stable row that exists
// before the first admin bootstrap.
const DefaultProjectID = "default"

// EnsureSystemUser inserts the system pseudo-user if it isn't already
// there, and returns its id. Idempotent on every boot. The row is used
// as the `created_by` / `created_by_user` FK target for server-initiated
// records (seed-time project, mesh project CA) so those tables'
// foreign keys are satisfied before any human has logged in.
func EnsureSystemUser(ctx context.Context, db *DB) (string, error) {
	if _, err := db.Users().GetByID(ctx, SystemUserID); err == nil {
		return SystemUserID, nil
	} else if !errors.Is(err, ErrNotFound) {
		return "", fmt.Errorf("storage: lookup system user: %w", err)
	}
	err := db.Users().Create(ctx, &user.User{
		ID:           SystemUserID,
		Username:     SystemUserID,
		PasswordHash: "", // bcrypt.Compare against "" always fails → unreachable via login
		Role:         user.RoleAdmin,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("storage: create system user: %w", err)
	}
	return SystemUserID, nil
}

// EnsureDefaultProject inserts the "default" project if missing. Its
// id is the literal DefaultProjectID so cfg.Mesh.ProjectID = "default"
// points at a real row from the first boot onwards; the admin bootstrap
// path in handler_auth_v1 still runs its own GetBySlug guard but now
// always finds this row and skips creation.
func EnsureDefaultProject(ctx context.Context, db *DB, createdBy string) (*Project, error) {
	if existing, err := db.Projects().GetByID(ctx, DefaultProjectID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("storage: lookup default project: %w", err)
	}
	p := &Project{
		ID:        DefaultProjectID,
		Name:      "Default",
		Slug:      "default",
		CreatedAt: time.Now().UTC(),
		CreatedBy: createdBy,
	}
	if err := db.Projects().Create(ctx, p); err != nil {
		return nil, fmt.Errorf("storage: create default project: %w", err)
	}
	return p, nil
}
