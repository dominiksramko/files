package models

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"` // bcrypt hash, never serialised
	RootDir  string `json:"root_dir"`
	IsAdmin  bool   `json:"is_admin"`
}

// UserResponse is the safe subset sent to clients.
type UserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	RootDir  string `json:"root_dir"`
	IsAdmin  bool   `json:"is_admin"`
}

func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:       u.ID,
		Username: u.Username,
		RootDir:  u.RootDir,
		IsAdmin:  u.IsAdmin,
	}
}
