package auth

import (
	"github.com/google/uuid"

	authsvc "github.com/Harshwagh21/NimbusCloud/internal/service/auth"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	AccessToken string       `json:"access_token"`
	ExpiresIn   int64        `json:"expires_in"`
	User        userResponse `json:"user"`
}

type userResponse struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	AvatarURL    *string   `json:"avatar_url,omitempty"`
	StorageUsed  int64     `json:"storage_used"`
	StorageLimit int64     `json:"storage_limit"`
}

func newSessionResponse(result authsvc.Result) sessionResponse {
	return sessionResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   result.ExpiresIn,
		User:        newUserResponse(result.User),
	}
}

func newUserResponse(profile authsvc.Profile) userResponse {
	return userResponse{
		ID:           profile.ID,
		Email:        profile.Email,
		Name:         profile.Name,
		AvatarURL:    profile.AvatarURL,
		StorageUsed:  profile.StorageUsed,
		StorageLimit: profile.StorageLimit,
	}
}
