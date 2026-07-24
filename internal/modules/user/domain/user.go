package domain

import "time"

type Role string

const (
	RoleUser       Role = "user"
	RoleSuperAdmin Role = "super_admin"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type User struct {
	ID           int64
	PublicID     string
	Email        string
	DisplayName  string
	PasswordHash string
	Role         Role
	Status       Status
	CreatedAt    time.Time
}
