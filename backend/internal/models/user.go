package models

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"-"` // bcrypt hash, never serialised
	RootDir     string `json:"root_dir"`
	IsAdmin     bool   `json:"is_admin"`
}

// UserResponse is the safe subset sent to clients.
type UserResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	RootDir     string `json:"root_dir"`
	IsAdmin     bool   `json:"is_admin"`
}

func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		RootDir:     u.RootDir,
		IsAdmin:     u.IsAdmin,
	}
}
