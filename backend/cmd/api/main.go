package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Iuran kas per kumpulan Ahad Pon. Ubah di satu tempat ini bila nominal berubah.
const (
	meetingBillAmount      int64 = 20000
	meetingBillDescription       = "Iuran kumpulan"
)

type App struct{ db *pgxpool.Pool }
type Resident struct {
	ID           int64  `json:"id"`
	HouseCode    string `json:"house_code"`
	HeadName     string `json:"head_name"`
	Address      string `json:"address"`
	QRToken      string `json:"qr_token"`
	Phone        string `json:"phone"`
	SavingsClass int    `json:"savings_class"`
	DuesRequired bool   `json:"dues_required"`
}
type Period struct {
	ID          int64           `json:"id"`
	StartDate   string          `json:"start_date"`
	MeetingDate string          `json:"meeting_date"`
	MeetingTime string          `json:"meeting_time"`
	Status      string          `json:"status"`
	Note        string          `json:"note"`
	Finance     *MeetingFinance `json:"finance,omitempty"`
	// Ringkasan tagihan periode ini, dipakai UI untuk konfirmasi sebelum
	// kumpulan dibatalkan (tagihan belum dibayar akan dihapus).
	BillsCount        int   `json:"bills_count"`
	BillsUnpaid       int   `json:"bills_unpaid"`
	BillsUnpaidAmount int64 `json:"bills_unpaid_amount"`
}
type PeriodSeriesInput struct {
	StartDate   string `json:"start_date"`
	MeetingTime string `json:"meeting_time"`
	ThroughDate string `json:"through_date"`
}
type MeetingFinance struct {
	PeriodID        int64 `json:"period_id"`
	PaymentCount    int   `json:"payment_count"`
	TotalCollected  int64 `json:"total_collected"`
	PeneranganJalan int64 `json:"penerangan_jalan"`
	Sosial          int64 `json:"sosial"`
	Kemasjidan      int64 `json:"kemasjidan"`
	Ziswaf          int64 `json:"ziswaf"`
	HostConsumption int64 `json:"host_consumption"`
	KasRT           int64 `json:"kas_rt"`
}
type Bill struct {
	ID           int64  `json:"id"`
	ResidentID   int64  `json:"resident_id"`
	HouseCode    string `json:"house_code"`
	HeadName     string `json:"head_name"`
	PeriodID     int64  `json:"period_id"`
	MeetingDate  string `json:"meeting_date"`
	PeriodStatus string `json:"period_status"`
	Description  string `json:"description"`
	Amount       int64  `json:"amount"`
	Paid         int64  `json:"paid"`
	Remaining    int64  `json:"remaining"`
}
type PaymentInput struct {
	ResidentID int64   `json:"resident_id"`
	Amount     int64   `json:"amount"`
	Note       string  `json:"note"`
	BillIDs    []int64 `json:"bill_ids"`
	Voluntary  bool    `json:"voluntary"`
}
type PaymentRecord struct {
	ID         int64  `json:"id"`
	ResidentID int64  `json:"resident_id"`
	HouseCode  string `json:"house_code"`
	HeadName   string `json:"head_name"`
	Amount     int64  `json:"amount"`
	Note       string `json:"note"`
	ReceivedAt string `json:"received_at"`
	Allocated  int64  `json:"allocated"`
}
type JimpitanInput struct {
	QRToken     string `json:"qr_token"`
	SessionDate string `json:"session_date"`
	Amount      int64  `json:"amount"`
	Status      string `json:"status"`
	Note        string `json:"note"`
}
type TabunganInput struct {
	GroupName  string `json:"group_name"`
	MemberName string `json:"member_name"`
	ResidentID int64  `json:"resident_id"`
	PeriodID   int64  `json:"period_id"`
	Amount     int64  `json:"amount"`
	Type       string `json:"type"`
	Note       string `json:"note"`
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://widoroku:widoroku-dev-password@localhost:5432/widoroku?sslmode=disable"
	}
	ctx := context.Background()
	var pool *pgxpool.Pool
	var err error
	for i := 0; i < 20; i++ {
		pool, err = pgxpool.New(ctx, dsn)
		if err == nil {
			err = pool.Ping(ctx)
			if err == nil {
				break
			}
			pool.Close()
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()
	if err := ensureMeetingFinanceSchema(ctx, pool); err != nil {
		log.Fatalf("meeting finance schema initialization failed: %v", err)
	}
	if err := ensureAuthSchema(ctx, pool); err != nil {
		log.Fatalf("authentication schema initialization failed: %v", err)
	}
	if err := ensurePamSchema(ctx, pool); err != nil {
		log.Fatalf("PAM schema initialization failed: %v", err)
	}
	if err := ensureRondaSchema(ctx, pool); err != nil {
		log.Fatalf("ronda schema initialization failed: %v", err)
	}
	app := &App{db: pool}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", app.health)
	mux.HandleFunc("GET /api/auth/bootstrap-status", app.bootstrapStatus)
	mux.HandleFunc("POST /api/auth/bootstrap", app.bootstrapAdmin)
	mux.HandleFunc("POST /api/auth/login", app.login)
	mux.HandleFunc("POST /api/auth/logout", app.logout)
	mux.HandleFunc("GET /api/auth/me", app.currentUser)
	mux.HandleFunc("GET /api/admin/fields", app.fields)
	mux.HandleFunc("POST /api/admin/fields", app.fields)
	mux.HandleFunc("PUT /api/admin/fields/{id}", app.fields)
	mux.HandleFunc("DELETE /api/admin/fields/{id}", app.fields)
	mux.HandleFunc("GET /api/admin/positions", app.positions)
	mux.HandleFunc("POST /api/admin/positions", app.positions)
	mux.HandleFunc("PUT /api/admin/positions/{id}", app.positions)
	mux.HandleFunc("DELETE /api/admin/positions/{id}", app.positions)
	mux.HandleFunc("GET /api/admin/users", app.users)
	mux.HandleFunc("POST /api/admin/users", app.users)
	mux.HandleFunc("PUT /api/admin/users/{id}", app.users)
	mux.HandleFunc("DELETE /api/admin/users/{id}", app.users)
	mux.HandleFunc("GET /api/pam/settings", app.pamSettings)
	mux.HandleFunc("PUT /api/pam/settings", app.pamSettings)
	mux.HandleFunc("GET /api/pam/customers", app.pamCustomers)
	mux.HandleFunc("GET /api/pam/eligible-residents", app.pamEligibleResidents)
	mux.HandleFunc("POST /api/pam/customers", app.pamCustomers)
	mux.HandleFunc("PUT /api/pam/customers/{id}", app.pamCustomer)
	mux.HandleFunc("DELETE /api/pam/customers/{id}", app.pamCustomer)
	mux.HandleFunc("GET /api/pam/bills", app.pamBills)
	mux.HandleFunc("POST /api/pam/bills/fee-only", app.pamFeeOnlyBill)
	mux.HandleFunc("GET /api/pam/payment-submissions", app.pamPaymentSubmissions)
	mux.HandleFunc("GET /api/pam/payment-submissions/{id}/proof", app.pamPaymentSubmissionProof)
	mux.HandleFunc("POST /api/pam/payment-submissions/{id}/review", app.pamReviewPaymentSubmission)
	mux.HandleFunc("GET /api/pam/commissions", app.pamCommissions)
	mux.HandleFunc("GET /api/pam/payments", app.pamPayment)
	mux.HandleFunc("PUT /api/pam/payments/{id}", app.pamPaymentRecord)
	mux.HandleFunc("DELETE /api/pam/payments/{id}", app.pamPaymentRecord)
	mux.HandleFunc("GET /api/pam/readings/pending", app.pamPendingReadings)
	mux.HandleFunc("POST /api/pam/readings/{id}/review", app.pamReviewReading)
	mux.HandleFunc("POST /api/pam/readings", app.pamReading)
	mux.HandleFunc("POST /api/pam/payments", app.pamPayment)
	mux.HandleFunc("POST /api/pam/payments/collective", app.pamCollectivePayment)
	mux.HandleFunc("GET /api/dashboard", app.dashboard)
	mux.HandleFunc("GET /api/resident/dashboard", app.residentDashboard)
	mux.HandleFunc("POST /api/resident/pam/readings", app.residentPAMReading)
	mux.HandleFunc("POST /api/resident/pam/payment-choice", app.residentPAMPaymentChoice)
	mux.HandleFunc("POST /api/resident/pam/payment-confirmations", app.residentPAMPaymentConfirmation)
	mux.HandleFunc("GET /api/residents", app.residents)
	mux.HandleFunc("POST /api/residents", app.createResident)
	mux.HandleFunc("PUT /api/residents/{id}/contact", app.updateResidentContact)
	mux.HandleFunc("PUT /api/residents/{id}", app.updateResident)
	mux.HandleFunc("POST /api/residents/import", app.importResidents)
	mux.HandleFunc("GET /api/periods", app.periods)
	mux.HandleFunc("POST /api/periods", app.createPeriod)
	mux.HandleFunc("PUT /api/periods/{id}", app.updatePeriod)
	mux.HandleFunc("DELETE /api/periods/{id}", app.deletePeriod)
	mux.HandleFunc("POST /api/periods/generate-series", app.generatePeriodSeries)
	mux.HandleFunc("POST /api/periods/{id}/finance", app.saveMeetingFinance)
	mux.HandleFunc("GET /api/periods/{id}/finance", app.meetingFinancePreview)
	mux.HandleFunc("POST /api/periods/{id}/finance/confirm", app.confirmMeetingFinanceReceipt)
	mux.HandleFunc("GET /api/finance/dates", app.financeReceiptDates)
	mux.HandleFunc("GET /api/finance/daily/{date}", app.dailyFinancePreview)
	mux.HandleFunc("POST /api/finance/daily/{date}", app.saveDailyFinance)
	mux.HandleFunc("POST /api/finance/daily/{date}/confirm", app.confirmDailyFinanceReceipt)
	mux.HandleFunc("GET /api/cashbook", app.cashbook)
	mux.HandleFunc("POST /api/cashbook/transactions", app.cashbookExpense)
	mux.HandleFunc("GET /api/periods/{id}/invitation", app.periodInvitation)
	mux.HandleFunc("POST /api/periods/{id}/generate-bills", app.generateBills)
	mux.HandleFunc("GET /api/bills", app.bills)
	mux.HandleFunc("POST /api/bills/sync", app.syncMeetingBills)
	mux.HandleFunc("GET /api/payments", app.listPayments)
	mux.HandleFunc("POST /api/payments", app.payment)
	mux.HandleFunc("PUT /api/payments/{id}", app.updatePayment)
	mux.HandleFunc("DELETE /api/payments/{id}", app.deletePayment)
	mux.HandleFunc("POST /api/jimpitan", app.jimpitan)
	mux.HandleFunc("GET /api/jimpitan", app.jimpitanList)
	mux.HandleFunc("GET /api/jimpitan/nightly-totals", app.jimpitanNightlyTotals)
	mux.HandleFunc("GET /api/jimpitan/handovers", app.jimpitanHandovers)
	mux.HandleFunc("POST /api/jimpitan/handovers", app.jimpitanHandovers)
	mux.HandleFunc("GET /api/ronda/schedule", app.rondaSchedule)
	mux.HandleFunc("POST /api/ronda/schedule", app.rondaSchedule)
	mux.HandleFunc("GET /api/ronda/attendance", app.rondaAttendance)
	mux.HandleFunc("POST /api/ronda/attendance", app.rondaAttendance)
	mux.HandleFunc("GET /api/ronda/audit", app.rondaScheduleAudit)
	mux.HandleFunc("POST /api/tabungan", app.tabungan)
	mux.HandleFunc("GET /api/tabungan", app.tabunganList)
	mux.HandleFunc("GET /api/tabungan/summary", app.tabunganSummary)
	mux.HandleFunc("GET /api/tabungan/transactions", app.tabunganTransactions)
	mux.HandleFunc("GET /api/tabungan/stages", app.savingsStages)
	mux.HandleFunc("PUT /api/tabungan/stages", app.saveSavingsStages)
	mux.HandleFunc("POST /api/tabungan/stages", app.createSavingsStage)
	mux.HandleFunc("PUT /api/tabungan/stages/{stage}", app.updateSavingsStage)
	mux.HandleFunc("DELETE /api/tabungan/stages/{stage}", app.deleteSavingsStage)
	mux.HandleFunc("PUT /api/tabungan/transactions/{id}", app.updateSavingsTransaction)
	mux.HandleFunc("DELETE /api/tabungan/transactions/{id}", app.deleteSavingsTransaction)
	handler := cors(app.authMiddleware(mux))
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("API_PORT")
	}
	if port == "" {
		port = "8080"
	}
	log.Printf("Widoroku API listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonIn(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if err := a.db.Ping(r.Context()); err != nil {
		jsonOut(w, 503, map[string]string{"status": "database unavailable"})
		return
	}
	jsonOut(w, 200, map[string]string{"status": "ok", "service": "widoroku-api"})
}
func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	if principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal); ok && principal.IsResident {
		a.residentDashboard(w, r)
		return
	}
	var residents int
	var bills int
	var outstanding int64
	var collected int64
	_ = a.db.QueryRow(r.Context(), "SELECT count(*) FROM residents WHERE active=true").Scan(&residents)
	_ = a.db.QueryRow(r.Context(), "SELECT count(*) FROM bills WHERE waived=false").Scan(&bills)
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(sum(b.amount-COALESCE(paid.total,0)),0)
		FROM bills b LEFT JOIN (SELECT bill_id,sum(amount) AS total FROM payment_allocations GROUP BY bill_id) paid ON paid.bill_id=b.id
		WHERE b.waived=false`).Scan(&outstanding)
	_ = a.db.QueryRow(r.Context(), "SELECT COALESCE(sum(amount),0) FROM payments").Scan(&collected)
	jsonOut(w, 200, map[string]any{"residents": residents, "bills": bills, "outstanding": outstanding, "payments_total": collected})
}
func (a *App) residents(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), "SELECT id,house_code,head_name,address,qr_token,phone,savings_class,dues_required FROM residents WHERE active=true ORDER BY house_code")
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []Resident{}
	for rows.Next() {
		var x Resident
		if err := rows.Scan(&x.ID, &x.HouseCode, &x.HeadName, &x.Address, &x.QRToken, &x.Phone, &x.SavingsClass, &x.DuesRequired); err != nil {
			fail(w, err)
			return
		}
		out = append(out, x)
	}
	jsonOut(w, 200, out)
}
func (a *App) createResident(w http.ResponseWriter, r *http.Request) {
	var x Resident
	if jsonIn(r, &x) != nil || strings.TrimSpace(x.HouseCode) == "" || strings.TrimSpace(x.HeadName) == "" {
		jsonOut(w, 400, map[string]string{"error": "house_code dan head_name wajib diisi"})
		return
	}
	if x.SavingsClass == 0 {
		x.SavingsClass = 3
	}
	if x.SavingsClass < 1 || x.SavingsClass > 4 {
		jsonOut(w, 400, map[string]string{"error": "savings_class harus 1 sampai 4"})
		return
	}
	if x.QRToken == "" {
		x.QRToken = "WDR-" + strings.ToUpper(strings.ReplaceAll(x.HouseCode, " ", "-"))
	}
	if !x.DuesRequired { /* explicitly exempt */
	} else {
		x.DuesRequired = true
	}
	err := a.db.QueryRow(r.Context(), "INSERT INTO residents(house_code,head_name,address,qr_token,phone,savings_class,dues_required) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id", x.HouseCode, x.HeadName, x.Address, x.QRToken, strings.TrimSpace(x.Phone), x.SavingsClass, x.DuesRequired).Scan(&x.ID)
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 201, x)
}
func (a *App) updateResidentContact(w http.ResponseWriter, r *http.Request) {
	residentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id warga tidak valid"})
		return
	}
	var input struct {
		Phone        string `json:"phone"`
		SavingsClass int    `json:"savings_class"`
		DuesRequired bool   `json:"dues_required"`
	}
	if jsonIn(r, &input) != nil || input.SavingsClass < 1 || input.SavingsClass > 4 {
		jsonOut(w, 400, map[string]string{"error": "savings_class harus 1 sampai 4"})
		return
	}
	result, err := a.db.Exec(r.Context(), "UPDATE residents SET phone=$1,savings_class=$2 WHERE id=$3 AND active=true", strings.TrimSpace(input.Phone), input.SavingsClass, residentID)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "warga tidak ditemukan"})
		return
	}
	jsonOut(w, 200, input)
}
func (a *App) updateResident(w http.ResponseWriter, r *http.Request) {
	residentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id warga tidak valid"})
		return
	}
	var x Resident
	if jsonIn(r, &x) != nil || strings.TrimSpace(x.HouseCode) == "" || strings.TrimSpace(x.HeadName) == "" {
		jsonOut(w, 400, map[string]string{"error": "house_code dan head_name wajib diisi"})
		return
	}
	if x.SavingsClass == 0 {
		x.SavingsClass = 3
	}
	if x.SavingsClass < 1 || x.SavingsClass > 4 {
		jsonOut(w, 400, map[string]string{"error": "savings_class harus 1 sampai 4"})
		return
	}
	if strings.TrimSpace(x.QRToken) == "" {
		x.QRToken = "WDR-" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(x.HouseCode), " ", "-"))
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), "UPDATE residents SET house_code=$1,head_name=$2,address=$3,qr_token=$4,phone=$5,savings_class=$6,dues_required=$7 WHERE id=$8 AND active=true",
		strings.TrimSpace(x.HouseCode), strings.TrimSpace(x.HeadName), x.Address, strings.TrimSpace(x.QRToken), strings.TrimSpace(x.Phone), x.SavingsClass, x.DuesRequired, residentID)
	if err != nil {
		if isUniqueViolation(err) {
			jsonOut(w, 409, map[string]string{"error": "Kode rumah atau token QR sudah dipakai warga lain"})
			return
		}
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "warga tidak ditemukan"})
		return
	}
	if !x.DuesRequired {
		if _, err = tx.Exec(r.Context(), `UPDATE bills SET waived=true WHERE resident_id=$1 AND description=$2`, residentID, meetingBillDescription); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	x.ID = residentID
	jsonOut(w, 200, x)
}
func (a *App) importResidents(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonOut(w, 400, map[string]string{"error": "unggahan CSV tidak valid atau melebihi 10 MB"})
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "file CSV wajib diunggah"})
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "header CSV tidak ditemukan"})
		return
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if _, exists := columns[name]; exists {
			jsonOut(w, 400, map[string]string{"error": "header CSV duplikat: " + name})
			return
		}
		columns[name] = index
	}
	if _, ok := columns["house_code"]; !ok {
		jsonOut(w, 400, map[string]string{"error": "kolom house_code wajib ada"})
		return
	}
	if _, ok := columns["head_name"]; !ok {
		jsonOut(w, 400, map[string]string{"error": "kolom head_name wajib ada"})
		return
	}

	records, err := reader.ReadAll()
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "format CSV tidak valid: " + err.Error()})
		return
	}
	get := func(record []string, name string) string {
		index, ok := columns[name]
		if !ok || index >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[index])
	}
	residents := make([]Resident, 0, len(records))
	var rowErrors []string
	for index, record := range records {
		savingsClass := 3
		if rawClass := get(record, "savings_class"); rawClass != "" {
			parsedClass, parseErr := strconv.Atoi(rawClass)
			if parseErr != nil || parsedClass < 1 || parsedClass > 4 {
				rowErrors = append(rowErrors, fmt.Sprintf("baris %d: savings_class harus 1 sampai 4", index+2))
				continue
			}
			savingsClass = parsedClass
		}
		resident := Resident{
			HouseCode:    get(record, "house_code"),
			HeadName:     get(record, "head_name"),
			Address:      get(record, "address"),
			QRToken:      get(record, "qr_token"),
			Phone:        get(record, "phone"),
			SavingsClass: savingsClass,
		}
		if resident.HouseCode == "" && resident.HeadName == "" && resident.Address == "" && resident.QRToken == "" {
			continue
		}
		if resident.HouseCode == "" || resident.HeadName == "" {
			rowErrors = append(rowErrors, fmt.Sprintf("baris %d: house_code dan head_name wajib diisi", index+2))
			continue
		}
		if resident.QRToken == "" {
			resident.QRToken = "WDR-" + strings.ToUpper(strings.ReplaceAll(resident.HouseCode, " ", "-"))
		}
		residents = append(residents, resident)
	}
	if len(rowErrors) > 0 {
		jsonOut(w, 400, map[string]string{"error": strings.Join(rowErrors, "; ")})
		return
	}
	if len(residents) == 0 {
		jsonOut(w, 400, map[string]string{"error": "CSV tidak berisi data warga"})
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var imported, skipped int
	for _, resident := range residents {
		result, err := tx.Exec(r.Context(), `INSERT INTO residents(house_code,head_name,address,qr_token,phone,savings_class)
			VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, resident.HouseCode, resident.HeadName, resident.Address, resident.QRToken, resident.Phone, resident.SavingsClass)
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			skipped++
		} else {
			imported++
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]int{"imported": imported, "skipped": skipped})
}
func (a *App) periods(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT p.id,p.start_date::text,p.meeting_date::text,p.meeting_time::text,p.status,p.note,
		COALESCE(f.payment_count,0),COALESCE(f.collected_amount,0),f.period_id IS NOT NULL,
		COALESCE(b.total,0),COALESCE(b.unpaid,0),COALESCE(b.unpaid_amount,0)
		FROM periods p
		LEFT JOIN meeting_finances f ON f.period_id=p.id
		LEFT JOIN (
			SELECT bills.period_id,
				count(*) AS total,
				count(*) FILTER (WHERE bills.amount > COALESCE(paid.total,0)) AS unpaid,
				COALESCE(sum(GREATEST(bills.amount-COALESCE(paid.total,0),0))
					FILTER (WHERE bills.amount > COALESCE(paid.total,0)),0)::bigint AS unpaid_amount
			FROM bills
			LEFT JOIN (SELECT bill_id,sum(amount) AS total FROM payment_allocations GROUP BY bill_id) paid
				ON paid.bill_id=bills.id
			WHERE bills.waived=false
			GROUP BY bills.period_id
		) b ON b.period_id=p.id
		ORDER BY p.meeting_date DESC`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []Period{}
	for rows.Next() {
		var p Period
		var paymentCount int
		var totalCollected int64
		var hasFinance bool
		if err := rows.Scan(&p.ID, &p.StartDate, &p.MeetingDate, &p.MeetingTime, &p.Status, &p.Note,
			&paymentCount, &totalCollected, &hasFinance,
			&p.BillsCount, &p.BillsUnpaid, &p.BillsUnpaidAmount); err != nil {
			fail(w, err)
			return
		}
		if hasFinance {
			p.Finance = calculateMeetingFinance(p.ID, paymentCount, totalCollected)
		}
		out = append(out, p)
	}
	jsonOut(w, 200, out)
}

// createPeriodBills membuat tagihan iuran kumpulan (Rp20.000) untuk seluruh
// warga aktif. Aman dipanggil berulang: kombinasi warga+periode+deskripsi unik,
// sehingga tagihan yang sudah ada dilewati.
func createPeriodBills(ctx context.Context, tx pgx.Tx, periodID int64) (int64, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO bills(resident_id,period_id,description,amount)
		SELECT r.id,$1,$2,$3 FROM residents r WHERE r.active=true AND r.dues_required=true
		ON CONFLICT(resident_id,period_id,description) DO NOTHING`,
		periodID, meetingBillDescription, meetingBillAmount)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (a *App) createPeriod(w http.ResponseWriter, r *http.Request) {
	var p Period
	if jsonIn(r, &p) != nil || p.StartDate == "" || p.MeetingDate == "" {
		jsonOut(w, 400, map[string]string{"error": "start_date dan meeting_date wajib diisi (YYYY-MM-DD)"})
		return
	}
	p.Status = strings.ToUpper(p.Status)
	if p.Status == "" {
		p.Status = "TERJADWAL"
	}
	if p.Status != "TERJADWAL" && p.Status != "TERLAKSANA" && p.Status != "LIBUR" {
		jsonOut(w, 400, map[string]string{"error": "status harus TERJADWAL, TERLAKSANA, atau LIBUR"})
		return
	}
	if p.MeetingTime == "" {
		p.MeetingTime = "19:30"
	}
	meetingTime, err := time.Parse("15:04", p.MeetingTime)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "jam kumpulan harus berformat HH:MM"})
		return
	}
	p.MeetingTime = meetingTime.Format("15:04")
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if err = tx.QueryRow(r.Context(), "INSERT INTO periods(start_date,meeting_date,meeting_time,status,note) VALUES($1,$2,$3,$4,$5) RETURNING id", p.StartDate, p.MeetingDate, p.MeetingTime, p.Status, p.Note).Scan(&p.ID); err != nil {
		fail(w, err)
		return
	}
	if p.Status == "TERLAKSANA" {
		if _, err = createPeriodBills(r.Context(), tx, p.ID); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 201, p)
}
func (a *App) updatePeriod(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id periode tidak valid"})
		return
	}
	var p Period
	if jsonIn(r, &p) != nil || p.StartDate == "" || p.MeetingDate == "" {
		jsonOut(w, 400, map[string]string{"error": "tanggal awal dan tanggal kumpulan wajib diisi"})
		return
	}
	if _, err := time.Parse("2006-01-02", p.StartDate); err != nil {
		jsonOut(w, 400, map[string]string{"error": "tanggal awal harus berformat YYYY-MM-DD"})
		return
	}
	if _, err := time.Parse("2006-01-02", p.MeetingDate); err != nil {
		jsonOut(w, 400, map[string]string{"error": "tanggal kumpulan harus berformat YYYY-MM-DD"})
		return
	}
	meetingTime, err := time.Parse("15:04", p.MeetingTime)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "jam kumpulan harus berformat HH:MM"})
		return
	}
	p.Status = strings.ToUpper(p.Status)
	if p.Status != "TERJADWAL" && p.Status != "LIBUR" && p.Status != "TERLAKSANA" {
		jsonOut(w, 400, map[string]string{"error": "status harus TERJADWAL, LIBUR, atau TERLAKSANA"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if err = tx.QueryRow(r.Context(), `UPDATE periods SET start_date=$1,meeting_date=$2,meeting_time=$3,status=$4,note=$5
		WHERE id=$6 RETURNING id`, p.StartDate, p.MeetingDate, meetingTime.Format("15:04"), p.Status, p.Note, periodID).Scan(&p.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "periode tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	// Kumpulan yang ditandai terlaksana langsung memunculkan tagihan warganya.
	if p.Status == "TERLAKSANA" {
		if _, err = createPeriodBills(r.Context(), tx, p.ID); err != nil {
			fail(w, err)
			return
		}
	} else {
		// Kumpulan batal/LIBUR tidak boleh menagih. Hapus tagihan yang belum
		// punya pembayaran; tagihan yang sudah dibayar dibiarkan agar riwayat
		// keuangan tetap utuh.
		if _, err = tx.Exec(r.Context(), `DELETE FROM bills b WHERE b.period_id=$1
			AND NOT EXISTS (SELECT 1 FROM payment_allocations pa WHERE pa.bill_id=b.id)`, p.ID); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	p.MeetingTime = meetingTime.Format("15:04")
	jsonOut(w, 200, p)
}
func (a *App) deletePeriod(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id periode tidak valid"})
		return
	}
	var status string
	if err := a.db.QueryRow(r.Context(), "SELECT status FROM periods WHERE id=$1", periodID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "periode tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	if status != "TERJADWAL" {
		jsonOut(w, 409, map[string]string{"error": "hanya jadwal TERJADWAL yang dapat dihapus"})
		return
	}
	var hasRecords bool
	err = a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM bills WHERE period_id=$1)
		OR EXISTS(SELECT 1 FROM meeting_finances WHERE period_id=$1)`, periodID).Scan(&hasRecords)
	if err != nil {
		fail(w, err)
		return
	}
	if hasRecords {
		jsonOut(w, 409, map[string]string{"error": "periode memiliki tagihan atau rekap kas sehingga tidak dapat dihapus"})
		return
	}
	if _, err := a.db.Exec(r.Context(), "DELETE FROM periods WHERE id=$1", periodID); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"deleted": true, "id": periodID})
}
func (a *App) generatePeriodSeries(w http.ResponseWriter, r *http.Request) {
	var input PeriodSeriesInput
	if jsonIn(r, &input) != nil {
		jsonOut(w, 400, map[string]string{"error": "data jadwal tidak valid"})
		return
	}
	startDate, err := time.Parse("2006-01-02", input.StartDate)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "tanggal awal harus berformat YYYY-MM-DD"})
		return
	}
	throughDate, err := time.Parse("2006-01-02", input.ThroughDate)
	if err != nil || throughDate.Before(startDate) {
		jsonOut(w, 400, map[string]string{"error": "tanggal akhir harus valid dan tidak boleh sebelum tanggal awal"})
		return
	}
	meetingTime, err := time.Parse("15:04", input.MeetingTime)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "jam kumpulan harus berformat HH:MM"})
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var created, skipped int
	var firstDate, lastDate string
	for date := startDate; !date.After(throughDate); date = date.AddDate(0, 0, 35) {
		meetingDate := date.Format("2006-01-02")
		if firstDate == "" {
			firstDate = meetingDate
		}
		lastDate = meetingDate
		result, err := tx.Exec(r.Context(), `INSERT INTO periods(start_date,meeting_date,meeting_time,status,note)
			VALUES($1,$1,$2,'TERJADWAL','Jadwal Ahad Pon otomatis') ON CONFLICT(meeting_date) DO NOTHING`, meetingDate, meetingTime.Format("15:04:05"))
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			skipped++
		} else {
			created++
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{
		"created": created, "skipped": skipped, "first_date": firstDate,
		"last_date": lastDate, "meeting_time": meetingTime.Format("15:04"),
	})
}
func (a *App) periodInvitation(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id periode tidak valid"})
		return
	}
	var period Period
	var isGenerated bool
	err = a.db.QueryRow(r.Context(), `SELECT id,start_date::text,meeting_date::text,meeting_time::text,status,note,
		note='Jadwal Ahad Pon otomatis' FROM periods WHERE id=$1`, periodID).Scan(&period.ID, &period.StartDate, &period.MeetingDate, &period.MeetingTime, &period.Status, &period.Note, &isGenerated)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "jadwal kumpulan tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	var stageNumber int
	if isGenerated {
		meetingDate, parseErr := time.Parse("2006-01-02", period.MeetingDate)
		if parseErr != nil {
			fail(w, parseErr)
			return
		}
		stageNumber, err = savingsStageForMeeting(r.Context(), a.db, meetingDate)
		if err != nil {
			fail(w, err)
			return
		}
	}
	rows, err := a.db.Query(r.Context(), `SELECT r.id,r.house_code,r.head_name,r.phone,r.savings_class,
		COALESCE(cash.due,0)::bigint,
		CASE WHEN $1=0 THEN 0 ELSE GREATEST(COALESCE(savings_bill.amount,0)-COALESCE(savings.paid,0),0)::bigint END
		FROM residents r
		LEFT JOIN (
			SELECT b.resident_id,SUM(b.amount-COALESCE(paid.total,0)) AS due
			FROM bills b LEFT JOIN (SELECT bill_id,SUM(amount) AS total FROM payment_allocations GROUP BY bill_id) paid ON paid.bill_id=b.id
			WHERE b.waived=false
			GROUP BY b.resident_id
		) cash ON cash.resident_id=r.id
		LEFT JOIN (
			SELECT resident_id,SUM(amount) AS paid FROM tabungan_transactions
			WHERE stage_number=$1 AND type='SETOR' GROUP BY resident_id
		) savings ON savings.resident_id=r.id
		LEFT JOIN savings_stage_bills savings_bill ON savings_bill.resident_id=r.id AND savings_bill.stage_number=$1
		WHERE r.active=true AND r.dues_required=true ORDER BY r.house_code`, stageNumber)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type recipient struct {
		ID           int64  `json:"id"`
		HouseCode    string `json:"house_code"`
		HeadName     string `json:"head_name"`
		Phone        string `json:"phone"`
		SavingsClass int    `json:"savings_class"`
		CashDue      int64  `json:"cash_due"`
		SavingsDue   int64  `json:"savings_due"`
	}
	recipients := []recipient{}
	for rows.Next() {
		var x recipient
		if err := rows.Scan(&x.ID, &x.HouseCode, &x.HeadName, &x.Phone, &x.SavingsClass, &x.CashDue, &x.SavingsDue); err != nil {
			fail(w, err)
			return
		}
		recipients = append(recipients, x)
	}
	reminderDate, _ := time.Parse("2006-01-02", period.MeetingDate)
	jsonOut(w, 200, map[string]any{
		"period": period, "reminder_date": reminderDate.AddDate(0, 0, -5).Format("2006-01-02"),
		"savings_stage": stageNumber, "recipients": recipients,
	})
}
func (a *App) meetingFinancePreview(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id periode tidak valid"})
		return
	}
	finance, status, err := loadMeetingFinance(r.Context(), a.db, periodID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "periode kumpulan tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	var prepared bool
	var savedCount int
	var savedTotal int64
	if err = a.db.QueryRow(r.Context(), "SELECT payment_count,collected_amount FROM meeting_finances WHERE period_id=$1", periodID).Scan(&savedCount, &savedTotal); err == nil {
		prepared = true
		finance = calculateMeetingFinance(periodID, savedCount, savedTotal)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, err)
		return
	}
	allocations, err := a.meetingFinanceReceipts(r.Context(), periodID, finance)
	if err != nil {
		fail(w, err)
		return
	}
	principal, _ := r.Context().Value(authContextKey{}).(AuthPrincipal)
	for i := range allocations {
		allocations[i].CanConfirm = canConfirmFinanceField(r.Context(), a.db, principal, allocations[i].Key)
	}
	jsonOut(w, 200, map[string]any{"finance": finance, "status": status, "prepared": prepared, "required_amount": 640000, "shortfall": max(640000-finance.TotalCollected, 0), "allocations": allocations})
}

func (a *App) saveMeetingFinance(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id periode tidak valid"})
		return
	}
	finance, status, err := loadMeetingFinance(r.Context(), a.db, periodID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "periode kumpulan tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	if status != "TERLAKSANA" {
		jsonOut(w, 409, map[string]string{"error": "rekap kas hanya dapat disimpan untuk kumpulan berstatus TERLAKSANA"})
		return
	}
	if finance.TotalCollected < 640000 {
		jsonOut(w, 400, map[string]any{"error": "pembayaran yang masuk pada tanggal kumpulan belum cukup untuk memenuhi alokasi Rp640.000", "total_collected": finance.TotalCollected, "shortfall": 640000 - finance.TotalCollected})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var confirmed int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM meeting_finance_receipts WHERE period_id=$1 AND confirmed_at IS NOT NULL`, periodID).Scan(&confirmed); err != nil {
		fail(w, err)
		return
	}
	var savedTotal int64
	_ = tx.QueryRow(r.Context(), `SELECT collected_amount FROM meeting_finances WHERE period_id=$1`, periodID).Scan(&savedTotal)
	if confirmed > 0 && savedTotal != finance.TotalCollected {
		jsonOut(w, 409, map[string]string{"error": "alokasi tidak dapat diubah setelah ada bidang yang mengonfirmasi penerimaan"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO meeting_finances(period_id,payment_count,collected_amount) VALUES($1,$2,$3)
		ON CONFLICT(period_id) DO UPDATE SET payment_count=EXCLUDED.payment_count,collected_amount=EXCLUDED.collected_amount,updated_at=now()`, periodID, finance.PaymentCount, finance.TotalCollected)
	if err != nil {
		fail(w, err)
		return
	}
	if confirmed == 0 {
		if _, err = tx.Exec(r.Context(), "DELETE FROM meeting_finance_receipts WHERE period_id=$1", periodID); err != nil {
			fail(w, err)
			return
		}
		for _, allocation := range financeAllocations(finance) {
			if _, err = tx.Exec(r.Context(), `INSERT INTO meeting_finance_receipts(period_id,allocation_key,label,responsible_field,amount) VALUES($1,$2,$3,$4,$5)`, periodID, allocation.Key, allocation.Label, allocation.Field, allocation.Amount); err != nil {
				fail(w, err)
				return
			}
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, finance)
}

type FinanceReceipt struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Field       string `json:"field"`
	Amount      int64  `json:"amount"`
	Confirmed   bool   `json:"confirmed"`
	ConfirmedBy string `json:"confirmed_by"`
	ConfirmedAt string `json:"confirmed_at"`
	Note        string `json:"note"`
	CanConfirm  bool   `json:"can_confirm"`
}

func financeAllocations(finance *MeetingFinance) []FinanceReceipt {
	return []FinanceReceipt{
		{Key: "consumption", Label: "Konsumsi", Field: "Konsumsi", Amount: finance.HostConsumption},
		{Key: "social", Label: "Dana Sosial", Field: "Sosial", Amount: finance.Sosial},
		{Key: "street_lighting", Label: "Penerangan Jalan", Field: "Penerangan Jalan", Amount: finance.PeneranganJalan},
		{Key: "baitul_mal", Label: "Baitul Mal", Field: "Baitul Mal", Amount: finance.Ziswaf},
		{Key: "mosque", Label: "Kemasjidan", Field: "Kemasjidan", Amount: finance.Kemasjidan},
		{Key: "general_treasury", Label: "Kas Bendahara Umum", Field: "Bendahara Umum", Amount: finance.KasRT},
	}
}

func (a *App) meetingFinanceReceipts(ctx context.Context, periodID int64, finance *MeetingFinance) ([]FinanceReceipt, error) {
	rows, err := a.db.Query(ctx, `SELECT allocation_key,label,responsible_field,amount,confirmed_by IS NOT NULL,COALESCE(u.full_name,''),COALESCE(confirmed_at::text,''),note
		FROM meeting_finance_receipts r LEFT JOIN app_users u ON u.id=r.confirmed_by WHERE period_id=$1 ORDER BY allocation_key`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinanceReceipt{}
	for rows.Next() {
		var x FinanceReceipt
		if err = rows.Scan(&x.Key, &x.Label, &x.Field, &x.Amount, &x.Confirmed, &x.ConfirmedBy, &x.ConfirmedAt, &x.Note); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	if len(out) == 0 {
		out = financeAllocations(finance)
	}
	return out, rows.Err()
}

func canConfirmFinanceField(ctx context.Context, db *pgxpool.Pool, principal AuthPrincipal, key string) bool {
	if !principal.Permissions["payments.manage"] {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(principal.Position), "Administrator") {
		return true
	}
	var field string
	if principal.PositionID > 0 {
		_ = db.QueryRow(ctx, `SELECT COALESCE(f.name,'') FROM positions p LEFT JOIN organizational_fields f ON f.id=p.field_id WHERE p.id=$1`, principal.PositionID).Scan(&field)
	}
	return financeFieldMatches(key, field)
}

func financeFieldMatches(key, field string) bool {
	f := strings.ToLower(strings.TrimSpace(field))
	if f == "" {
		return false
	}
	aliases := map[string][]string{"consumption": {"konsumsi"}, "social": {"sosial"}, "street_lighting": {"penerangan", "jalan"}, "baitul_mal": {"baitul", "ziswaf"}, "mosque": {"kemasjidan", "masjid"}, "general_treasury": {"bendahara", "kas umum"}, "road_savings": {"jalan", "infrastruktur", "bendahara"}}
	for _, alias := range aliases[key] {
		if strings.Contains(f, alias) {
			return true
		}
	}
	return false
}

func parseReceiptDate(w http.ResponseWriter, r *http.Request) (string, bool) {
	date := r.PathValue("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		jsonOut(w, 400, map[string]string{"error": "tanggal penerimaan harus berformat YYYY-MM-DD"})
		return "", false
	}
	return date, true
}

func (a *App) financeReceiptDates(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT DISTINCT timezone('Asia/Jakarta',received_at)::date::text FROM payments ORDER BY 1 DESC`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	dates := []string{}
	for rows.Next() {
		var date string
		if err = rows.Scan(&date); err != nil {
			fail(w, err)
			return
		}
		dates = append(dates, date)
	}
	jsonOut(w, 200, dates)
}

func (a *App) dailyFinancePreview(w http.ResponseWriter, r *http.Request) {
	date, ok := parseReceiptDate(w, r)
	if !ok {
		return
	}
	finance, count, err := a.loadDailyFinance(r.Context(), date)
	if err != nil {
		fail(w, err)
		return
	}
	currentTotal, currentCount := finance.TotalCollected, count
	var prepared bool
	var savedCount int
	var savedTotal int64
	if err = a.db.QueryRow(r.Context(), "SELECT payment_count,collected_amount FROM daily_finance_snapshots WHERE receipt_date=$1", date).Scan(&savedCount, &savedTotal); err == nil {
		prepared = true
		finance = calculateMeetingFinance(0, savedCount, savedTotal)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT allocation_key,label,responsible_field,amount,confirmed_by IS NOT NULL,COALESCE(u.full_name,''),COALESCE(confirmed_at::text,''),note
		FROM daily_finance_allocations d LEFT JOIN app_users u ON u.id=d.confirmed_by WHERE receipt_date=$1 ORDER BY allocation_key`, date)
	if err != nil {
		fail(w, err)
		return
	}
	allocations := []FinanceReceipt{}
	for rows.Next() {
		var x FinanceReceipt
		if err = rows.Scan(&x.Key, &x.Label, &x.Field, &x.Amount, &x.Confirmed, &x.ConfirmedBy, &x.ConfirmedAt, &x.Note); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		allocations = append(allocations, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()
	if !prepared {
		allocations = financeAllocations(finance)
	}
	principal, _ := r.Context().Value(authContextKey{}).(AuthPrincipal)
	for i := range allocations {
		allocations[i].CanConfirm = canConfirmFinanceField(r.Context(), a.db, principal, allocations[i].Key)
	}
	jsonOut(w, 200, map[string]any{"date": date, "finance": finance, "current_total": currentTotal, "current_count": currentCount, "prepared": prepared, "required_amount": allocationTotal, "shortfall": max(allocationTotal-finance.TotalCollected, 0), "allocations": allocations})
}

func (a *App) loadDailyFinance(ctx context.Context, date string) (*MeetingFinance, int, error) {
	var count int
	var total int64
	err := a.db.QueryRow(ctx, `SELECT count(*)::int,COALESCE(sum(amount),0)::bigint FROM payments WHERE timezone('Asia/Jakarta',received_at)::date=$1`, date).Scan(&count, &total)
	if err != nil {
		return nil, 0, err
	}
	return calculateMeetingFinance(0, count, total), count, nil
}

func (a *App) saveDailyFinance(w http.ResponseWriter, r *http.Request) {
	date, ok := parseReceiptDate(w, r)
	if !ok {
		return
	}
	finance, count, err := a.loadDailyFinance(r.Context(), date)
	if err != nil {
		fail(w, err)
		return
	}
	if finance.TotalCollected < allocationTotal {
		jsonOut(w, 400, map[string]any{"error": "penerimaan pada tanggal tersebut belum cukup untuk memenuhi alokasi Rp640.000", "total_collected": finance.TotalCollected, "shortfall": allocationTotal - finance.TotalCollected})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var confirmed int
	_ = tx.QueryRow(r.Context(), `SELECT count(*) FROM daily_finance_allocations WHERE receipt_date=$1 AND confirmed_at IS NOT NULL`, date).Scan(&confirmed)
	var savedTotal int64
	_ = tx.QueryRow(r.Context(), `SELECT collected_amount FROM daily_finance_snapshots WHERE receipt_date=$1`, date).Scan(&savedTotal)
	if confirmed > 0 && savedTotal != finance.TotalCollected {
		jsonOut(w, 409, map[string]string{"error": "alokasi tidak dapat diubah setelah pengurus mengonfirmasi penerimaan"})
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO daily_finance_snapshots(receipt_date,payment_count,collected_amount) VALUES($1,$2,$3) ON CONFLICT(receipt_date) DO UPDATE SET payment_count=EXCLUDED.payment_count,collected_amount=EXCLUDED.collected_amount,updated_at=now()`, date, count, finance.TotalCollected)
	if err != nil {
		fail(w, err)
		return
	}
	if confirmed == 0 {
		if _, err = tx.Exec(r.Context(), "DELETE FROM daily_finance_allocations WHERE receipt_date=$1", date); err != nil {
			fail(w, err)
			return
		}
		for _, x := range financeAllocations(finance) {
			if _, err = tx.Exec(r.Context(), `INSERT INTO daily_finance_allocations(receipt_date,allocation_key,label,responsible_field,amount) VALUES($1,$2,$3,$4,$5)`, date, x.Key, x.Label, x.Field, x.Amount); err != nil {
				fail(w, err)
				return
			}
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"date": date, "finance": finance, "prepared": true})
}

func (a *App) confirmDailyFinanceReceipt(w http.ResponseWriter, r *http.Request) {
	date, ok := parseReceiptDate(w, r)
	if !ok {
		return
	}
	var input struct {
		Key  string `json:"key"`
		Note string `json:"note"`
	}
	if jsonIn(r, &input) != nil {
		jsonOut(w, 400, map[string]string{"error": "key alokasi wajib diisi"})
		return
	}
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok {
		jsonOut(w, 401, map[string]string{"error": "login diperlukan"})
		return
	}
	if !canConfirmFinanceField(r.Context(), a.db, principal, input.Key) {
		jsonOut(w, 403, map[string]string{"error": "hanya pengurus bidang penerima yang dapat mengonfirmasi alokasi ini"})
		return
	}
	result, err := a.db.Exec(r.Context(), `UPDATE daily_finance_allocations SET confirmed_by=$1,confirmed_at=now(),note=$2 WHERE receipt_date=$3 AND allocation_key=$4`, principal.UserID, strings.TrimSpace(input.Note), date, input.Key)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, 409, map[string]string{"error": "alokasi belum dibuat; proses rekap tanggal penerimaan terlebih dahulu"})
		return
	}
	jsonOut(w, 200, map[string]any{"date": date, "key": input.Key, "confirmed": true, "confirmed_by": principal.FullName})
}

type CashbookField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Income    int64  `json:"income"`
	Pending   int64  `json:"pending"`
	Expense   int64  `json:"expense"`
	Balance   int64  `json:"balance"`
	CanRecord bool   `json:"can_record"`
}
type CashbookEntry struct {
	ID         int64  `json:"id"`
	Date       string `json:"date"`
	FieldKey   string `json:"field_key"`
	FieldLabel string `json:"field_label"`
	Kind       string `json:"kind"`
	Amount     int64  `json:"amount"`
	Note       string `json:"note"`
	Actor      string `json:"actor"`
	CreatedAt  string `json:"created_at"`
}

type CashbookInput struct {
	FieldKey  string `json:"field_key"`
	Direction string `json:"direction"`
	Date      string `json:"date"`
	Amount    int64  `json:"amount"`
	Note      string `json:"note"`
}

func cashbookFieldBelongsToPosition(ctx context.Context, db *pgxpool.Pool, principal AuthPrincipal, key string) bool {
	if strings.EqualFold(strings.TrimSpace(principal.Position), "Administrator") {
		return true
	}
	if principal.FieldID == 0 || principal.PositionID == 0 {
		return false
	}
	if key == fmt.Sprintf("field_%d", principal.FieldID) {
		return true
	}
	var name string
	if err := db.QueryRow(ctx, "SELECT name FROM organizational_fields WHERE id=$1", principal.FieldID).Scan(&name); err != nil {
		return false
	}
	if key == "road_savings" {
		return financeFieldMatches(key, name)
	}
	for _, allocation := range financeAllocations(calculateMeetingFinance(0, 0, allocationTotal)) {
		if allocation.Key == key && financeFieldMatches(key, name) {
			return true
		}
	}
	return false
}

func (a *App) cashbook(w http.ResponseWriter, r *http.Request) {
	allocations := financeAllocations(calculateMeetingFinance(0, 0, allocationTotal))
	type orgField struct {
		id   int64
		name string
	}
	fieldRows, err := a.db.Query(r.Context(), "SELECT id,name FROM organizational_fields ORDER BY name")
	if err != nil {
		fail(w, err)
		return
	}
	orgFields := []orgField{}
	for fieldRows.Next() {
		var f orgField
		if err = fieldRows.Scan(&f.id, &f.name); err != nil {
			fieldRows.Close()
			fail(w, err)
			return
		}
		orgFields = append(orgFields, f)
	}
	fieldRows.Close()
	principal, _ := r.Context().Value(authContextKey{}).(AuthPrincipal)
	fields := []CashbookField{}
	used := map[int64]bool{}
	appendSummary := func(key, label string, automaticKey string) {
		var income, pending, expense, manualIncome int64
		_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(sum(amount) FILTER(WHERE confirmed_at IS NOT NULL),0)::bigint,COALESCE(sum(amount) FILTER(WHERE confirmed_at IS NULL),0)::bigint FROM daily_finance_allocations WHERE allocation_key=$1`, automaticKey).Scan(&income, &pending)
		_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(sum(amount) FILTER(WHERE direction='MASUK'),0)::bigint,COALESCE(sum(amount) FILTER(WHERE direction='KELUAR'),0)::bigint FROM cashbook_transactions WHERE field_key=$1`, key).Scan(&manualIncome, &expense)
		income += manualIncome
		fields = append(fields, CashbookField{Key: key, Label: label, Income: income, Pending: pending, Expense: expense, Balance: income - expense, CanRecord: principal.Permissions["cashbook.manage"] && cashbookFieldBelongsToPosition(r.Context(), a.db, principal, key)})
	}
	for _, allocation := range allocations {
		key := allocation.Key
		label := allocation.Label
		for _, f := range orgFields {
			if !used[f.id] && financeFieldMatches(key, f.name) {
				key = allocation.Key
				label = f.name
				used[f.id] = true
				break
			}
		}
		appendSummary(key, label, allocation.Key)
	}
	for _, f := range orgFields {
		if used[f.id] {
			continue
		}
		key := fmt.Sprintf("field_%d", f.id)
		appendSummary(key, f.name, "")
	}
	var savingsIncome, savingsExpense int64
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions WHERE type='SETOR'`).Scan(&savingsIncome)
	_ = a.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM cashbook_transactions WHERE field_key='road_savings' AND direction='KELUAR'`).Scan(&savingsExpense)
	canRecordSavings := principal.Permissions["cashbook.manage"] && cashbookFieldBelongsToPosition(r.Context(), a.db, principal, "road_savings")
	fields = append(fields, CashbookField{Key: "road_savings", Label: "Tabungan Jalan", Income: savingsIncome, Expense: savingsExpense, Balance: savingsIncome - savingsExpense, CanRecord: canRecordSavings})
	rows, err := a.db.Query(r.Context(), `SELECT id,transaction_date::text,field_key,field_label,kind,amount,note,actor,created_at::text FROM (
		SELECT 0::bigint AS id,d.receipt_date AS transaction_date,d.allocation_key AS field_key,d.label AS field_label,'MASUK'::text AS kind,
			d.amount,d.note,COALESCE(u.full_name,'') AS actor,d.confirmed_at AS created_at
		FROM daily_finance_allocations d LEFT JOIN app_users u ON u.id=d.confirmed_by WHERE d.confirmed_at IS NOT NULL
		UNION ALL
		SELECT c.id,c.transaction_date,c.field_key,COALESCE(NULLIF(c.field_label,''),c.field_key),c.direction,
			c.amount,c.note,COALESCE(u.full_name,''),c.created_at FROM cashbook_transactions c LEFT JOIN app_users u ON u.id=c.recorded_by
		UNION ALL
		SELECT t.id,COALESCE(p.meeting_date,t.created_at::date),'road_savings','Tabungan Jalan','MASUK',t.amount,
			concat('Setoran ',t.member_name),COALESCE(u.full_name,''),t.created_at
		FROM tabungan_transactions t LEFT JOIN periods p ON p.id=t.period_id LEFT JOIN app_users u ON u.id=t.recorded_by WHERE t.type='SETOR'
	) ledger ORDER BY transaction_date DESC,created_at DESC`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	entries := []CashbookEntry{}
	for rows.Next() {
		var e CashbookEntry
		if err = rows.Scan(&e.ID, &e.Date, &e.FieldKey, &e.FieldLabel, &e.Kind, &e.Amount, &e.Note, &e.Actor, &e.CreatedAt); err != nil {
			fail(w, err)
			return
		}
		entries = append(entries, e)
	}
	if !strings.EqualFold(strings.TrimSpace(principal.Position), "Administrator") {
		visible := []CashbookField{}
		allowed := map[string]bool{}
		for _, field := range fields {
			if cashbookFieldBelongsToPosition(r.Context(), a.db, principal, field.Key) {
				visible = append(visible, field)
				allowed[field.Key] = true
			}
		}
		fields = visible
		filtered := []CashbookEntry{}
		for _, entry := range entries {
			if allowed[entry.FieldKey] {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	jsonOut(w, 200, map[string]any{"fields": fields, "entries": entries})
}

func (a *App) cashbookExpense(w http.ResponseWriter, r *http.Request) {
	var input CashbookInput
	if jsonIn(r, &input) != nil || input.Amount < 1 || strings.TrimSpace(input.Note) == "" || (input.Direction != "MASUK" && input.Direction != "KELUAR") {
		jsonOut(w, 400, map[string]string{"error": "bidang, arah arus, tanggal, nominal positif, dan keterangan wajib diisi"})
		return
	}
	principal, _ := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !cashbookFieldBelongsToPosition(r.Context(), a.db, principal, input.FieldKey) {
		jsonOut(w, 403, map[string]string{"error": "pencatatan hanya dapat dilakukan untuk bidang yang terhubung ke jabatan Anda"})
		return
	}
	if _, err := time.Parse("2006-01-02", input.Date); err != nil {
		jsonOut(w, 400, map[string]string{"error": "tanggal harus berformat YYYY-MM-DD"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, input.FieldKey); err != nil {
		fail(w, err)
		return
	}
	var income, spent int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount),0)::bigint FROM daily_finance_allocations WHERE allocation_key=$1 AND confirmed_at IS NOT NULL`, input.FieldKey).Scan(&income); err != nil {
		fail(w, err)
		return
	}
	var manualIn int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(sum(amount) FILTER(WHERE direction='MASUK'),0)::bigint,COALESCE(sum(amount) FILTER(WHERE direction='KELUAR'),0)::bigint FROM cashbook_transactions WHERE field_key=$1`, input.FieldKey).Scan(&manualIn, &spent); err != nil {
		fail(w, err)
		return
	}
	if input.FieldKey == "road_savings" {
		if err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions WHERE type='SETOR'`).Scan(&income); err != nil {
			fail(w, err)
			return
		}
	}
	if input.Direction == "KELUAR" && input.Amount > income+manualIn-spent {
		jsonOut(w, 409, map[string]any{"error": "pengeluaran melebihi saldo kas bidang", "balance": income + manualIn - spent})
		return
	}
	var id int64
	var fieldLabel string
	if strings.HasPrefix(input.FieldKey, "field_") {
		_ = tx.QueryRow(r.Context(), `SELECT name FROM organizational_fields WHERE id=$1`, strings.TrimPrefix(input.FieldKey, "field_")).Scan(&fieldLabel)
	} else {
		if input.FieldKey == "road_savings" {
			fieldLabel = "Tabungan Jalan"
		}
		for _, f := range financeAllocations(calculateMeetingFinance(0, 0, allocationTotal)) {
			if f.Key == input.FieldKey {
				fieldLabel = f.Label
				break
			}
		}
	}
	if err = tx.QueryRow(r.Context(), `INSERT INTO cashbook_transactions(field_key,field_label,direction,transaction_date,amount,note,recorded_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, input.FieldKey, fieldLabel, input.Direction, input.Date, input.Amount, strings.TrimSpace(input.Note), principal.UserID).Scan(&id); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "field_key": input.FieldKey, "direction": input.Direction, "date": input.Date, "amount": input.Amount})
}

func (a *App) confirmMeetingFinanceReceipt(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "id periode tidak valid"})
		return
	}
	var input struct {
		Key  string `json:"key"`
		Note string `json:"note"`
	}
	if jsonIn(r, &input) != nil {
		jsonOut(w, 400, map[string]string{"error": "key alokasi wajib diisi"})
		return
	}
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok {
		jsonOut(w, 401, map[string]string{"error": "login diperlukan"})
		return
	}
	if !canConfirmFinanceField(r.Context(), a.db, principal, input.Key) {
		jsonOut(w, 403, map[string]string{"error": "hanya pengurus bidang penerima yang dapat mengonfirmasi alokasi ini"})
		return
	}
	result, err := a.db.Exec(r.Context(), `UPDATE meeting_finance_receipts SET confirmed_by=$1,confirmed_at=now(),note=$2 WHERE period_id=$3 AND allocation_key=$4`, principal.UserID, strings.TrimSpace(input.Note), periodID, input.Key)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, 409, map[string]string{"error": "alokasi belum dibuat; proses rekap penerimaan kas terlebih dahulu"})
		return
	}
	jsonOut(w, 200, map[string]any{"period_id": periodID, "key": input.Key, "confirmed": true, "confirmed_by": principal.FullName})
}

func loadMeetingFinance(ctx context.Context, db *pgxpool.Pool, periodID int64) (*MeetingFinance, string, error) {
	var meetingDate, status string
	var paymentCount int
	var totalCollected int64
	err := db.QueryRow(ctx, `SELECT p.meeting_date::text,p.status,COUNT(pay.id)::int,COALESCE(SUM(pay.amount),0)::bigint
		FROM periods p LEFT JOIN payments pay ON timezone('Asia/Jakarta',pay.received_at)::date=p.meeting_date
		WHERE p.id=$1 GROUP BY p.id`, periodID).Scan(&meetingDate, &status, &paymentCount, &totalCollected)
	if err != nil {
		return nil, "", err
	}
	return calculateMeetingFinance(periodID, paymentCount, totalCollected), status, nil
}

// Alokasi penerimaan kas setiap kumpulan Ahad Pon. Nominalnya tetap; sisa
// setelah kelima pos ini masuk kas Bendahara Umum (field kas_rt).
const (
	allocationConsumption    int64 = 300000 // konsumsi
	allocationSocial         int64 = 150000 // dana sosial
	allocationStreetLighting int64 = 150000 // penerangan jalan
	allocationBaitulMal      int64 = 20000  // baitul mal (ziswaf)
	allocationMosque         int64 = 20000  // kemasjidan
	allocationTotal                = allocationConsumption + allocationSocial +
		allocationStreetLighting + allocationBaitulMal + allocationMosque
)

func calculateMeetingFinance(periodID int64, paymentCount int, total int64) *MeetingFinance {
	return &MeetingFinance{
		PeriodID:        periodID,
		PaymentCount:    paymentCount,
		TotalCollected:  total,
		PeneranganJalan: allocationStreetLighting,
		Sosial:          allocationSocial,
		Kemasjidan:      allocationMosque,
		Ziswaf:          allocationBaitulMal,
		HostConsumption: allocationConsumption,
		KasRT:           total - allocationTotal,
	}
}
func ensureMeetingFinanceSchema(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `ALTER TABLE periods ADD COLUMN IF NOT EXISTS meeting_time TIME NOT NULL DEFAULT '19:30'`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE residents ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE residents ADD COLUMN IF NOT EXISTS savings_class INTEGER NOT NULL DEFAULT 3`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE residents ADD COLUMN IF NOT EXISTS dues_required BOOLEAN NOT NULL DEFAULT TRUE`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE bills ADD COLUMN IF NOT EXISTS waived BOOLEAN NOT NULL DEFAULT FALSE`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS meeting_finance_receipts (
		period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE CASCADE, allocation_key TEXT NOT NULL, label TEXT NOT NULL,
		responsible_field TEXT NOT NULL, amount BIGINT NOT NULL CHECK(amount>=0), confirmed_by BIGINT,
		confirmed_at TIMESTAMPTZ, note TEXT NOT NULL DEFAULT '', PRIMARY KEY(period_id,allocation_key))`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS daily_finance_snapshots (
		receipt_date DATE PRIMARY KEY,payment_count INTEGER NOT NULL,collected_amount BIGINT NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS daily_finance_allocations (
		receipt_date DATE NOT NULL REFERENCES daily_finance_snapshots(receipt_date) ON DELETE CASCADE,allocation_key TEXT NOT NULL,label TEXT NOT NULL,
		responsible_field TEXT NOT NULL,amount BIGINT NOT NULL CHECK(amount>=0),confirmed_by BIGINT,confirmed_at TIMESTAMPTZ,note TEXT NOT NULL DEFAULT '',PRIMARY KEY(receipt_date,allocation_key))`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS cashbook_transactions (
		id BIGSERIAL PRIMARY KEY,field_key TEXT NOT NULL,field_label TEXT NOT NULL DEFAULT '',direction TEXT NOT NULL DEFAULT 'KELUAR' CHECK(direction IN ('MASUK','KELUAR')),transaction_date DATE NOT NULL,amount BIGINT NOT NULL CHECK(amount>0),
		note TEXT NOT NULL DEFAULT '',recorded_by BIGINT,created_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE cashbook_transactions ADD COLUMN IF NOT EXISTS field_label TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE cashbook_transactions ADD COLUMN IF NOT EXISTS direction TEXT NOT NULL DEFAULT 'KELUAR'`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_cashbook_transactions_date ON cashbook_transactions(transaction_date DESC,id DESC)`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE residents DROP CONSTRAINT IF EXISTS residents_savings_class_check`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE residents ADD CONSTRAINT residents_savings_class_check CHECK(savings_class BETWEEN 1 AND 4)`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE tabungan_transactions ADD COLUMN IF NOT EXISTS resident_id BIGINT REFERENCES residents(id)`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE tabungan_transactions ADD COLUMN IF NOT EXISTS period_id BIGINT REFERENCES periods(id)`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE tabungan_transactions ADD COLUMN IF NOT EXISTS stage_number INTEGER`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE tabungan_transactions ADD COLUMN IF NOT EXISTS recorded_by BIGINT`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_tabungan_resident_stage ON tabungan_transactions(resident_id,stage_number)`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS savings_stages(stage_number INTEGER PRIMARY KEY CHECK(stage_number>0),meeting_count INTEGER NOT NULL CHECK(meeting_count>0))`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO savings_stages(stage_number,meeting_count) SELECT n,4 FROM generate_series(1,5) n ON CONFLICT(stage_number) DO NOTHING`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE savings_stages ADD COLUMN IF NOT EXISTS bills_created_at TIMESTAMPTZ`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS savings_stage_bills(
		id BIGSERIAL PRIMARY KEY,stage_number INTEGER NOT NULL REFERENCES savings_stages(stage_number) ON DELETE CASCADE,
		resident_id BIGINT NOT NULL REFERENCES residents(id) ON DELETE CASCADE,amount BIGINT NOT NULL CHECK(amount>0),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(stage_number,resident_id))`); err != nil {
		return err
	}
	rows, err := db.Query(ctx, `SELECT stage_number FROM savings_stages WHERE bills_created_at IS NULL ORDER BY stage_number`)
	if err != nil {
		return err
	}
	stagesToBill := []int{}
	for rows.Next() {
		var stage int
		if err := rows.Scan(&stage); err != nil {
			rows.Close()
			return err
		}
		stagesToBill = append(stagesToBill, stage)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, stage := range stagesToBill {
		if _, err := db.Exec(ctx, `INSERT INTO savings_stage_bills(stage_number,resident_id,amount)
			SELECT $1,id,CASE savings_class WHEN 1 THEN 75000 WHEN 2 THEN 50000 ELSE 25000 END FROM residents WHERE active=true AND savings_class BETWEEN 1 AND 3 ON CONFLICT(stage_number,resident_id) DO NOTHING`, stage); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, `UPDATE savings_stages SET bills_created_at=now() WHERE stage_number=$1`, stage); err != nil {
			return err
		}
	}
	_, err = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS meeting_finances (
		period_id BIGINT PRIMARY KEY REFERENCES periods(id) ON DELETE CASCADE,
		payment_count INTEGER NOT NULL DEFAULT 0,
		collected_amount BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	if err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE meeting_finances DROP CONSTRAINT IF EXISTS meeting_finances_attendee_count_check`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `DO $$ BEGIN
		IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='meeting_finances' AND column_name='attendee_count') THEN
			ALTER TABLE meeting_finances ALTER COLUMN attendee_count SET DEFAULT 0;
		END IF;
	END $$`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE meeting_finances ADD COLUMN IF NOT EXISTS payment_count INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `ALTER TABLE meeting_finances ADD COLUMN IF NOT EXISTS collected_amount BIGINT NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	_, err = db.Exec(ctx, `UPDATE meeting_finances f SET payment_count=summary.payment_count,collected_amount=summary.total_collected
		FROM periods p CROSS JOIN LATERAL (
			SELECT COUNT(id)::int AS payment_count,COALESCE(SUM(amount),0)::bigint AS total_collected
			FROM payments WHERE timezone('Asia/Jakarta',received_at)::date=p.meeting_date
		) summary
		WHERE f.period_id=p.id AND f.payment_count=0 AND f.collected_amount=0`)
	return err
}
func (a *App) generateBills(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonOut(w, 400, map[string]string{"error": "invalid period id"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	if err = tx.QueryRow(r.Context(), "SELECT status FROM periods WHERE id=$1 FOR UPDATE", id).Scan(&status); err != nil {
		fail(w, err)
		return
	}
	if status != "TERLAKSANA" {
		jsonOut(w, 409, map[string]string{"error": "tagihan hanya dapat dibuat jika kumpulan berstatus TERLAKSANA; kumpulan LIBUR tidak ditagih"})
		return
	}
	created, err := createPeriodBills(r.Context(), tx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"created": created, "period_id": id, "amount": meetingBillAmount, "message": "Tagihan iuran kumpulan dibuat untuk seluruh warga aktif."})
}
func (a *App) bills(w http.ResponseWriter, r *http.Request) {
	var residentID int64
	if v := r.URL.Query().Get("resident_id"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			jsonOut(w, 400, map[string]string{"error": "resident_id tidak valid"})
			return
		}
		residentID = parsed
	}
	rows, err := a.db.Query(r.Context(), `SELECT b.id,b.resident_id,r.house_code,r.head_name,b.period_id,
			p.meeting_date::text,p.status,b.description,b.amount,COALESCE(sum(pa.amount),0)::bigint,
			b.amount-COALESCE(sum(pa.amount),0)::bigint
		FROM bills b
		JOIN residents r ON r.id=b.resident_id
		JOIN periods p ON p.id=b.period_id
		LEFT JOIN payment_allocations pa ON pa.bill_id=b.id
		WHERE b.waived=false AND ($1=0 OR b.resident_id=$1)
		GROUP BY b.id,r.house_code,r.head_name,p.meeting_date,p.status
		ORDER BY p.meeting_date DESC,r.house_code`, residentID)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []Bill{}
	for rows.Next() {
		var b Bill
		if err := rows.Scan(&b.ID, &b.ResidentID, &b.HouseCode, &b.HeadName, &b.PeriodID,
			&b.MeetingDate, &b.PeriodStatus, &b.Description, &b.Amount, &b.Paid, &b.Remaining); err != nil {
			fail(w, err)
			return
		}
		out = append(out, b)
	}
	jsonOut(w, 200, out)
}

// syncMeetingBills membuat tagihan yang belum ada untuk semua kumpulan
// TERLAKSANA. Dipakai untuk menyusul periode lama sebelum fitur otomatis aktif.
func (a *App) syncMeetingBills(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), "SELECT id FROM periods WHERE status='TERLAKSANA' ORDER BY meeting_date")
	if err != nil {
		fail(w, err)
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var created int64
	affected := 0
	for _, id := range ids {
		n, err := createPeriodBills(r.Context(), tx, id)
		if err != nil {
			fail(w, err)
			return
		}
		if n > 0 {
			affected++
		}
		created += n
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"periods": affected, "created": created, "amount": meetingBillAmount})
}
func (a *App) payment(w http.ResponseWriter, r *http.Request) {
	var p PaymentInput
	if jsonIn(r, &p) != nil || p.ResidentID < 1 || p.Amount < 1 {
		jsonOut(w, 400, map[string]string{"error": "resident_id dan amount positif wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var paymentID int64
	err = tx.QueryRow(r.Context(), "INSERT INTO payments(resident_id,amount,note) VALUES($1,$2,$3) RETURNING id", p.ResidentID, p.Amount, p.Note).Scan(&paymentID)
	if err != nil {
		fail(w, err)
		return
	}
	if p.Voluntary {
		if err = tx.Commit(r.Context()); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, 201, map[string]any{"payment_id": paymentID, "received": p.Amount, "allocated": 0, "bills_paid": 0, "remaining_unallocated": p.Amount, "message": "Pembayaran sukarela dicatat tanpa alokasi tagihan."})
		return
	}
	// bill_ids kosong = bayar semua tagihan belum lunas (lunas semua).
	// bill_ids terisi = bayar periode tertentu saja.
	var billIDs []int64
	if len(p.BillIDs) > 0 {
		billIDs = p.BillIDs
	}
	remaining := p.Amount
	rows, err := tx.Query(r.Context(), `SELECT b.id,
			b.amount-COALESCE((SELECT sum(pa.amount) FROM payment_allocations pa WHERE pa.bill_id=b.id),0)::bigint AS due
		FROM bills b
		WHERE b.resident_id=$1
			AND ($2::bigint[] IS NULL OR b.id = ANY($2))
			AND b.amount>COALESCE((SELECT sum(pa.amount) FROM payment_allocations pa WHERE pa.bill_id=b.id),0)
		ORDER BY b.period_id,b.id FOR UPDATE`, p.ResidentID, billIDs)
	if err != nil {
		fail(w, err)
		return
	}
	type dueRow struct {
		id  int64
		due int64
	}
	due := []dueRow{}
	for rows.Next() {
		var d dueRow
		if err = rows.Scan(&d.id, &d.due); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		due = append(due, d)
	}
	rows.Close()
	if len(due) == 0 {
		if len(billIDs) > 0 {
			jsonOut(w, 409, map[string]string{"error": "tagihan yang dipilih sudah lunas atau bukan milik warga ini"})
		} else {
			jsonOut(w, 409, map[string]string{"error": "warga tidak memiliki tagihan yang belum lunas"})
		}
		return
	}
	var allocated int64
	for _, d := range due {
		if remaining == 0 {
			break
		}
		n := d.due
		if n > remaining {
			n = remaining
		}
		_, err = tx.Exec(r.Context(), "INSERT INTO payment_allocations(payment_id,bill_id,amount) VALUES($1,$2,$3)", paymentID, d.id, n)
		if err != nil {
			fail(w, err)
			return
		}
		remaining -= n
		allocated += n
	}
	if remaining > 0 {
		jsonOut(w, 400, map[string]any{"error": "pembayaran melebihi total tunggakan", "max_payable": allocated})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"payment_id": paymentID, "received": p.Amount, "allocated": allocated, "bills_paid": len(due), "remaining_unallocated": remaining, "message": "Pembayaran dialokasikan ke tagihan terlama terlebih dahulu."})
}

func (a *App) listPayments(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT p.id,p.resident_id,r.house_code,r.head_name,p.amount,p.note,p.received_at::text,
		COALESCE(SUM(pa.amount),0)::bigint FROM payments p JOIN residents r ON r.id=p.resident_id
		LEFT JOIN payment_allocations pa ON pa.payment_id=p.id
		GROUP BY p.id,r.house_code,r.head_name ORDER BY p.received_at DESC,p.id DESC LIMIT 300`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	out := []PaymentRecord{}
	for rows.Next() {
		var p PaymentRecord
		if err := rows.Scan(&p.ID, &p.ResidentID, &p.HouseCode, &p.HeadName, &p.Amount, &p.Note, &p.ReceivedAt, &p.Allocated); err != nil {
			fail(w, err)
			return
		}
		out = append(out, p)
	}
	jsonOut(w, 200, out)
}

func (a *App) updatePayment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var input struct {
		Amount int64 `json:"amount"`
	}
	if err != nil || id < 1 || jsonIn(r, &input) != nil || input.Amount < 1 {
		jsonOut(w, 400, map[string]string{"error": "id pembayaran dan nominal positif wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var exists bool
	if err = tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM payments WHERE id=$1)", id).Scan(&exists); err != nil {
		fail(w, err)
		return
	}
	if !exists {
		jsonOut(w, 404, map[string]string{"error": "pembayaran tidak ditemukan"})
		return
	}
	var allocated int64
	if err = tx.QueryRow(r.Context(), "SELECT COALESCE(sum(amount),0) FROM payment_allocations WHERE payment_id=$1", id).Scan(&allocated); err != nil {
		fail(w, err)
		return
	}
	if input.Amount < allocated {
		cut := allocated - input.Amount
		rows, qerr := tx.Query(r.Context(), "SELECT id,amount FROM payment_allocations WHERE payment_id=$1 ORDER BY created_at DESC,id DESC FOR UPDATE", id)
		if qerr != nil {
			fail(w, qerr)
			return
		}
		type allocation struct{ id, amount int64 }
		allocations := []allocation{}
		for rows.Next() {
			var a allocation
			if err = rows.Scan(&a.id, &a.amount); err != nil {
				rows.Close()
				fail(w, err)
				return
			}
			allocations = append(allocations, a)
		}
		rows.Close()
		for _, a := range allocations {
			if cut == 0 {
				break
			}
			reduce := a.amount
			if reduce > cut {
				reduce = cut
			}
			if reduce == a.amount {
				_, err = tx.Exec(r.Context(), "DELETE FROM payment_allocations WHERE id=$1", a.id)
			} else {
				_, err = tx.Exec(r.Context(), "UPDATE payment_allocations SET amount=amount-$1 WHERE id=$2", reduce, a.id)
			}
			if err != nil {
				fail(w, err)
				return
			}
			cut -= reduce
		}
	}
	if _, err = tx.Exec(r.Context(), "UPDATE payments SET amount=$1 WHERE id=$2", input.Amount, id); err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "amount": input.Amount})
}

func (a *App) deletePayment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonOut(w, 400, map[string]string{"error": "id pembayaran tidak valid"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "DELETE FROM payment_allocations WHERE payment_id=$1", id); err != nil {
		fail(w, err)
		return
	}
	result, err := tx.Exec(r.Context(), "DELETE FROM payments WHERE id=$1", id)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, 404, map[string]string{"error": "pembayaran tidak ditemukan"})
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"deleted": id})
}
func (a *App) jimpitan(w http.ResponseWriter, r *http.Request) {
	var x JimpitanInput
	if jsonIn(r, &x) != nil || x.QRToken == "" || x.SessionDate == "" {
		jsonOut(w, 400, map[string]string{"error": "qr_token dan session_date wajib diisi"})
		return
	}
	if x.Amount < 0 {
		jsonOut(w, 400, map[string]string{"error": "amount tidak boleh negatif"})
		return
	}
	if x.Status == "" {
		x.Status = "TERAMBIL"
	}
	var id int64
	err := a.db.QueryRow(r.Context(), "INSERT INTO jimpitan_transactions(resident_id,session_date,amount,status,note) SELECT id,$2,$3,$4,$5 FROM residents WHERE qr_token=$1 RETURNING id", x.QRToken, x.SessionDate, x.Amount, x.Status, x.Note).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "QR rumah tidak ditemukan"})
			return
		}
		fail(w, err)
		return
	}
	jsonOut(w, 201, map[string]any{"id": id, "message": "Catatan jimpitan tersimpan"})
}
func (a *App) jimpitanList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), "SELECT j.id,r.house_code,r.head_name,j.session_date::text,j.amount,j.status,j.note FROM jimpitan_transactions j JOIN residents r ON r.id=j.resident_id ORDER BY j.session_date DESC,j.id DESC LIMIT 200")
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID        int64  `json:"id"`
		HouseCode string `json:"house_code"`
		HeadName  string `json:"head_name"`
		Date      string `json:"date"`
		Amount    int64  `json:"amount"`
		Status    string `json:"status"`
		Note      string `json:"note"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ID, &x.HouseCode, &x.HeadName, &x.Date, &x.Amount, &x.Status, &x.Note); err != nil {
			fail(w, err)
			return
		}
		out = append(out, x)
	}
	jsonOut(w, 200, out)
}
func (a *App) tabungan(w http.ResponseWriter, r *http.Request) {
	var x TabunganInput
	if jsonIn(r, &x) != nil || x.ResidentID < 1 || x.PeriodID < 1 || x.Amount < 1 {
		jsonOut(w, 400, map[string]string{"error": "resident_id, period_id, dan amount positif wajib diisi"})
		return
	}
	if x.Type == "" {
		x.Type = "SETOR"
	}
	if x.Type == "TARIK" {
		jsonOut(w, 400, map[string]string{"error": "tabungan infrastruktur tidak dapat ditarik kembali"})
		return
	}
	if x.Type != "SETOR" {
		jsonOut(w, 400, map[string]string{"error": "jenis transaksi hanya SETOR"})
		return
	}
	var memberName string
	var savingsClass int
	if err := a.db.QueryRow(r.Context(), "SELECT head_name,savings_class FROM residents WHERE id=$1 AND active=true", x.ResidentID).Scan(&memberName, &savingsClass); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "warga tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	var meetingDate time.Time
	var automatic bool
	var completed bool
	if err := a.db.QueryRow(r.Context(), `SELECT meeting_date,note='Jadwal Ahad Pon otomatis',status='TERLAKSANA' FROM periods WHERE id=$1`, x.PeriodID).Scan(&meetingDate, &automatic, &completed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "periode kumpulan tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	stageNumber := 0
	if automatic && completed {
		var err error
		stageNumber, err = savingsStageForMeeting(r.Context(), a.db, meetingDate)
		if err != nil {
			fail(w, err)
			return
		}
	}
	if stageNumber == 0 {
		jsonOut(w, 400, map[string]string{"error": "transaksi tahap tabungan harus terkait ke jadwal Ahad Pon otomatis"})
		return
	}
	if x.Type == "SETOR" && savingsClass != 4 {
		var target int64
		if err := a.db.QueryRow(r.Context(), `SELECT amount FROM savings_stage_bills WHERE resident_id=$1 AND stage_number=$2`, x.ResidentID, stageNumber).Scan(&target); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				jsonOut(w, 409, map[string]string{"error": "tagihan untuk tahap ini belum dibuat"})
			} else {
				fail(w, err)
			}
			return
		}
		var paid int64
		if err := a.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions
			WHERE resident_id=$1 AND stage_number=$2 AND type='SETOR'`, x.ResidentID, stageNumber).Scan(&paid); err != nil {
			fail(w, err)
			return
		}
		remaining := target - paid
		if remaining <= 0 || x.Amount > remaining {
			jsonOut(w, 409, map[string]any{"error": "setoran melebihi sisa tagihan tabungan tahap", "remaining_due": max(remaining, 0)})
			return
		}
	}
	var id int64
	principal, _ := r.Context().Value(authContextKey{}).(AuthPrincipal)
	err := a.db.QueryRow(r.Context(), `INSERT INTO tabungan_transactions(group_name,member_name,amount,type,note,resident_id,period_id,stage_number)
		VALUES($1,$2,$3,'SETOR',$4,$5,$6,$7) RETURNING id`, fmt.Sprintf("Kelas %d", savingsClass), memberName, x.Amount, x.Note, x.ResidentID, x.PeriodID, stageNumber).Scan(&id)
	if err != nil {
		fail(w, err)
		return
	}
	_, _ = a.db.Exec(r.Context(), `UPDATE tabungan_transactions SET recorded_by=$1 WHERE id=$2`, principal.UserID, id)
	jsonOut(w, 201, map[string]any{"id": id, "stage_number": stageNumber, "message": "Transaksi tabungan tersimpan"})
}
func (a *App) tabunganList(w http.ResponseWriter, r *http.Request) {
	page := max(parseInt(r.URL.Query().Get("page"), 1), 1)
	pageSize := min(max(parseInt(r.URL.Query().Get("page_size"), 10), 1), 100)
	stage := parseInt(r.URL.Query().Get("stage"), 0)
	class := parseInt(r.URL.Query().Get("class"), 0)
	var total int
	if err := a.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM residents WHERE active=true AND ($1=0 OR savings_class=$1)`, class).Scan(&total); err != nil {
		fail(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT r.id,r.house_code,r.head_name,r.savings_class,$1::int AS stage,
		COALESCE(b.target,0)::bigint,COALESCE(paid.deposited,0)::bigint
		FROM residents r
		LEFT JOIN (SELECT resident_id,SUM(amount)::bigint AS target FROM savings_stage_bills WHERE ($1=0 OR stage_number=$1) GROUP BY resident_id) b ON b.resident_id=r.id
		LEFT JOIN (SELECT resident_id,SUM(amount) FILTER(WHERE type='SETOR')::bigint AS deposited FROM tabungan_transactions WHERE ($1=0 OR stage_number=$1) GROUP BY resident_id) paid ON paid.resident_id=r.id
		WHERE r.active=true AND ($4=0 OR r.savings_class=$4)
		ORDER BY r.house_code LIMIT $2 OFFSET $3`, stage, pageSize, (page-1)*pageSize, class)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ResidentID   int64  `json:"resident_id"`
		HouseCode    string `json:"house_code"`
		Name         string `json:"name"`
		SavingsClass int    `json:"savings_class"`
		Stage        int    `json:"stage"`
		Tariff       int64  `json:"tariff"`
		Deposited    int64  `json:"deposited"`
		Remaining    int64  `json:"remaining"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ResidentID, &x.HouseCode, &x.Name, &x.SavingsClass, &x.Stage, &x.Tariff, &x.Deposited); err != nil {
			fail(w, err)
			return
		}
		x.Remaining = max(x.Tariff-x.Deposited, 0)
		out = append(out, x)
	}
	jsonOut(w, 200, map[string]any{"items": out, "page": page, "page_size": pageSize, "total": total})
}

func (a *App) tabunganTransactions(w http.ResponseWriter, r *http.Request) {
	page := max(parseInt(r.URL.Query().Get("page"), 1), 1)
	pageSize := min(max(parseInt(r.URL.Query().Get("page_size"), 10), 1), 100)
	stage := parseInt(r.URL.Query().Get("stage"), 0)
	var total int
	if err := a.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM tabungan_transactions WHERE ($1=0 OR stage_number=$1)`, stage).Scan(&total); err != nil {
		fail(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT t.id,COALESCE(t.resident_id,0),COALESCE(r.house_code,''),t.member_name,COALESCE(t.stage_number,0),t.amount,t.note,
		COALESCE(p.meeting_date::text,''),t.created_at
		FROM tabungan_transactions t LEFT JOIN residents r ON r.id=t.resident_id LEFT JOIN periods p ON p.id=t.period_id
		WHERE ($1=0 OR t.stage_number=$1) ORDER BY t.created_at DESC,t.id DESC LIMIT $2 OFFSET $3`, stage, pageSize, (page-1)*pageSize)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type transaction struct {
		ID          int64     `json:"id"`
		ResidentID  int64     `json:"resident_id"`
		HouseCode   string    `json:"house_code"`
		Name        string    `json:"name"`
		Stage       int       `json:"stage"`
		Amount      int64     `json:"amount"`
		Note        string    `json:"note"`
		MeetingDate string    `json:"meeting_date"`
		CreatedAt   time.Time `json:"created_at"`
	}
	items := []transaction{}
	for rows.Next() {
		var item transaction
		if err := rows.Scan(&item.ID, &item.ResidentID, &item.HouseCode, &item.Name, &item.Stage, &item.Amount, &item.Note, &item.MeetingDate, &item.CreatedAt); err != nil {
			fail(w, err)
			return
		}
		items = append(items, item)
	}
	jsonOut(w, 200, map[string]any{"items": items, "page": page, "page_size": pageSize, "total": total})
}
func fail(w http.ResponseWriter, err error) {
	log.Printf("request error: %v", err)
	jsonOut(w, 500, map[string]string{"error": "internal server error"})
}

// isUniqueViolation melaporkan apakah error berasal dari constraint UNIQUE Postgres.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
