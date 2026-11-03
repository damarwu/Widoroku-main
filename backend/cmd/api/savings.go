package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type savingsStageConfig struct {
	StageNumber     int `json:"stage_number"`
	MeetingCount    int `json:"meeting_count"`
	BilledResidents int `json:"billed_residents,omitempty"`
}

func parseInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func savingsStageForMeeting(ctx context.Context, db *pgxpool.Pool, meetingDate time.Time) (int, error) {
	var meetingNumber int
	if err := db.QueryRow(ctx, `SELECT COUNT(*)::int FROM periods WHERE note='Jadwal Ahad Pon otomatis' AND status<>'LIBUR' AND meeting_date<=$1`, meetingDate).Scan(&meetingNumber); err != nil {
		return 0, err
	}
	rows, err := db.Query(ctx, `SELECT stage_number,meeting_count FROM savings_stages ORDER BY stage_number`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cumulative := 0
	for rows.Next() {
		var stage, count int
		if err := rows.Scan(&stage, &count); err != nil {
			return 0, err
		}
		cumulative += count
		if meetingNumber <= cumulative {
			return stage, rows.Err()
		}
	}
	return 0, rows.Err()
}

func (a *App) tabunganSummary(w http.ResponseWriter, r *http.Request) {
	type stageSummary struct {
		Stage           int   `json:"stage_number"`
		MeetingCount    int   `json:"meeting_count"`
		Target          int64 `json:"target"`
		Collected       int64 `json:"collected"`
		Remaining       int64 `json:"remaining"`
		UnpaidResidents int   `json:"unpaid_residents"`
	}
	var residentCount int
	var targetPerStage, allStagesTarget, totalDeposits, totalExpenses int64
	if err := a.db.QueryRow(r.Context(), `SELECT COUNT(*)::int FROM residents WHERE active=true`).Scan(&residentCount); err != nil {
		fail(w, err)
		return
	}
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount) FILTER(WHERE stage_number=(SELECT MAX(stage_number) FROM savings_stages)),0)::bigint,COALESCE(SUM(amount),0)::bigint FROM savings_stage_bills`).Scan(&targetPerStage, &allStagesTarget); err != nil {
		fail(w, err)
		return
	}
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions WHERE type='SETOR'`).Scan(&totalDeposits); err != nil {
		fail(w, err)
		return
	}
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM cashbook_transactions WHERE field_key='road_savings' AND direction='KELUAR'`).Scan(&totalExpenses); err != nil {
		fail(w, err)
		return
	}
	var stageCount int
	if err := a.db.QueryRow(r.Context(), `SELECT COUNT(*)::int FROM savings_stages`).Scan(&stageCount); err != nil {
		fail(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `WITH paid AS (
		SELECT resident_id,stage_number,SUM(amount)::bigint AS amount FROM tabungan_transactions WHERE type='SETOR' AND stage_number>0 GROUP BY resident_id,stage_number
	)
	SELECT s.stage_number,s.meeting_count,COALESCE(SUM(b.amount),0)::bigint,
		COALESCE((SELECT SUM(amount) FROM tabungan_transactions WHERE stage_number=s.stage_number AND type='SETOR'),0)::bigint,
		COUNT(b.resident_id) FILTER(WHERE COALESCE(p.amount,0)<b.amount)::int
	FROM savings_stages s LEFT JOIN savings_stage_bills b ON b.stage_number=s.stage_number LEFT JOIN paid p ON p.resident_id=b.resident_id AND p.stage_number=s.stage_number
	GROUP BY s.stage_number,s.meeting_count ORDER BY s.stage_number`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	stages := []stageSummary{}
	for rows.Next() {
		var item stageSummary
		if err := rows.Scan(&item.Stage, &item.MeetingCount, &item.Target, &item.Collected, &item.UnpaidResidents); err != nil {
			fail(w, err)
			return
		}
		item.Remaining = max(item.Target-item.Collected, 0)
		stages = append(stages, item)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	expenseRows, err := a.db.Query(r.Context(), `SELECT id,transaction_date::text,amount,note FROM cashbook_transactions WHERE field_key='road_savings' AND direction='KELUAR' ORDER BY transaction_date DESC,id DESC LIMIT 20`)
	if err != nil {
		fail(w, err)
		return
	}
	defer expenseRows.Close()
	type realization struct {
		ID     int64  `json:"id"`
		Date   string `json:"date"`
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	expenses := []realization{}
	for expenseRows.Next() {
		var item realization
		if err := expenseRows.Scan(&item.ID, &item.Date, &item.Amount, &item.Note); err != nil {
			fail(w, err)
			return
		}
		expenses = append(expenses, item)
	}
	jsonOut(w, 200, map[string]any{"resident_count": residentCount, "target_per_stage": targetPerStage, "all_stages_target": allStagesTarget, "stage_count": stageCount, "total_collected": totalDeposits, "total_spent": totalExpenses, "ending_balance": totalDeposits - totalExpenses, "stages": stages, "realizations": expenses})
}

func (a *App) savingsStages(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT s.stage_number,s.meeting_count,COUNT(b.resident_id)::int FROM savings_stages s LEFT JOIN savings_stage_bills b ON b.stage_number=s.stage_number GROUP BY s.stage_number,s.meeting_count ORDER BY s.stage_number`)
	if err != nil {
		fail(w, err)
		return
	}
	stages := []savingsStageConfig{}
	for rows.Next() {
		var s savingsStageConfig
		if err := rows.Scan(&s.StageNumber, &s.MeetingCount, &s.BilledResidents); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		stages = append(stages, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()
	meetings, err := a.db.Query(r.Context(), `SELECT id,meeting_date,status FROM periods WHERE note='Jadwal Ahad Pon otomatis' AND status<>'LIBUR' ORDER BY meeting_date`)
	if err != nil {
		fail(w, err)
		return
	}
	defer meetings.Close()
	type meeting struct {
		ID     int64  `json:"id"`
		Date   string `json:"meeting_date"`
		Status string `json:"status"`
		Stage  int    `json:"stage_number"`
	}
	items := []meeting{}
	for meetings.Next() {
		var m meeting
		var date time.Time
		if err := meetings.Scan(&m.ID, &date, &m.Status); err != nil {
			fail(w, err)
			return
		}
		m.Date = date.Format("2006-01-02")
		m.Stage, err = savingsStageForMeeting(r.Context(), a.db, date)
		if err != nil {
			fail(w, err)
			return
		}
		items = append(items, m)
	}
	residents, err := a.db.Query(r.Context(), `SELECT id,house_code,head_name,savings_class FROM residents WHERE active=true ORDER BY house_code`)
	if err != nil {
		fail(w, err)
		return
	}
	type residentOption struct {
		ID           int64  `json:"id"`
		HouseCode    string `json:"house_code"`
		HeadName     string `json:"head_name"`
		SavingsClass int    `json:"savings_class"`
	}
	residentItems := []residentOption{}
	for residents.Next() {
		var item residentOption
		if err := residents.Scan(&item.ID, &item.HouseCode, &item.HeadName, &item.SavingsClass); err != nil {
			residents.Close()
			fail(w, err)
			return
		}
		residentItems = append(residentItems, item)
	}
	residents.Close()
	jsonOut(w, 200, map[string]any{"stages": stages, "meetings": items, "residents": residentItems})
}

func (a *App) saveSavingsStages(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Stages []savingsStageConfig `json:"stages"`
	}
	if jsonIn(r, &input) != nil || len(input.Stages) == 0 {
		jsonOut(w, 400, map[string]string{"error": "Tambahkan minimal satu tahap"})
		return
	}
	for i, s := range input.Stages {
		if s.StageNumber != i+1 || s.MeetingCount < 1 || s.MeetingCount > 100 {
			jsonOut(w, 400, map[string]string{"error": "Nomor tahap harus berurutan dan jumlah kumpulan minimal 1"})
			return
		}
	}
	var lockedThrough int
	if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(MAX(stage_number),0)::int FROM tabungan_transactions WHERE stage_number>0`).Scan(&lockedThrough); err != nil {
		fail(w, err)
		return
	}
	var old []savingsStageConfig
	rows, err := a.db.Query(r.Context(), `SELECT stage_number,meeting_count FROM savings_stages WHERE stage_number<=$1 ORDER BY stage_number`, lockedThrough)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var s savingsStageConfig
		if err := rows.Scan(&s.StageNumber, &s.MeetingCount); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		old = append(old, s)
	}
	rows.Close()
	for _, existing := range old {
		if existing.StageNumber > len(input.Stages) || input.Stages[existing.StageNumber-1].MeetingCount != existing.MeetingCount {
			jsonOut(w, 409, map[string]string{"error": "Pengaturan tahap yang sudah memiliki setoran tidak dapat diubah agar riwayat tetap akurat"})
			return
		}
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM savings_stages WHERE stage_number>$1`, lockedThrough); err != nil {
		fail(w, err)
		return
	}
	for _, s := range input.Stages {
		if _, err = tx.Exec(r.Context(), `INSERT INTO savings_stages(stage_number,meeting_count) VALUES($1,$2) ON CONFLICT(stage_number) DO UPDATE SET meeting_count=EXCLUDED.meeting_count`, s.StageNumber, s.MeetingCount); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"message": "Pengaturan periode tabungan tersimpan"})
}

func (a *App) createSavingsStage(w http.ResponseWriter, r *http.Request) {
	var input savingsStageConfig
	if jsonIn(r, &input) != nil || input.MeetingCount < 1 || input.MeetingCount > 100 {
		jsonOut(w, 400, map[string]string{"error": "Jumlah kumpulan minimal 1"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('savings_stages')::bigint)`); err != nil {
		fail(w, err)
		return
	}
	var number int
	if err := tx.QueryRow(r.Context(), `SELECT COALESCE(MAX(stage_number),0)+1 FROM savings_stages`).Scan(&number); err != nil {
		fail(w, err)
		return
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO savings_stages(stage_number,meeting_count) VALUES($1,$2)`, number, input.MeetingCount); err != nil {
		fail(w, err)
		return
	}
	result, err := tx.Exec(r.Context(), `INSERT INTO savings_stage_bills(stage_number,resident_id,amount)
		SELECT $1,id,CASE savings_class WHEN 1 THEN 75000 WHEN 2 THEN 50000 ELSE 25000 END FROM residents WHERE active=true AND savings_class BETWEEN 1 AND 3 ON CONFLICT(stage_number,resident_id) DO NOTHING`, number)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE savings_stages SET bills_created_at=now() WHERE stage_number=$1`, number); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 201, map[string]int{"stage_number": number, "meeting_count": input.MeetingCount, "billed_residents": int(result.RowsAffected())})
}

func (a *App) updateSavingsStage(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue("stage"))
	if err != nil || number < 1 {
		jsonOut(w, 400, map[string]string{"error": "Nomor tahap tidak valid"})
		return
	}
	var input savingsStageConfig
	if jsonIn(r, &input) != nil || input.MeetingCount < 1 || input.MeetingCount > 100 {
		jsonOut(w, 400, map[string]string{"error": "Jumlah kumpulan minimal 1"})
		return
	}
	var locked bool
	if err = a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM tabungan_transactions WHERE stage_number>=$1)`, number).Scan(&locked); err != nil {
		fail(w, err)
		return
	}
	if locked {
		jsonOut(w, 409, map[string]string{"error": "Tahap ini atau tahap sesudahnya sudah memiliki transaksi dan tidak dapat diubah"})
		return
	}
	result, err := a.db.Exec(r.Context(), `UPDATE savings_stages SET meeting_count=$1 WHERE stage_number=$2`, input.MeetingCount, number)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "Tahap tidak ditemukan"})
		return
	}
	jsonOut(w, 200, map[string]any{"stage_number": number, "meeting_count": input.MeetingCount})
}

func (a *App) deleteSavingsStage(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.PathValue("stage"))
	if err != nil || number < 1 {
		jsonOut(w, 400, map[string]string{"error": "Nomor tahap tidak valid"})
		return
	}
	var last int
	if err = a.db.QueryRow(r.Context(), `SELECT COALESCE(MAX(stage_number),0) FROM savings_stages`).Scan(&last); err != nil {
		fail(w, err)
		return
	}
	if number != last {
		jsonOut(w, 409, map[string]string{"error": "Hapus tahap terakhir terlebih dahulu agar urutan periode tetap konsisten"})
		return
	}
	var used bool
	if err = a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM tabungan_transactions WHERE stage_number>=$1)`, number).Scan(&used); err != nil {
		fail(w, err)
		return
	}
	if used {
		jsonOut(w, 409, map[string]string{"error": "Tahap yang sudah memiliki transaksi tidak dapat dihapus"})
		return
	}
	if _, err = a.db.Exec(r.Context(), `DELETE FROM savings_stages WHERE stage_number=$1`, number); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"message": "Tahap dihapus"})
}

func (a *App) updateSavingsTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonOut(w, 400, map[string]string{"error": "ID transaksi tidak valid"})
		return
	}
	var input struct {
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	if jsonIn(r, &input) != nil || input.Amount < 1 {
		jsonOut(w, 400, map[string]string{"error": "Nominal positif wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var residentID int64
	var stage int
	var class int
	var oldAmount int64
	if err = tx.QueryRow(r.Context(), `SELECT t.resident_id,t.stage_number,t.amount,r.savings_class FROM tabungan_transactions t JOIN residents r ON r.id=t.resident_id WHERE t.id=$1 AND t.type='SETOR' FOR UPDATE OF t`, id).Scan(&residentID, &stage, &oldAmount, &class); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "Setoran tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	var paid int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions WHERE resident_id=$1 AND stage_number=$2 AND type='SETOR' AND id<>$3`, residentID, stage, id).Scan(&paid); err != nil {
		fail(w, err)
		return
	}
	if class != 4 {
		var target int64
		if err = tx.QueryRow(r.Context(), `SELECT amount FROM savings_stage_bills WHERE resident_id=$1 AND stage_number=$2`, residentID, stage).Scan(&target); err != nil {
			jsonOut(w, 409, map[string]string{"error": "Tagihan tahap tidak ditemukan"})
			return
		}
		if input.Amount > target-paid {
			jsonOut(w, 409, map[string]any{"error": "Nominal melebihi sisa target tahap", "remaining_due": max(target-paid, 0)})
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE tabungan_transactions SET amount=$1,note=$2 WHERE id=$3`, input.Amount, input.Note, id); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "amount": input.Amount, "message": "Setoran diperbarui"})
}

func (a *App) deleteSavingsTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonOut(w, 400, map[string]string{"error": "ID transaksi tidak valid"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var amount int64
	if err = tx.QueryRow(r.Context(), `SELECT amount FROM tabungan_transactions WHERE id=$1 AND type='SETOR' FOR UPDATE`, id).Scan(&amount); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "Setoran tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	var collected, spent int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions WHERE type='SETOR'`).Scan(&collected); err != nil {
		fail(w, err)
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM cashbook_transactions WHERE field_key='road_savings' AND direction='KELUAR'`).Scan(&spent); err != nil {
		fail(w, err)
		return
	}
	if collected-amount < spent {
		jsonOut(w, 409, map[string]string{"error": "Setoran tidak dapat dihapus karena sebagian dana sudah tercatat keluar di buku kas"})
		return
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM tabungan_transactions WHERE id=$1`, id); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]string{"message": "Setoran dihapus"})
}
