//go:build !linux

package handlers

import "time"

func getBirthTime(_ string) *time.Time { return nil }
