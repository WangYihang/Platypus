package storage

import "errors"

// ErrNotFound is returned by repo methods when a single-row lookup yields no
// rows. Handlers map it to a 404; other errors propagate as 500. Keeping it
// as a sentinel (not a wrapping type) lets callers use == for simplicity.
var ErrNotFound = errors.New("storage: not found")

// ErrRoleBuiltin is returned by RoleRepo.Delete when the caller tries
// to delete a role marked is_builtin=1. Builtin roles are part of the
// product surface (referenced by the bootstrap seed user, by docs);
// editing the permission set is fine, but the row itself stays.
var ErrRoleBuiltin = errors.New("storage: role is builtin and cannot be deleted")

// ErrRoleInUse is returned by RoleRepo.Delete when the caller tries to
// delete a role still referenced by users.role or
// project_members.role. The handler maps this to 409 Conflict and
// surfaces the count of references so the operator can reassign
// before retrying.
var ErrRoleInUse = errors.New("storage: role is still assigned to users or project members")
