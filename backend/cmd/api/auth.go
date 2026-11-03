package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type authContextKey struct{}

type AuthPrincipal struct {
	UserID      int64           `json:"id"`
	Username    string          `json:"username"`
	FullName    string          `json:"full_name"`
	PositionID  int64           `json:"position_id"`
	FieldID     int64           `json:"field_id"`
	Position    string          `json:"position"`
	ResidentID  int64           `json:"resident_id"`
	IsResident  bool            `json:"is_resident"`
	Permissions map[string]bool `json:"permissions"`
}

type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type bootstrapInput struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Password string `json:"password"`
}

var permissionMenus = []string{"dashboard", "residents", "schedule", "bills", "payments", "cashbook", "jimpitan", "savings", "pam", "ronda", "users", "positions", "fields"}

func ensureAuthSchema(ctx context.Context, db *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS organizational_fields (
			id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS positions (
			id BIGSERIAL PRIMARY KEY, field_id BIGINT REFERENCES organizational_fields(id) ON DELETE SET NULL,
			name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS position_permissions (
			position_id BIGINT NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
			menu_key TEXT NOT NULL, can_view BOOLEAN NOT NULL DEFAULT FALSE, can_manage BOOLEAN NOT NULL DEFAULT FALSE,
			PRIMARY KEY(position_id,menu_key)
		)`,
		`CREATE TABLE IF NOT EXISTS app_users (
			id BIGSERIAL PRIMARY KEY, username TEXT NOT NULL UNIQUE, full_name TEXT NOT NULL,
			password_hash TEXT NOT NULL, position_id BIGINT NOT NULL REFERENCES positions(id),
			resident_id BIGINT REFERENCES residents(id) ON DELETE SET NULL,
			active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			token_hash TEXT PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
			expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_expiry ON auth_sessions(expires_at)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO positions(name,description)
		VALUES('Warga','Akses warga untuk melihat informasi dan tagihan KK sendiri')
		ON CONFLICT(name) DO NOTHING`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO position_permissions(position_id,menu_key,can_view,can_manage)
		SELECT id,'dashboard',true,false FROM positions WHERE positions.name='Warga'
		ON CONFLICT(position_id,menu_key) DO UPDATE SET can_view=true,can_manage=false`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO position_permissions(position_id,menu_key,can_view,can_manage)
		SELECT id,'ronda',true,true FROM positions WHERE name='Administrator'
		ON CONFLICT(position_id,menu_key) DO NOTHING`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO position_permissions(position_id,menu_key,can_view,can_manage)
		SELECT id,'cashbook',true,true FROM positions WHERE name='Administrator'
		ON CONFLICT(position_id,menu_key) DO NOTHING`); err != nil {
		return err
	}
	return nil
}

func (a *App) bootstrapStatus(w http.ResponseWriter, r *http.Request) {
	var required bool
	if err := a.db.QueryRow(r.Context(), "SELECT NOT EXISTS(SELECT 1 FROM app_users)").Scan(&required); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"required": required})
}

func (a *App) bootstrapAdmin(w http.ResponseWriter, r *http.Request) {
	var input bootstrapInput
	if jsonIn(r, &input) != nil || strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.FullName) == "" || len(input.Password) < 12 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "username, nama, dan password minimal 12 karakter wajib diisi"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(w, err)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), "LOCK TABLE app_users IN EXCLUSIVE MODE"); err != nil {
		fail(w, err)
		return
	}
	var hasUsers bool
	if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM app_users)").Scan(&hasUsers); err != nil {
		fail(w, err)
		return
	}
	if hasUsers {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "akun admin awal sudah dibuat"})
		return
	}
	fieldID, err := upsertField(r.Context(), tx, "Kepengurusan RT", "Pengelolaan administrasi RT")
	if err != nil {
		fail(w, err)
		return
	}
	adminPosition, err := upsertPosition(r.Context(), tx, "Administrator", fieldID, "Pengelola seluruh menu dan hak akses")
	if err != nil {
		fail(w, err)
		return
	}
	if err := setPositionPermissions(r.Context(), tx, adminPosition, allPermissions()); err != nil {
		fail(w, err)
		return
	}
	youthField, err := upsertField(r.Context(), tx, "Kepemudaan", "Pemuda dan kegiatan lingkungan")
	if err != nil {
		fail(w, err)
		return
	}
	youthPosition, err := upsertPosition(r.Context(), tx, "Petugas Jimpitan", youthField, "Petugas pengambilan jimpitan")
	if err != nil {
		fail(w, err)
		return
	}
	if err := setPositionPermissions(r.Context(), tx, youthPosition, map[string]bool{"dashboard.view": true, "jimpitan.view": true, "jimpitan.manage": true, "ronda.view": true, "ronda.manage": true}); err != nil {
		fail(w, err)
		return
	}
	pamField, err := upsertField(r.Context(), tx, "Pelayanan Air PAM", "Pencatatan meter dan penagihan air")
	if err != nil {
		fail(w, err)
		return
	}
	pamPosition, err := upsertPosition(r.Context(), tx, "Petugas PAM", pamField, "Petugas meter dan penagihan PAM")
	if err != nil {
		fail(w, err)
		return
	}
	if err := setPositionPermissions(r.Context(), tx, pamPosition, map[string]bool{"dashboard.view": true, "pam.view": true, "pam.manage": true}); err != nil {
		fail(w, err)
		return
	}
	var userID int64
	err = tx.QueryRow(r.Context(), `INSERT INTO app_users(username,full_name,password_hash,position_id)
		VALUES($1,$2,$3,$4) RETURNING id`, strings.TrimSpace(input.Username), strings.TrimSpace(input.FullName), string(hash), adminPosition).Scan(&userID)
	if err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	token, err := a.createAuthSession(w, r, userID)
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"id": userID, "username": input.Username, "token": token})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var input loginInput
	if jsonIn(r, &input) != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "username dan password wajib diisi"})
		return
	}
	var userID int64
	var passwordHash string
	err := a.db.QueryRow(r.Context(), "SELECT id,password_hash FROM app_users WHERE username=$1 AND active=true", strings.TrimSpace(input.Username)).Scan(&userID, &passwordHash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(input.Password)) != nil {
		jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "username atau password salah"})
		return
	}
	token, err := a.createAuthSession(w, r, userID)
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"authenticated": true, "token": token})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if token := authSessionToken(r); token != "" {
		_, _ = a.db.Exec(r.Context(), "DELETE FROM auth_sessions WHERE token_hash=$1", hashSessionToken(token))
	}
	http.SetCookie(w, &http.Cookie{Name: "widoroku_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	jsonOut(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (a *App) currentUser(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok {
		jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "login diperlukan"})
		return
	}
	jsonOut(w, http.StatusOK, principal)
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/api/health" || path == "/api/auth/bootstrap-status" || path == "/api/auth/bootstrap" || path == "/api/auth/login" {
			next.ServeHTTP(w, r)
			return
		}
		token := authSessionToken(r)
		if token == "" {
			jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "login diperlukan"})
			return
		}
		var principal AuthPrincipal
		principal.Permissions = map[string]bool{}
		if err := a.db.QueryRow(r.Context(), `SELECT u.id,u.username,u.full_name,p.id,p.name,COALESCE(p.field_id,0),COALESCE(u.resident_id,0),p.name='Warga'
			FROM auth_sessions s JOIN app_users u ON u.id=s.user_id JOIN positions p ON p.id=u.position_id
			WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active=true`, hashSessionToken(token)).Scan(&principal.UserID, &principal.Username, &principal.FullName, &principal.PositionID, &principal.Position, &principal.FieldID, &principal.ResidentID, &principal.IsResident); err != nil {
			jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "sesi login tidak berlaku"})
			return
		}
		rows, err := a.db.Query(r.Context(), "SELECT menu_key,can_view,can_manage FROM position_permissions WHERE position_id=$1", principal.PositionID)
		if err != nil {
			fail(w, err)
			return
		}
		for rows.Next() {
			var menu string
			var canView, canManage bool
			if err := rows.Scan(&menu, &canView, &canManage); err != nil {
				rows.Close()
				fail(w, err)
				return
			}
			principal.Permissions[menu+".view"] = canView
			principal.Permissions[menu+".manage"] = canManage
		}
		rows.Close()
		if path != "/api/auth/me" && path != "/api/auth/logout" {
			residentAllowedPath := path == "/api/resident/dashboard" || path == "/api/dashboard" ||
				path == "/api/resident/pam/readings" || path == "/api/resident/pam/payment-choice" ||
				path == "/api/resident/pam/payment-confirmations"
			if principal.IsResident && !residentAllowedPath {
				jsonOut(w, http.StatusForbidden, map[string]string{"error": "akun warga hanya dapat mengakses dashboard KK sendiri"})
				return
			}
			permission := requiredPermission(r)
			allowed := principal.Permissions[permission]
			if permission == "" || !allowed {
				jsonOut(w, http.StatusForbidden, map[string]string{"error": "akses tidak diizinkan"})
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authContextKey{}, principal)))
	})
}

func requiredPermission(r *http.Request) string {
	path := r.URL.Path
	if strings.Contains(path, "/finance") {
		if r.Method == http.MethodGet {
			return "payments.view"
		}
		return "payments.manage"
	}
	if path == "/api/resident/pam/readings" || path == "/api/resident/pam/payment-choice" ||
		path == "/api/resident/pam/payment-confirmations" {
		return "dashboard.view"
	}
	menu := ""
	switch {
	case strings.HasPrefix(path, "/api/cashbook"):
		menu = "cashbook"
	case path == "/api/resident/dashboard":
		menu = "dashboard"
	case path == "/api/dashboard":
		menu = "dashboard"
	case strings.HasPrefix(path, "/api/residents"):
		menu = "residents"
	case strings.HasPrefix(path, "/api/periods"):
		menu = "schedule"
	case strings.HasPrefix(path, "/api/bills"):
		menu = "bills"
	case strings.HasPrefix(path, "/api/payments"):
		menu = "payments"
	case strings.HasPrefix(path, "/api/jimpitan"):
		menu = "jimpitan"
	case strings.HasPrefix(path, "/api/ronda"):
		menu = "ronda"
	case strings.HasPrefix(path, "/api/tabungan"):
		menu = "savings"
	case strings.HasPrefix(path, "/api/pam"):
		menu = "pam"
	case strings.HasPrefix(path, "/api/admin/users"):
		menu = "users"
	case strings.HasPrefix(path, "/api/admin/positions"):
		menu = "positions"
	case strings.HasPrefix(path, "/api/admin/fields"):
		menu = "fields"
	default:
		return ""
	}
	action := "view"
	if r.Method != http.MethodGet {
		action = "manage"
	}
	return menu + "." + action
}

func (a *App) createAuthSession(w http.ResponseWriter, r *http.Request, userID int64) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().Add(12 * time.Hour)
	if _, err := a.db.Exec(r.Context(), "INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", hashSessionToken(token), userID, expires); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: "widoroku_session", Value: token, Path: "/", Expires: expires, MaxAge: 12 * 60 * 60, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil})
	return token, nil
}

func authSessionToken(r *http.Request) string {
	const prefix = "Bearer "
	if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	}
	cookie, err := r.Cookie("widoroku_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func hashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func upsertField(ctx context.Context, tx pgx.Tx, name, description string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO organizational_fields(name,description) VALUES($1,$2)
		ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description RETURNING id`, name, description).Scan(&id)
	return id, err
}

func upsertPosition(ctx context.Context, tx pgx.Tx, name string, fieldID int64, description string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO positions(name,field_id,description) VALUES($1,$2,$3)
		ON CONFLICT(name) DO UPDATE SET field_id=EXCLUDED.field_id,description=EXCLUDED.description RETURNING id`, name, fieldID, description).Scan(&id)
	return id, err
}

func allPermissions() map[string]bool {
	permissions := make(map[string]bool, len(permissionMenus)*2)
	for _, menu := range permissionMenus {
		permissions[menu+".view"] = true
		permissions[menu+".manage"] = true
	}
	return permissions
}

func setPositionPermissions(ctx context.Context, tx pgx.Tx, positionID int64, permissions map[string]bool) error {
	if _, err := tx.Exec(ctx, "DELETE FROM position_permissions WHERE position_id=$1", positionID); err != nil {
		return err
	}
	for _, menu := range permissionMenus {
		canManage := permissions[menu+".manage"]
		canView := permissions[menu+".view"] || canManage
		if _, err := tx.Exec(ctx, `INSERT INTO position_permissions(position_id,menu_key,can_view,can_manage)
			VALUES($1,$2,$3,$4)`, positionID, menu, canView, canManage); err != nil {
			return err
		}
	}
	return nil
}

func scanPermissions(rows pgx.Rows) (map[string]bool, error) {
	permissions := map[string]bool{}
	for rows.Next() {
		var menu string
		var canView, canManage bool
		if err := rows.Scan(&menu, &canView, &canManage); err != nil {
			return nil, err
		}
		permissions[menu+".view"] = canView
		permissions[menu+".manage"] = canManage
	}
	return permissions, rows.Err()
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
