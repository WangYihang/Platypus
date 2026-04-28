package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sqlite3 "modernc.org/sqlite"
	sqlite3lib "modernc.org/sqlite/lib"

	"github.com/WangYihang/Platypus/internal/storage"
	"github.com/WangYihang/Platypus/internal/user"
)

// UsersHandler exposes admin-only CRUD for the users table. The routing
// layer protects every route with RequireGlobalRole(admin); the handler
// itself assumes the caller has already passed that check.
type UsersHandler struct {
	db *storage.DB
}

func NewUsersHandler(db *storage.DB) *UsersHandler {
	return &UsersHandler{db: db}
}

type createUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
}

type updateUserRequest struct {
	Role     *string `json:"role,omitempty"`
	Password *string `json:"password,omitempty"`
}

func (h *UsersHandler) List(c *gin.Context) {
	users, err := h.db.Users().List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list users"})
		return
	}
	out := make([]userBody, 0, len(users))
	for _, u := range users {
		out = append(out, userBody{ID: u.ID, Username: u.Username, Role: u.Role})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (h *UsersHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, err := user.ParseRole(req.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hashed, err := user.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u := &user.User{
		ID:           uuid.NewString(),
		Username:     strings.TrimSpace(req.Username),
		PasswordHash: hashed,
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.db.Users().Create(c.Request.Context(), u); err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create user"})
		return
	}
	empty := ""
	RecordActivity(c, ActivityInput{
		ProjectID:   &empty,
		Category:    storage.CategoryUser,
		Action:      "user.create",
		TargetType:  "user",
		TargetID:    u.ID,
		TargetLabel: u.Username,
		Meta:        map[string]any{"username": u.Username, "role": string(u.Role)},
	})
	c.JSON(http.StatusCreated, userBody{ID: u.ID, Username: u.Username, Role: u.Role})
}

func (h *UsersHandler) Get(c *gin.Context) {
	u, err := h.db.Users().GetByID(c.Request.Context(), c.Param("id"))
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup user"})
		return
	}
	c.JSON(http.StatusOK, userBody{ID: u.ID, Username: u.Username, Role: u.Role})
}

func (h *UsersHandler) Update(c *gin.Context) {
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id := c.Param("id")
	ctx := c.Request.Context()

	if req.Role != nil {
		role, err := user.ParseRole(*req.Role)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := h.db.Users().UpdateRole(ctx, id, role); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update role"})
			return
		}
	}
	if req.Password != nil {
		hashed, err := user.HashPassword(*req.Password)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := h.db.Users().UpdatePasswordHash(ctx, id, hashed); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update password"})
			return
		}
		// Password change invalidates every existing user_session for
		// the target user. Other live tabs lose access on next request.
		// Other token kinds owned by the user are left alone — they have their
		// own rotation lifecycle and the change of an issuer's
		// password doesn't imply intent to kill their tokens.
		actor := ""
		if p, ok := PrincipalFromContext(c); ok {
			actor = p.UserID
		}
		_, _ = h.db.AuthTokens().RevokeAllSessionsForUser(ctx, id, actor, "admin password reset", time.Now().UTC())
	}

	u, err := h.db.Users().GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup updated user"})
		return
	}
	empty := ""
	meta := map[string]any{"username": u.Username}
	if req.Role != nil {
		meta["role"] = *req.Role
	}
	if req.Password != nil {
		meta["password_changed"] = true
	}
	RecordActivity(c, ActivityInput{
		ProjectID:   &empty,
		Category:    storage.CategoryUser,
		Action:      "user.update",
		TargetType:  "user",
		TargetID:    u.ID,
		TargetLabel: u.Username,
		Meta:        meta,
	})
	c.JSON(http.StatusOK, userBody{ID: u.ID, Username: u.Username, Role: u.Role})
}

func (h *UsersHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	err := h.db.Users().Delete(c.Request.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete user"})
		return
	}
	empty := ""
	RecordActivity(c, ActivityInput{
		ProjectID:  &empty,
		Category:   storage.CategoryUser,
		Action:     "user.delete",
		TargetType: "user",
		TargetID:   id,
	})
	c.Status(http.StatusNoContent)
}

// isUniqueViolation inspects a modernc.org/sqlite driver error and returns
// true iff it represents a UNIQUE constraint failure. Used to map
// username collisions to 409 Conflict instead of 500.
func isUniqueViolation(err error) bool {
	var sqliteErr *sqlite3.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	return code == sqlite3lib.SQLITE_CONSTRAINT_UNIQUE ||
		code == sqlite3lib.SQLITE_CONSTRAINT_PRIMARYKEY
}
