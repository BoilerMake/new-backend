package models

import (
	"errors"
	"net/http"

	"github.com/BoilerMake/new-backend/pkg/argon2"

	"github.com/gorilla/sessions"
)

// Authentication errors
var (
	ErrUserNotFound   = errors.New("user was not found")
	ErrEmailInUse     = errors.New("email is already in use")
	ErrRequiredField  = errors.New("required field is missing")
	ErrIncorrectLogin = errors.New("email or password is incorrect")
)

// Validation errors
var (
	ErrEmptyEmail           = errors.New("email is empty")
	ErrInvalidEmail         = errors.New("email is invalid")
	ErrEmptyPassword        = errors.New("password is empty")
	ErrEmptyPasswordConfirm = errors.New("password confirmation is empty")
	ErrEmptyFirstName       = errors.New("first name is empty")
	ErrEmptyLastName        = errors.New("last name is empty")
	ErrPasswordConfirm      = errors.New("password and confirmation password do not match")
)

// Password Reset errors
var (
	ErrInvalidToken = errors.New("password reset token is invalid")
	ErrExpiredToken = errors.New("password reset token has expired")
)

const (
	RoleHacker = iota
	RoleSponsor
	RoleExec
	RoleAdmin
)

// A User is an account stored in the database.
type User struct {
	ID   int `json:"id"`   // NOT NULL
	Role int `json:"role"` // NOT NULL

	Email string `json:"email"` // NOT NULL

	// Password and PasswordConfirm should only ever be in memory, never in the db
	Password        string `json:"password"`
	PasswordConfirm string `json:"passwordConfirm"`
	PasswordHash    string `json:"-"` // NOT NULL

	FirstName string `json:"firstName"` // NOT NULL
	LastName  string `json:"lastName"`  // NOT NULL

	Phone string `json:"phone"`

	IsActive         bool   `json:"isActive"`
	ConfirmationCode string `json:"confirmationCode"`
}

// EmailModel struct for password reset emails
type EmailModel struct {
	Email string `json:"email"`
}

// PasswordResetPayload struct for resetting passwords
type PasswordResetPayload struct {
	UserToken   string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// SetSession sets all the session values for a user and sets IsNew to false
// to show that the session is in use.
func (u *User) SetSession(session *sessions.Session) {
	session.Values["ID"] = u.ID
	session.Values["EMAIL"] = u.Email
	session.Values["ROLE"] = u.Role

	session.IsNew = false
}

// FromFormData converts a user from a request's FormData to a models.User
// struct.
func (u *User) FromFormData(r *http.Request) {
	u.Email = r.FormValue("email")

	u.Password = r.FormValue("password")
	u.PasswordConfirm = r.FormValue("password-confirm")

	u.FirstName = r.FormValue("first-name")
	u.LastName = r.FormValue("last-name")

	u.Phone = r.FormValue("phone")
}

// Validate checks if a User has all the necessary fields.
func (u *User) Validate() error {
	if u.Email == "" {
		return ErrEmptyEmail
	} else if u.Password == "" {
		return ErrEmptyPassword
	} else if u.PasswordConfirm == "" {
		return ErrEmptyPasswordConfirm
	} else if u.PasswordConfirm != u.Password {
		return ErrPasswordConfirm
	} else if u.FirstName == "" {
		return ErrEmptyFirstName
	} else if u.LastName == "" {
		return ErrEmptyLastName
	}

	return nil
}

// HashPassword hashes a User's password and sets its PasswordHash field.  It
// also empties its Password and PasswordConfirm fields.
func (u *User) HashPassword() error {
	passwordHash, err := argon2.DefaultParameters.HashPassword(u.Password)
	if err != nil {
		return err
	}

	// Remove any trace of unhashed password (can never be too safe)
	u.Password = ""
	u.PasswordConfirm = ""

	u.PasswordHash = passwordHash
	return nil
}

// CheckPassword compares a User's hashed password to a string.
func (u *User) CheckPassword(password string) bool {
	return argon2.CheckPassword(password, u.PasswordHash)
}

// A UserService defines an interface between the user struct (AKA the model)
// and its representation in our database.  Abstracting it to an interface
// makes it database independent, which helps with testing.
type UserService interface {
	Signup(u *User) (int, string, error)
	Login(u *User) error
	GetById(id int) (*User, error)
	GetByEmail(email string) (*User, error)
	GetByCode(code string) (*User, error)
	Update(u *User) error
	GetPasswordReset(email string) (string, error)
	ResetPassword(token string, newPassword string) error
}
