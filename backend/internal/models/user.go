package models

import "encoding/json"

type SubtitlePrefs struct {
	Color      string  `json:"color"`
	BgOpacity  float64 `json:"bg_opacity"`
	FontSize   int     `json:"font_size"`
	TextStyle  string  `json:"text_style"`
	FontFamily string  `json:"font_family"`
}

func DefaultSubtitlePrefs() SubtitlePrefs {
	return SubtitlePrefs{
		Color:      "#ffffff",
		BgOpacity:  0.5,
		FontSize:   100,
		TextStyle:  "drop-shadow",
		FontFamily: "sans-serif",
	}
}

type User struct {
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	DisplayName      string `json:"display_name"`
	Password         string `json:"-"` // bcrypt hash, never serialised
	RootDir          string `json:"root_dir"`
	IsAdmin          bool   `json:"is_admin"`
	SubtitlePrefsRaw string `json:"-"`
}

// UserResponse is the safe subset sent to clients.
type UserResponse struct {
	ID            int64         `json:"id"`
	Username      string        `json:"username"`
	DisplayName   string        `json:"display_name"`
	RootDir       string        `json:"root_dir"`
	IsAdmin       bool          `json:"is_admin"`
	SubtitlePrefs SubtitlePrefs `json:"subtitle_prefs"`
}

func (u *User) ToResponse() UserResponse {
	prefs := DefaultSubtitlePrefs()
	if u.SubtitlePrefsRaw != "" && u.SubtitlePrefsRaw != "{}" {
		var p SubtitlePrefs
		if err := json.Unmarshal([]byte(u.SubtitlePrefsRaw), &p); err == nil {
			prefs = p
			if prefs.Color == "" {
				prefs.Color = "#ffffff"
			}
			if prefs.FontSize == 0 {
				prefs.FontSize = 100
			}
			if prefs.TextStyle == "" {
				prefs.TextStyle = "drop-shadow"
			}
			if prefs.FontFamily == "" {
				prefs.FontFamily = "sans-serif"
			}
		}
	}
	return UserResponse{
		ID:            u.ID,
		Username:      u.Username,
		DisplayName:   u.DisplayName,
		RootDir:       u.RootDir,
		IsAdmin:       u.IsAdmin,
		SubtitlePrefs: prefs,
	}
}
