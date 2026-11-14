package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is implemented by pgxpool.Pool and pgx.Tx.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Account struct {
	ApprovalState  string
	ApprovalReason *string
	ID             int64
	Role           string
	Username       string
	UsernameNorm   string
	Email          string
	EmailNorm      string
	PID            string
	PasswordHash   string
	KeyCipher      []byte
	KeyVersion     int32
	BannedAt       *time.Time
	BanReason      *string
	TOTPCipher     []byte
	LastTOTPStep   int64
	Preferences    []byte
	CreatedAt      time.Time
}

type Repository struct{ DB DBTX }

func (r Repository) ByIdentifier(ctx context.Context, role, identifier string) (Account, error) {
	var account Account
	if role == "admin" {
		err := r.DB.QueryRow(ctx, `SELECT id,'admin',username,username_normalized,'','', '',password_hash,NULL,0,NULL,NULL,totp_cipher,last_totp_step,preferences,created_at FROM admins WHERE username_normalized=$1`, strings.ToLower(strings.TrimSpace(identifier))).Scan(
			&account.ID, &account.Role, &account.Username, &account.UsernameNorm, &account.Email, &account.EmailNorm, &account.PID, &account.PasswordHash, &account.KeyCipher, &account.KeyVersion, &account.BannedAt, &account.BanReason, &account.TOTPCipher, &account.LastTOTPStep, &account.Preferences, &account.CreatedAt)
		return account, err
	}
	err := r.DB.QueryRow(ctx, `SELECT id,'merchant',username,username_normalized,email,email_normalized,pid,password_hash,key_cipher,key_version,banned_at,ban_reason,totp_cipher,last_totp_step,preferences,created_at,approval_state,approval_reason FROM merchants WHERE username_normalized=$1 OR email_normalized=$1 LIMIT 1`, strings.ToLower(strings.TrimSpace(identifier))).Scan(
		&account.ID, &account.Role, &account.Username, &account.UsernameNorm, &account.Email, &account.EmailNorm, &account.PID, &account.PasswordHash, &account.KeyCipher, &account.KeyVersion, &account.BannedAt, &account.BanReason, &account.TOTPCipher, &account.LastTOTPStep, &account.Preferences, &account.CreatedAt, &account.ApprovalState, &account.ApprovalReason)
	return account, err
}

func (r Repository) ByID(ctx context.Context, role string, id int64) (Account, error) {
	var account Account
	var err error
	if role == "admin" {
		err = r.DB.QueryRow(ctx, `SELECT id,'admin',username,username_normalized,'','', '',password_hash,NULL,0,NULL,NULL,totp_cipher,last_totp_step,preferences,created_at FROM admins WHERE id=$1`, id).Scan(
			&account.ID, &account.Role, &account.Username, &account.UsernameNorm, &account.Email, &account.EmailNorm, &account.PID, &account.PasswordHash, &account.KeyCipher, &account.KeyVersion, &account.BannedAt, &account.BanReason, &account.TOTPCipher, &account.LastTOTPStep, &account.Preferences, &account.CreatedAt)
	} else {
		err = r.DB.QueryRow(ctx, `SELECT id,'merchant',username,username_normalized,email,email_normalized,pid,password_hash,key_cipher,key_version,banned_at,ban_reason,totp_cipher,last_totp_step,preferences,created_at,approval_state,approval_reason FROM merchants WHERE id=$1`, id).Scan(
			&account.ID, &account.Role, &account.Username, &account.UsernameNorm, &account.Email, &account.EmailNorm, &account.PID, &account.PasswordHash, &account.KeyCipher, &account.KeyVersion, &account.BannedAt, &account.BanReason, &account.TOTPCipher, &account.LastTOTPStep, &account.Preferences, &account.CreatedAt, &account.ApprovalState, &account.ApprovalReason)
	}
	return account, err
}

func (a Account) DTO() UserDTO {
	locale, theme := "zh-CN", "system"
	var pref struct {
		Locale string `json:"locale"`
		Theme  string `json:"theme"`
	}
	if len(a.Preferences) > 0 && json.Unmarshal(a.Preferences, &pref) == nil {
		if pref.Locale == "en" {
			locale = pref.Locale
		}
		if pref.Theme == "light" || pref.Theme == "dark" || pref.Theme == "system" {
			theme = pref.Theme
		}
	}
	var approvalState string
	if a.ApprovalState == "pending" || a.ApprovalState == "rejected" {
		approvalState = a.ApprovalState
	}
	return UserDTO{ApprovalState: approvalState, ApprovalReason: a.ApprovalReason, ID: strconv.FormatInt(a.ID, 10), Role: a.Role, Username: a.Username, Email: a.Email, PID: a.PID, Banned: a.BannedAt != nil, BannedAt: a.BannedAt, BanReason: a.BanReason, TOTP: len(a.TOTPCipher) > 0, Locale: locale, Theme: theme, CreatedAt: a.CreatedAt}
}

var ErrAccountNotFound = errors.New("account not found")
