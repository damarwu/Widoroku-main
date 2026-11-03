package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var weekdayNames = []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}

type jimpitanHandoverInput struct {
	SessionDate    string `json:"session_date"`
	CoordinatorOne string `json:"coordinator_one"`
	CoordinatorTwo string `json:"coordinator_two"`
	Note           string `json:"note"`
}

type rondaDayInput struct {
	DayOfWeek int      `json:"day_of_week"`
	Officers  []string `json:"officers"`
}

func ensureRondaSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS jimpitan_handovers (
		id BIGSERIAL PRIMARY KEY, session_date DATE NOT NULL UNIQUE,
		total_amount BIGINT NOT NULL CHECK(total_amount>=0),
		coordinator_one TEXT NOT NULL, coordinator_two TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '', handed_over_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS ronda_schedule (
		day_of_week SMALLINT NOT NULL CHECK(day_of_week BETWEEN 1 AND 7),
		officer_name TEXT NOT NULL,
		PRIMARY KEY(day_of_week,officer_name)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS ronda_attendance (
		session_date DATE NOT NULL, officer_name TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('HADIR','ALPA')),
		note TEXT NOT NULL DEFAULT '', recorded_by BIGINT REFERENCES app_users(id) ON DELETE SET NULL,
		recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(session_date,officer_name)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS ronda_schedule_audit (
		id BIGSERIAL PRIMARY KEY, changed_by BIGINT REFERENCES app_users(id) ON DELETE SET NULL,
		before_schedule JSONB NOT NULL, after_schedule JSONB NOT NULL,
		changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	return err
}

func (a *App) jimpitanHandovers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(r.Context(), `SELECT id,session_date::text,total_amount,coordinator_one,coordinator_two,note,handed_over_at
			FROM jimpitan_handovers ORDER BY session_date DESC,id DESC`)
		if err != nil {
			fail(w, err)
			return
		}

		defer rows.Close()
		type handover struct {
			ID             int64  `json:"id"`
			SessionDate    string `json:"session_date"`
			TotalAmount    int64  `json:"total_amount"`
			CoordinatorOne string `json:"coordinator_one"`
			CoordinatorTwo string `json:"coordinator_two"`
			Note           string `json:"note"`
			HandedOverAt   string `json:"handed_over_at"`
		}
		result := []handover{}
		for rows.Next() {
			var item handover
			if err := rows.Scan(&item.ID, &item.SessionDate, &item.TotalAmount, &item.CoordinatorOne, &item.CoordinatorTwo, &item.Note, &item.HandedOverAt); err != nil {
				fail(w, err)
				return
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, result)
	case http.MethodPost:
		var input jimpitanHandoverInput
		if jsonIn(r, &input) != nil || strings.TrimSpace(input.SessionDate) == "" ||
			strings.TrimSpace(input.CoordinatorOne) == "" || strings.TrimSpace(input.CoordinatorTwo) == "" {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "tanggal dan nama kedua koordinator wajib diisi"})
			return
		}
		first, second := strings.TrimSpace(input.CoordinatorOne), strings.TrimSpace(input.CoordinatorTwo)
		if strings.EqualFold(first, second) {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama dua koordinator harus berbeda"})
			return
		}
		if _, err := time.Parse("2006-01-02", input.SessionDate); err != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "format tanggal tidak valid"})
			return
		}
		if len(first) > 120 || len(second) > 120 || len(input.Note) > 1000 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama koordinator atau catatan terlalu panjang"})
			return
		}
		var id int64
		err := a.db.QueryRow(r.Context(), `INSERT INTO jimpitan_handovers(session_date,total_amount,coordinator_one,coordinator_two,note)
			SELECT $1,COALESCE(SUM(amount) FILTER (WHERE status='TERAMBIL'),0)::bigint,$2,$3,$4
			FROM jimpitan_transactions WHERE session_date=$1
			HAVING COUNT(*) FILTER (WHERE status='TERAMBIL') > 0
			RETURNING id`, input.SessionDate, first, second, strings.TrimSpace(input.Note)).Scan(&id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "tidak ada catatan jimpitan berhasil diambil pada tanggal tersebut"})
				return
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				jsonOut(w, http.StatusConflict, map[string]string{"error": "serah terima untuk tanggal ini sudah tercatat"})
				return
			}
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusCreated, map[string]any{"id": id, "message": "Serah terima jimpitan berhasil dicatat"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) jimpitanNightlyTotals(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT session_date::text,SUM(amount)::bigint
		FROM jimpitan_transactions WHERE status='TERAMBIL'
		GROUP BY session_date HAVING SUM(amount)>0 ORDER BY session_date DESC`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type nightlyTotal struct {
		Date   string `json:"date"`
		Amount int64  `json:"amount"`
	}
	result := []nightlyTotal{}
	for rows.Next() {
		var item nightlyTotal
		if err := rows.Scan(&item.Date, &item.Amount); err != nil {
			fail(w, err)
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, result)
}

func (a *App) rondaSchedule(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := a.db.Query(r.Context(), "SELECT day_of_week,officer_name FROM ronda_schedule ORDER BY day_of_week,officer_name")
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		officers := make([][]string, len(weekdayNames))
		for i := range officers {
			officers[i] = []string{}
		}
		for rows.Next() {
			var day int
			var name string
			if err := rows.Scan(&day, &name); err != nil {
				fail(w, err)
				return
			}
			officers[day-1] = append(officers[day-1], name)
		}
		if err := rows.Err(); err != nil {
			fail(w, err)
			return
		}
		result := make([]scheduleDay, len(weekdayNames))
		for i, day := range weekdayNames {
			result[i] = scheduleDay{DayOfWeek: i + 1, Day: day, Officers: officers[i]}
		}
		jsonOut(w, http.StatusOK, result)
	case http.MethodPost:
		var input []rondaDayInput
		if jsonIn(r, &input) != nil || len(input) != len(weekdayNames) {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "jadwal harus memuat Senin sampai Minggu"})
			return
		}
		seenDays := make([]bool, len(weekdayNames))
		cleaned := make([]rondaDayInput, len(input))
		totalOfficers := 0
		for i, day := range input {
			if day.DayOfWeek < 1 || day.DayOfWeek > len(weekdayNames) || seenDays[day.DayOfWeek-1] {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "hari jadwal tidak valid atau duplikat"})
				return
			}
			seenDays[day.DayOfWeek-1] = true
			cleaned[i].DayOfWeek = day.DayOfWeek
			names := make(map[string]struct{})
			for _, officer := range day.Officers {
				name := strings.TrimSpace(officer)
				if name == "" {
					continue
				}
				if len(name) > 120 {
					jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama petugas maksimal 120 karakter"})
					return
				}
				if _, exists := names[strings.ToLower(name)]; exists {
					continue
				}
				names[strings.ToLower(name)] = struct{}{}
				cleaned[i].Officers = append(cleaned[i].Officers, name)
				totalOfficers++
			}
			if len(cleaned[i].Officers) > 50 {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "maksimal 50 petugas untuk satu hari"})
				return
			}
			sort.Strings(cleaned[i].Officers)
		}
		if totalOfficers > 350 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "jumlah petugas jadwal terlalu banyak"})
			return
		}
		tx, err := a.db.Begin(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		defer tx.Rollback(r.Context())
		if _, err := tx.Exec(r.Context(), "LOCK TABLE ronda_schedule IN EXCLUSIVE MODE"); err != nil {
			fail(w, err)
			return
		}
		before, err := loadRondaScheduleSnapshot(r.Context(), tx)
		if err != nil {
			fail(w, err)
			return
		}
		if _, err := tx.Exec(r.Context(), "DELETE FROM ronda_schedule"); err != nil {
			fail(w, err)
			return
		}
		for _, day := range cleaned {
			for _, name := range day.Officers {
				if _, err := tx.Exec(r.Context(), "INSERT INTO ronda_schedule(day_of_week,officer_name) VALUES($1,$2)", day.DayOfWeek, name); err != nil {
					fail(w, err)
					return
				}
			}
		}
		afterDays := make([]scheduleDay, len(weekdayNames))
		for i, name := range weekdayNames {
			afterDays[i] = scheduleDay{DayOfWeek: i + 1, Day: name, Officers: []string{}}
		}
		for _, day := range cleaned {
			afterDays[day.DayOfWeek-1].Officers = day.Officers
		}
		after, err := json.Marshal(afterDays)
		if err != nil {
			fail(w, err)
			return
		}
		if string(before) != string(after) {
			principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
			if !ok {
				jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "login diperlukan"})
				return
			}
			if _, err := tx.Exec(r.Context(), `INSERT INTO ronda_schedule_audit(changed_by,before_schedule,after_schedule)
				VALUES($1,$2::jsonb,$3::jsonb)`, principal.UserID, before, after); err != nil {
				fail(w, err)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]string{"message": "Jadwal ronda berhasil disimpan"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type scheduleDay struct {
	DayOfWeek int      `json:"day_of_week"`
	Day       string   `json:"day"`
	Officers  []string `json:"officers"`
}

func loadRondaScheduleSnapshot(ctx context.Context, tx pgx.Tx) ([]byte, error) {
	officers := make([][]string, len(weekdayNames))
	for i := range officers {
		officers[i] = []string{}
	}
	rows, err := tx.Query(ctx, "SELECT day_of_week,officer_name FROM ronda_schedule ORDER BY day_of_week,officer_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var day int
		var name string
		if err := rows.Scan(&day, &name); err != nil {
			return nil, err
		}
		officers[day-1] = append(officers[day-1], name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	snapshot := make([]scheduleDay, len(weekdayNames))
	for i, day := range weekdayNames {
		snapshot[i] = scheduleDay{DayOfWeek: i + 1, Day: day, Officers: officers[i]}
	}
	return json.Marshal(snapshot)
}

type attendanceInput struct {
	SessionDate string                   `json:"session_date"`
	Officers    []attendanceOfficerInput `json:"officers"`
}

type attendanceOfficerInput struct {
	Name   string `json:"officer_name"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

func parseRondaDate(value string) (time.Time, int, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, 0, err
	}
	day := int(date.Weekday())
	if day == 0 {
		day = 7
	}
	return date, day, nil
}

func (a *App) rondaAttendance(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dateValue := r.URL.Query().Get("date")
		_, day, err := parseRondaDate(dateValue)
		if err != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "parameter date wajib memakai format YYYY-MM-DD"})
			return
		}
		rows, err := a.db.Query(r.Context(), `SELECT s.officer_name,COALESCE(a.status,''),COALESCE(a.note,'')
			FROM ronda_schedule s LEFT JOIN ronda_attendance a ON a.officer_name=s.officer_name AND a.session_date=$1
			WHERE s.day_of_week=$2 ORDER BY s.officer_name`, dateValue, day)
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		type attendanceItem struct {
			OfficerName string `json:"officer_name"`
			Status      string `json:"status"`
			Note        string `json:"note"`
		}
		result := []attendanceItem{}
		for rows.Next() {
			var item attendanceItem
			if err := rows.Scan(&item.OfficerName, &item.Status, &item.Note); err != nil {
				fail(w, err)
				return
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, result)
	case http.MethodPost:
		var input attendanceInput
		if jsonIn(r, &input) != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "data kehadiran tidak valid"})
			return
		}
		_, day, err := parseRondaDate(input.SessionDate)
		if err != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "tanggal wajib memakai format YYYY-MM-DD"})
			return
		}
		if len(input.Officers) > 50 {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "maksimal 50 petugas per malam"})
			return
		}
		tx, err := a.db.Begin(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		defer tx.Rollback(r.Context())
		rows, err := tx.Query(r.Context(), "SELECT officer_name FROM ronda_schedule WHERE day_of_week=$1 ORDER BY officer_name", day)
		if err != nil {
			fail(w, err)
			return
		}
		assigned := make(map[string]struct{})
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				fail(w, err)
				return
			}
			assigned[name] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		rows.Close()
		if len(input.Officers) != len(assigned) {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "data kehadiran harus mencakup semua petugas jadwal pada tanggal ini"})
			return
		}
		seen := make(map[string]struct{}, len(input.Officers))
		for _, item := range input.Officers {
			if _, ok := assigned[item.Name]; !ok {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama petugas tidak sesuai jadwal pada tanggal ini"})
				return
			}
			if _, ok := seen[item.Name]; ok {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nama petugas duplikat"})
				return
			}
			seen[item.Name] = struct{}{}
			if item.Status != "HADIR" && item.Status != "ALPA" {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "status kehadiran harus HADIR atau ALPA"})
				return
			}
			if len(item.Note) > 500 {
				jsonOut(w, http.StatusBadRequest, map[string]string{"error": "catatan maksimal 500 karakter"})
				return
			}
		}
		principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
		if !ok {
			jsonOut(w, http.StatusUnauthorized, map[string]string{"error": "login diperlukan"})
			return
		}
		for _, item := range input.Officers {
			_, err := tx.Exec(r.Context(), `INSERT INTO ronda_attendance(session_date,officer_name,status,note,recorded_by)
				VALUES($1,$2,$3,$4,$5) ON CONFLICT(session_date,officer_name) DO UPDATE
				SET status=EXCLUDED.status,note=EXCLUDED.note,recorded_by=EXCLUDED.recorded_by,recorded_at=now()`,
				input.SessionDate, item.Name, item.Status, strings.TrimSpace(item.Note), principal.UserID)
			if err != nil {
				fail(w, err)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]string{"message": "Kehadiran ronda berhasil disimpan"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) rondaScheduleAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT a.id,a.changed_at::text,COALESCE(u.full_name,'Pengguna dihapus'),
		a.before_schedule,a.after_schedule FROM ronda_schedule_audit a
		LEFT JOIN app_users u ON u.id=a.changed_by ORDER BY a.changed_at DESC,a.id DESC LIMIT 100`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type auditEntry struct {
		ID             int64           `json:"id"`
		ChangedAt      string          `json:"changed_at"`
		ChangedBy      string          `json:"changed_by"`
		BeforeSchedule json.RawMessage `json:"before_schedule"`
		AfterSchedule  json.RawMessage `json:"after_schedule"`
	}
	result := []auditEntry{}
	for rows.Next() {
		var item auditEntry
		if err := rows.Scan(&item.ID, &item.ChangedAt, &item.ChangedBy, &item.BeforeSchedule, &item.AfterSchedule); err != nil {
			fail(w, err)
			return
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, result)
}
