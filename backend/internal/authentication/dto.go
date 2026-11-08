package authentication

import (
	"giki-sports-backend/internal/user"
)

// LoginRequest defines the public JSON payload required for user login.
// Strictly restricted to email and password.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse defines the safe public JSON representation of an authenticated user.
// Excludes credentials, password hashes, session tokens, and token hashes.
type UserResponse struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"displayName"`
	Roles       []string `json:"roles"`
}

// ErrorResponse defines the standard error payload returned by authentication endpoints.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ToUserResponse converts a user domain model to the safe public UserResponse DTO.
func ToUserResponse(u *user.User) UserResponse {
	if u == nil {
		return UserResponse{}
	}
	roles := make([]string, len(u.Roles))
	for i, r := range u.Roles {
		roles[i] = string(r)
	}
	return UserResponse{
		ID:          u.ID.String(),
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Roles:       roles,
	}
}
