package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type fieldInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type positionInput struct {
	FieldID     int64           `json:"field_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Permissions map[string]bool `json:"permissions"`
}

type userInput struct {
	Username   string `json:"username"`
	FullName   string `json:"full_name"`
	Password   string `json:"password"`
	PositionID int64  `json:"position_id"`
	ResidentID int64  `json:"resident_id"`
	Active     *bool  `json:"active"`
}

func (a *App) fields(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(r.Context(), "SELECT id,name,description FROM organizational_fields ORDER BY name")
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		type item struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		out := []item{}
		for rows.Next() {
			var x item
			if err := rows.Scan(&x.ID, &x.Name, &x.Description); err != nil {
				fail(w, err)
				return
			}
			out = append(out, x)
		}
		jsonOut(w, http.StatusOK, out)
	case http.MethodPost:
		var input fieldInput
		if jsonIn(r, &input) != nil || strings.TrimSpace(input.Name) == "" {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama bidang wajib diisi"})
			return
		}
		var id int64
		err := a.db.QueryRow(r.Context(), "INSERT INTO organizational_fields(name,description) VALUES($1,$2) RETURNING id", strings.TrimSpace(input.Name), strings.TrimSpace(input.Description)).Scan(&id)
		if err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusCreated, map[string]any{"id": id, "name": input.Name, "description": input.Description})
	case http.MethodPut:
		id, ok := parseAdminID(w, r)
		if !ok {
			return
		}
		var input fieldInput
		if jsonIn(r, &input) != nil || strings.TrimSpace(input.Name) == "" {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama bidang wajib diisi"})
			return
		}
		result, err := a.db.Exec(r.Context(), "UPDATE organizational_fields SET name=$1,description=$2 WHERE id=$3", strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), id)
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "bidang tidak ditemukan"})
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"updated": true})
	case http.MethodDelete:
		id, ok := parseAdminID(w, r)
		if !ok {
			return
		}
		var inUse bool
		if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM positions WHERE field_id=$1)", id).Scan(&inUse); err != nil {
			fail(w, err)
			return
		}
		if inUse {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "bidang masih digunakan oleh jabatan"})
			return
		}
		result, err := a.db.Exec(r.Context(), "DELETE FROM organizational_fields WHERE id=$1", id)
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "bidang tidak ditemukan"})
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) positions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(r.Context(), `SELECT p.id,p.field_id,COALESCE(f.name,''),p.name,p.description
			FROM positions p LEFT JOIN organizational_fields f ON f.id=p.field_id ORDER BY f.name,p.name`)
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		type item struct {
			ID          int64           `json:"id"`
			FieldID     *int64          `json:"field_id"`
			Field       string          `json:"field"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Permissions map[string]bool `json:"permissions"`
		}
		out := []item{}
		for rows.Next() {
			var x item
			if err := rows.Scan(&x.ID, &x.FieldID, &x.Field, &x.Name, &x.Description); err != nil {
				fail(w, err)
				return
			}
			permissionRows, err := a.db.Query(r.Context(), "SELECT menu_key,can_view,can_manage FROM position_permissions WHERE position_id=$1", x.ID)
			if err != nil {
				fail(w, err)
				return
			}
			x.Permissions, err = scanPermissions(permissionRows)
			permissionRows.Close()
			if err != nil {
				fail(w, err)
				return
			}
			out = append(out, x)
		}
		jsonOut(w, http.StatusOK, map[string]any{"items": out, "menus": permissionMenus})
	case http.MethodPost, http.MethodPut:
		var id int64
		if r.Method == http.MethodPut {
			var ok bool
			id, ok = parseAdminID(w, r)
			if !ok {
				return
			}
		}
		var input positionInput
		if jsonIn(r, &input) != nil || strings.TrimSpace(input.Name) == "" {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama jabatan wajib diisi"})
			return
		}
		tx, err := a.db.Begin(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		defer tx.Rollback(r.Context())
		if r.Method == http.MethodPost {
			err = tx.QueryRow(r.Context(), `INSERT INTO positions(field_id,name,description) VALUES(NULLIF($1,0),$2,$3) RETURNING id`, input.FieldID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description)).Scan(&id)
		} else {
			err = tx.QueryRow(r.Context(), `UPDATE positions SET field_id=NULLIF($1,0),name=$2,description=$3 WHERE id=$4 RETURNING id`, input.FieldID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), id).Scan(&id)
		}
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonOut(w, http.StatusNotFound, map[string]string{"error": "jabatan tidak ditemukan"})
			} else {
				fail(w, err)
			}
			return
		}
		if err := setPositionPermissions(r.Context(), tx, id, input.Permissions); err != nil {
			fail(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"id": id, "name": input.Name})
	case http.MethodDelete:
		id, ok := parseAdminID(w, r)
		if !ok {
			return
		}
		var inUse bool
		if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM app_users WHERE position_id=$1)", id).Scan(&inUse); err != nil {
			fail(w, err)
			return
		}
		if inUse {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "jabatan masih digunakan akun"})
			return
		}
		result, err := a.db.Exec(r.Context(), "DELETE FROM positions WHERE id=$1", id)
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "jabatan tidak ditemukan"})
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) users(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(r.Context(), `SELECT u.id,u.username,u.full_name,u.position_id,p.name,COALESCE(u.resident_id,0),u.active
			FROM app_users u JOIN positions p ON p.id=u.position_id ORDER BY u.full_name`)
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		type item struct {
			ID         int64  `json:"id"`
			Username   string `json:"username"`
			FullName   string `json:"full_name"`
			PositionID int64  `json:"position_id"`
			Position   string `json:"position"`
			ResidentID int64  `json:"resident_id"`
			Active     bool   `json:"active"`
		}
		out := []item{}
		for rows.Next() {
			var x item
			if err := rows.Scan(&x.ID, &x.Username, &x.FullName, &x.PositionID, &x.Position, &x.ResidentID, &x.Active); err != nil {
				fail(w, err)
				return
			}
			out = append(out, x)
		}
		jsonOut(w, http.StatusOK, out)
	case http.MethodPost, http.MethodPut:
		var id int64
		if r.Method == http.MethodPut {
			var ok bool
			id, ok = parseAdminID(w, r)
			if !ok {
				return
			}
		}
		var input userInput
		if jsonIn(r, &input) != nil || strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.FullName) == "" || input.PositionID < 1 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "username, nama, dan jabatan wajib diisi"})
			return
		}
		if r.Method == http.MethodPost && len(input.Password) < 12 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "password minimal 12 karakter"})
			return
		}
		var positionName string
		if err := a.db.QueryRow(r.Context(), "SELECT name FROM positions WHERE id=$1", input.PositionID).Scan(&positionName); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "jabatan tidak ditemukan"})
			} else {
				fail(w, err)
			}
			return
		}
		if positionName == "Warga" && input.ResidentID < 1 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "akun Warga wajib ditautkan ke rumah/KK"})
			return
		}
		if input.ResidentID > 0 {
			var residentExists bool
			if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM residents WHERE id=$1 AND active=true)", input.ResidentID).Scan(&residentExists); err != nil {
				fail(w, err)
				return
			}
			if !residentExists {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "rumah/KK tidak ditemukan atau tidak aktif"})
				return
			}
		}
		var passwordHash any
		if input.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
			if err != nil {
				fail(w, err)
				return
			}
			passwordHash = string(hash)
		}
		active := true
		if input.Active != nil {
			active = *input.Active
		}
		var err error
		if r.Method == http.MethodPost {
			err = a.db.QueryRow(r.Context(), `INSERT INTO app_users(username,full_name,password_hash,position_id,resident_id,active)
				VALUES($1,$2,$3,$4,NULLIF($5,0),$6) RETURNING id`, strings.TrimSpace(input.Username), strings.TrimSpace(input.FullName), passwordHash, input.PositionID, input.ResidentID, active).Scan(&id)
		} else if passwordHash != nil {
			err = a.db.QueryRow(r.Context(), `UPDATE app_users SET username=$1,full_name=$2,password_hash=$3,position_id=$4,resident_id=NULLIF($5,0),active=$6
				WHERE id=$7 RETURNING id`, strings.TrimSpace(input.Username), strings.TrimSpace(input.FullName), passwordHash, input.PositionID, input.ResidentID, active, id).Scan(&id)
		} else {
			err = a.db.QueryRow(r.Context(), `UPDATE app_users SET username=$1,full_name=$2,position_id=$3,resident_id=NULLIF($4,0),active=$5
				WHERE id=$6 RETURNING id`, strings.TrimSpace(input.Username), strings.TrimSpace(input.FullName), input.PositionID, input.ResidentID, active, id).Scan(&id)
		}
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonOut(w, http.StatusNotFound, map[string]string{"error": "akun tidak ditemukan"})
			} else {
				fail(w, err)
			}
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"id": id, "username": input.Username, "active": active})
	case http.MethodDelete:
		id, ok := parseAdminID(w, r)
		if !ok {
			return
		}
		principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
		if id == principal.UserID {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "akun yang sedang login tidak dapat dihapus"})
			return
		}
		result, err := a.db.Exec(r.Context(), "DELETE FROM app_users WHERE id=$1", id)
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "akun tidak ditemukan"})
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func parseAdminID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "id tidak valid"})
		return 0, false
	}
	return id, true
}
