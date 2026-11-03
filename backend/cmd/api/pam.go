package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pamSettingsInput struct {
	RatePerM3             int64              `json:"rate_per_m3"`
	MonthlyFee            int64              `json:"monthly_fee"`
	PaymentMethods        []pamPaymentMethod `json:"payment_methods"`
	CommissionBasisPoints int64              `json:"commission_basis_points"`
}

type pamPaymentMethod struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	Instructions  string `json:"instructions"`
	Active        bool   `json:"active"`
	AccountNumber string `json:"account_number"`
	ImageData     string `json:"image_data"`
}

type pamCustomerInput struct {
	ResidentID  int64  `json:"resident_id"`
	MeterNumber string `json:"meter_number"`
	Active      *bool  `json:"active"`
}

type pamReadingInput struct {
	CustomerID     int64  `json:"customer_id"`
	BillingMonth   string `json:"billing_month"`
	CurrentReading int64  `json:"current_reading"`
}

type pamFeeOnlyBillInput struct {
	CustomerID   int64  `json:"customer_id"`
	BillingMonth string `json:"billing_month"`
}

type pamPaymentInput struct {
	BillID     int64  `json:"bill_id"`
	Amount     int64  `json:"amount"`
	Note       string `json:"note"`
	MethodCode string `json:"method_code"`
}

type pamPaymentReviewInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

type pamPaymentConfirmationInput struct {
	BillID     int64  `json:"bill_id"`
	Amount     int64  `json:"amount"`
	MethodCode string `json:"method_code"`
	ProofData  string `json:"proof_data"`
	Note       string `json:"note"`
}

type pamReadingReviewInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

var pamPaymentMethodCode = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func ensurePamSchema(ctx context.Context, db *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS pam_settings (
			id SMALLINT PRIMARY KEY CHECK(id=1), rate_per_m3 BIGINT NOT NULL DEFAULT 0 CHECK(rate_per_m3>=0),
			monthly_fee BIGINT NOT NULL DEFAULT 0 CHECK(monthly_fee>=0), updated_by BIGINT REFERENCES app_users(id),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			payment_methods JSONB NOT NULL DEFAULT '[{"code":"collection","name":"Diambil petugas","instructions":"Petugas akan menghubungi Anda untuk mengambil pembayaran.","active":true},{"code":"bank_transfer","name":"Transfer Bank","instructions":"Hubungi pengurus untuk mendapatkan informasi rekening resmi.","active":true},{"code":"qris","name":"QRIS","instructions":"Hubungi pengurus untuk mendapatkan kode QRIS resmi.","active":true}]'::jsonb,
			commission_basis_points INT NOT NULL DEFAULT 0 CHECK(commission_basis_points BETWEEN 0 AND 10000)
		)`,
		`INSERT INTO pam_settings(id) VALUES(1) ON CONFLICT(id) DO NOTHING`,
		`ALTER TABLE pam_settings ADD COLUMN IF NOT EXISTS payment_methods JSONB NOT NULL DEFAULT '[{"code":"collection","name":"Diambil petugas","instructions":"Petugas akan menghubungi Anda untuk mengambil pembayaran.","active":true},{"code":"bank_transfer","name":"Transfer Bank","instructions":"Hubungi pengurus untuk mendapatkan informasi rekening resmi.","active":true},{"code":"qris","name":"QRIS","instructions":"Hubungi pengurus untuk mendapatkan kode QRIS resmi.","active":true}]'::jsonb`,
		`ALTER TABLE pam_settings ADD COLUMN IF NOT EXISTS commission_basis_points INT NOT NULL DEFAULT 0 CHECK(commission_basis_points BETWEEN 0 AND 10000)`,
		`CREATE TABLE IF NOT EXISTS pam_customers (
			id BIGSERIAL PRIMARY KEY, resident_id BIGINT NOT NULL UNIQUE REFERENCES residents(id),
			meter_number TEXT NOT NULL UNIQUE, active BOOLEAN NOT NULL DEFAULT TRUE, enrolled_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS pam_readings (
			id BIGSERIAL PRIMARY KEY, customer_id BIGINT NOT NULL REFERENCES pam_customers(id), billing_month DATE NOT NULL,
			previous_reading BIGINT NOT NULL CHECK(previous_reading>=0), current_reading BIGINT NOT NULL CHECK(current_reading>=previous_reading),
			consumption BIGINT NOT NULL CHECK(consumption>=0), recorded_by BIGINT REFERENCES app_users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			status TEXT NOT NULL DEFAULT 'APPROVED' CHECK(status IN ('PENDING','APPROVED','REJECTED')),
			reviewed_by BIGINT REFERENCES app_users(id), reviewed_at TIMESTAMPTZ, review_note TEXT NOT NULL DEFAULT '',
			UNIQUE(customer_id,billing_month)
		)`,
		`ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'APPROVED' CHECK(status IN ('PENDING','APPROVED','REJECTED'))`,
		`ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS reviewed_by BIGINT REFERENCES app_users(id)`,
		`ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ`,
		`ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS review_note TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS fee_only BOOLEAN NOT NULL DEFAULT FALSE`,
		`CREATE TABLE IF NOT EXISTS pam_bills (
			id BIGSERIAL PRIMARY KEY, reading_id BIGINT NOT NULL UNIQUE REFERENCES pam_readings(id),
			rate_per_m3 BIGINT NOT NULL CHECK(rate_per_m3>=0), monthly_fee BIGINT NOT NULL CHECK(monthly_fee>=0),
			total BIGINT NOT NULL CHECK(total>=0), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS pam_payments (
			id BIGSERIAL PRIMARY KEY, bill_id BIGINT NOT NULL REFERENCES pam_bills(id), amount BIGINT NOT NULL CHECK(amount>0),
			note TEXT NOT NULL DEFAULT '', received_by BIGINT REFERENCES app_users(id), received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			method_code TEXT NOT NULL DEFAULT 'collection'
		)`,
		`ALTER TABLE pam_payments ADD COLUMN IF NOT EXISTS method_code TEXT NOT NULL DEFAULT 'collection'`,
		`CREATE TABLE IF NOT EXISTS pam_payment_choices (
			bill_id BIGINT PRIMARY KEY REFERENCES pam_bills(id) ON DELETE CASCADE,
			resident_id BIGINT NOT NULL REFERENCES residents(id), method_code TEXT NOT NULL,
			selected_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS pam_payment_submissions (
			id BIGSERIAL PRIMARY KEY, bill_id BIGINT NOT NULL REFERENCES pam_bills(id),
			resident_id BIGINT NOT NULL REFERENCES residents(id), amount BIGINT NOT NULL CHECK(amount>0),
			method_code TEXT NOT NULL, proof_data TEXT NOT NULL, note TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','APPROVED','REJECTED')),
			submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(), reviewed_by BIGINT REFERENCES app_users(id),
			reviewed_at TIMESTAMPTZ, review_note TEXT NOT NULL DEFAULT '', payment_id BIGINT REFERENCES pam_payments(id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_pam_one_pending_submission_per_bill ON pam_payment_submissions(bill_id) WHERE status='PENDING'`,
		`CREATE INDEX IF NOT EXISTS idx_pam_payment_submissions_status ON pam_payment_submissions(status,submitted_at)`,
		`CREATE TABLE IF NOT EXISTS pam_commissions (
			bill_id BIGINT PRIMARY KEY REFERENCES pam_bills(id), staff_user_id BIGINT NOT NULL REFERENCES app_users(id),
			bill_total BIGINT NOT NULL, rate_basis_points INT NOT NULL CHECK(rate_basis_points BETWEEN 0 AND 10000),
			amount BIGINT NOT NULL CHECK(amount>=0), earned_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pam_commissions_staff ON pam_commissions(staff_user_id,earned_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pam_readings_customer_month ON pam_readings(customer_id,billing_month DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_pam_payments_bill ON pam_payments(bill_id)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// pamFeeOnlyBill records a monthly fixed fee for a missed meter month without
// inventing water consumption or changing the last actual meter reading.
func (a *App) pamFeeOnlyBill(w http.ResponseWriter, r *http.Request) {
	var input pamFeeOnlyBillInput
	if jsonIn(r, &input) != nil || input.CustomerID < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "pelanggan dan bulan tagihan wajib diisi"})
		return
	}
	month, err := time.Parse("2006-01", input.BillingMonth)
	if err != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "bulan harus berformat YYYY-MM"})
		return
	}
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var fee int64
	if err := tx.QueryRow(r.Context(), "SELECT monthly_fee FROM pam_settings WHERE id=1 FOR SHARE").Scan(&fee); err != nil {
		fail(w, err)
		return
	}
	if fee < 1 {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "biaya tetap bulanan PAM belum diatur atau bernilai nol"})
		return
	}
	var previous int64
	err = tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT current_reading FROM pam_readings
		WHERE customer_id=$1 AND billing_month<$2 AND status='APPROVED' AND fee_only=false
		ORDER BY billing_month DESC LIMIT 1),0) FROM pam_customers WHERE id=$1 AND active=true FOR UPDATE`, input.CustomerID, month).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "pelanggan PAM aktif tidak ditemukan"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	var readingID int64
	err = tx.QueryRow(r.Context(), `INSERT INTO pam_readings(customer_id,billing_month,previous_reading,current_reading,consumption,recorded_by,fee_only)
		VALUES($1,$2,$3,$3,0,$4,true) RETURNING id`, input.CustomerID, month, previous, principal.UserID).Scan(&readingID)
	if err != nil {
		if strings.Contains(err.Error(), "pam_readings_customer_id_billing_month_key") {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "meter atau tagihan bulan tersebut sudah dicatat"})
		} else {
			fail(w, err)
		}
		return
	}
	var billID int64
	if err := tx.QueryRow(r.Context(), `INSERT INTO pam_bills(reading_id,rate_per_m3,monthly_fee,total)
		VALUES($1,0,$2,$2) RETURNING id`, readingID, fee).Scan(&billID); err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"bill_id": billID, "billing_month": month.Format("2006-01"), "fee_only": true, "total": fee})
}

func (a *App) pamSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var settings pamSettingsInput
		var methods []byte
		if err := a.db.QueryRow(r.Context(), "SELECT rate_per_m3,monthly_fee,payment_methods,commission_basis_points FROM pam_settings WHERE id=1").Scan(&settings.RatePerM3, &settings.MonthlyFee, &methods, &settings.CommissionBasisPoints); err != nil {
			fail(w, err)
			return
		}
		if err := json.Unmarshal(methods, &settings.PaymentMethods); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, settings)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var input pamSettingsInput
	if jsonIn(r, &input) != nil || input.RatePerM3 < 0 || input.MonthlyFee < 0 ||
		input.CommissionBasisPoints < 0 || input.CommissionBasisPoints > 10000 || !validPAMPaymentMethods(input.PaymentMethods) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "tarif dan komisi harus valid (komisi 0–100%) serta minimal satu metode pembayaran aktif wajib diisi"})
		return
	}
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	methods, err := json.Marshal(input.PaymentMethods)
	if err != nil {
		fail(w, err)
		return
	}
	var currentMethodsJSON []byte
	if err := a.db.QueryRow(r.Context(), "SELECT payment_methods FROM pam_settings WHERE id=1").Scan(&currentMethodsJSON); err != nil {
		fail(w, err)
		return
	}
	var currentMethods []pamPaymentMethod
	if err := json.Unmarshal(currentMethodsJSON, &currentMethods); err != nil {
		fail(w, err)
		return
	}
	keptCodes := make(map[string]bool, len(input.PaymentMethods))
	for _, method := range input.PaymentMethods {
		keptCodes[method.Code] = true
		if err := validatePAMImageData(method.ImageData); err != nil {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "gambar QRIS tidak valid: " + err.Error()})
			return
		}
	}
	for _, method := range currentMethods {
		if keptCodes[method.Code] {
			continue
		}
		var used bool
		if err := a.db.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pam_payment_choices WHERE method_code=$1)", method.Code).Scan(&used); err != nil {
			fail(w, err)
			return
		}
		if used {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "metode yang sudah dipilih warga tidak dapat dihapus; nonaktifkan metode tersebut"})
			return
		}
	}
	_, err = a.db.Exec(r.Context(), `UPDATE pam_settings SET rate_per_m3=$1,monthly_fee=$2,payment_methods=$3,
		commission_basis_points=$4,updated_by=$5,updated_at=now() WHERE id=1`,
		input.RatePerM3, input.MonthlyFee, methods, input.CommissionBasisPoints, principal.UserID)
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, input)
}

func validatePAMImageData(data string) error {
	if data == "" {
		return nil
	}
	if len(data) > 1400000 {
		return errors.New("ukuran gambar melebihi 1 MB")
	}
	header, encoded, ok := strings.Cut(data, ",")
	if !ok || !strings.HasSuffix(header, ";base64") {
		return errors.New("format gambar harus PNG, JPEG, atau WebP")
	}
	contentType := strings.TrimPrefix(strings.TrimSuffix(header, ";base64"), "data:")
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/webp" {
		return errors.New("format gambar harus PNG, JPEG, atau WebP")
	}
	image, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(image) > 1<<20 {
		return errors.New("data gambar tidak valid atau melebihi 1 MB")
	}
	if detected := http.DetectContentType(image); detected != contentType {
		return errors.New("format gambar tidak sesuai isi berkas")
	}
	return nil
}

func validPAMPaymentMethods(methods []pamPaymentMethod) bool {
	if len(methods) == 0 || len(methods) > 20 {
		return false
	}
	codes := make(map[string]bool, len(methods))
	active := false
	for _, method := range methods {
		code := strings.TrimSpace(method.Code)
		name := strings.TrimSpace(method.Name)
		if !pamPaymentMethodCode.MatchString(code) || name == "" || len(name) > 80 ||
			len(method.Instructions) > 500 || len(method.AccountNumber) > 100 || codes[code] {
			return false
		}
		codes[code] = true
		active = active || method.Active
	}
	return active
}

func pamBillTotal(consumption, rate, fee int64) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)
	if consumption < 0 || rate < 0 || fee < 0 || (rate > 0 && consumption > (maxInt64-fee)/rate) {
		return 0, false
	}
	return consumption*rate + fee, true
}

func (a *App) pamCustomers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := a.db.Query(r.Context(), `SELECT c.id,c.resident_id,r.house_code,r.head_name,c.meter_number,c.active,
			COALESCE(last.current_reading,0),COALESCE(last.billing_month::text,''),COALESCE(lastbill.remaining,0)
			FROM pam_customers c JOIN residents r ON r.id=c.resident_id
			LEFT JOIN LATERAL (SELECT current_reading,billing_month FROM pam_readings WHERE customer_id=c.id AND status='APPROVED' AND fee_only=false ORDER BY billing_month DESC LIMIT 1) last ON true
			LEFT JOIN LATERAL (SELECT b.total-COALESCE(SUM(p.amount),0)::bigint AS remaining FROM pam_bills b LEFT JOIN pam_payments p ON p.bill_id=b.id JOIN pam_readings pr ON pr.id=b.reading_id WHERE pr.customer_id=c.id GROUP BY b.id,pr.billing_month ORDER BY pr.billing_month DESC LIMIT 1) lastbill ON true
			ORDER BY r.house_code`)
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		type item struct {
			ID          int64  `json:"id"`
			ResidentID  int64  `json:"resident_id"`
			HouseCode   string `json:"house_code"`
			HeadName    string `json:"head_name"`
			MeterNumber string `json:"meter_number"`
			Active      bool   `json:"active"`
			LastReading int64  `json:"last_reading"`
			LastMonth   string `json:"last_month"`
			LastDue     int64  `json:"last_due"`
		}
		out := []item{}
		for rows.Next() {
			var x item
			if err := rows.Scan(&x.ID, &x.ResidentID, &x.HouseCode, &x.HeadName, &x.MeterNumber, &x.Active, &x.LastReading, &x.LastMonth, &x.LastDue); err != nil {
				fail(w, err)
				return
			}
			out = append(out, x)
		}
		jsonOut(w, http.StatusOK, out)
		return
	}
	var input pamCustomerInput
	if jsonIn(r, &input) != nil || input.ResidentID < 1 || strings.TrimSpace(input.MeterNumber) == "" {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "warga dan nomor meter wajib diisi"})
		return
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	var id int64
	err := a.db.QueryRow(r.Context(), `INSERT INTO pam_customers(resident_id,meter_number,active) VALUES($1,$2,$3) RETURNING id`, input.ResidentID, strings.TrimSpace(input.MeterNumber), active).Scan(&id)
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"id": id, "message": "Pelanggan PAM ditambahkan"})
}

func (a *App) pamEligibleResidents(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT r.id,r.house_code,r.head_name FROM residents r
		LEFT JOIN pam_customers c ON c.resident_id=r.id WHERE r.active=true AND c.id IS NULL ORDER BY r.house_code`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID        int64  `json:"id"`
		HouseCode string `json:"house_code"`
		HeadName  string `json:"head_name"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ID, &x.HouseCode, &x.HeadName); err != nil {
			fail(w, err)
			return
		}
		out = append(out, x)
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *App) pamCustomer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "id pelanggan tidak valid"})
		return
	}
	if r.Method == http.MethodDelete {
		result, err := a.db.Exec(r.Context(), "UPDATE pam_customers SET active=false WHERE id=$1", id)
		if err != nil {
			fail(w, err)
			return
		}
		if result.RowsAffected() == 0 {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "pelanggan tidak ditemukan"})
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"deactivated": true})
		return
	}
	var input pamCustomerInput
	if jsonIn(r, &input) != nil || strings.TrimSpace(input.MeterNumber) == "" || input.Active == nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "nomor meter dan status pelanggan wajib diisi"})
		return
	}
	result, err := a.db.Exec(r.Context(), "UPDATE pam_customers SET meter_number=$1,active=$2 WHERE id=$3", strings.TrimSpace(input.MeterNumber), *input.Active, id)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "pelanggan tidak ditemukan"})
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"updated": true})
}

func (a *App) pamReading(w http.ResponseWriter, r *http.Request) {
	var input pamReadingInput
	if jsonIn(r, &input) != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "data meter tidak valid"})
		return
	}
	month, monthErr := time.Parse("2006-01", input.BillingMonth)
	if monthErr != nil || input.CustomerID < 1 || input.CurrentReading < 0 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "pelanggan, bulan YYYY-MM, dan meter non-negatif wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var rate, fee int64
	if err := tx.QueryRow(r.Context(), "SELECT rate_per_m3,monthly_fee FROM pam_settings WHERE id=1 FOR SHARE").Scan(&rate, &fee); err != nil {
		fail(w, err)
		return
	}
	if rate == 0 {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "tarif PAM belum diatur administrator"})
		return
	}
	var previous int64
	if err := tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT current_reading FROM pam_readings WHERE customer_id=$1 AND billing_month<$2 AND status='APPROVED' AND fee_only=false ORDER BY billing_month DESC LIMIT 1),0) FROM pam_customers WHERE id=$1 AND active=true FOR UPDATE`, input.CustomerID, month).Scan(&previous); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "pelanggan PAM aktif tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	if input.CurrentReading < previous {
		jsonOut(w, http.StatusBadRequest, map[string]any{"error": "meter baru tidak boleh lebih kecil dari meter sebelumnya", "previous_reading": previous})
		return
	}
	consumption := input.CurrentReading - previous
	total, validTotal := pamBillTotal(consumption, rate, fee)
	if !validTotal {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "total tagihan melebihi batas nominal yang dapat diproses"})
		return
	}
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	month = month.UTC()
	var readingID int64
	err = tx.QueryRow(r.Context(), `INSERT INTO pam_readings(customer_id,billing_month,previous_reading,current_reading,consumption,recorded_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, input.CustomerID, month, previous, input.CurrentReading, consumption, principal.UserID).Scan(&readingID)
	if err != nil {
		if strings.Contains(err.Error(), "pam_readings_customer_id_billing_month_key") {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "meter bulan tersebut sudah dicatat"})
		} else {
			fail(w, err)
		}
		return
	}
	var billID int64
	if err := tx.QueryRow(r.Context(), "INSERT INTO pam_bills(reading_id,rate_per_m3,monthly_fee,total) VALUES($1,$2,$3,$4) RETURNING id", readingID, rate, fee, total).Scan(&billID); err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"bill_id": billID, "billing_month": month.Format("2006-01"), "previous_reading": previous, "current_reading": input.CurrentReading, "consumption": consumption, "rate_per_m3": rate, "monthly_fee": fee, "total": total})
}

func (a *App) pamPendingReadings(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT pr.id,c.resident_id,r.house_code,r.head_name,c.meter_number,
		pr.billing_month::text,pr.previous_reading,pr.current_reading,pr.consumption,pr.created_at::text
		FROM pam_readings pr JOIN pam_customers c ON c.id=pr.customer_id JOIN residents r ON r.id=c.resident_id
		WHERE pr.status='PENDING' ORDER BY pr.created_at,pr.id`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID              int64  `json:"id"`
		ResidentID      int64  `json:"resident_id"`
		HouseCode       string `json:"house_code"`
		HeadName        string `json:"head_name"`
		MeterNumber     string `json:"meter_number"`
		BillingMonth    string `json:"billing_month"`
		PreviousReading int64  `json:"previous_reading"`
		CurrentReading  int64  `json:"current_reading"`
		Consumption     int64  `json:"consumption"`
		CreatedAt       string `json:"created_at"`
	}
	out := []item{}
	for rows.Next() {
		var reading item
		if err := rows.Scan(&reading.ID, &reading.ResidentID, &reading.HouseCode, &reading.HeadName, &reading.MeterNumber,
			&reading.BillingMonth, &reading.PreviousReading, &reading.CurrentReading, &reading.Consumption, &reading.CreatedAt); err != nil {
			fail(w, err)
			return
		}
		out = append(out, reading)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *App) pamReviewReading(w http.ResponseWriter, r *http.Request) {
	readingID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || readingID < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "id pencatatan meter tidak valid"})
		return
	}
	var input pamReadingReviewInput
	if jsonIn(r, &input) != nil || (input.Decision != "approve" && input.Decision != "reject") || len(input.Note) > 500 ||
		(input.Decision == "reject" && strings.TrimSpace(input.Note) == "") {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "pilih setujui atau tolak; alasan penolakan wajib diisi dan catatan maksimal 500 karakter"})
		return
	}
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var customerID, current int64
	var month time.Time
	var status string
	err = tx.QueryRow(r.Context(), "SELECT customer_id,billing_month,current_reading,status FROM pam_readings WHERE id=$1 FOR UPDATE", readingID).Scan(&customerID, &month, &current, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "pencatatan meter tidak ditemukan"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if status != "PENDING" {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "pencatatan meter sudah ditinjau"})
		return
	}
	var lockedCustomerID int64
	if err := tx.QueryRow(r.Context(), "SELECT id FROM pam_customers WHERE id=$1 FOR UPDATE", customerID).Scan(&lockedCustomerID); err != nil {
		fail(w, err)
		return
	}
	note := strings.TrimSpace(input.Note)
	if input.Decision == "reject" {
		_, err = tx.Exec(r.Context(), `UPDATE pam_readings SET status='REJECTED',reviewed_by=$2,reviewed_at=now(),review_note=$3 WHERE id=$1`, readingID, principal.UserID, note)
		if err != nil {
			fail(w, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, map[string]string{"status": "REJECTED"})
		return
	}

	var latestMonth time.Time
	err = tx.QueryRow(r.Context(), `SELECT billing_month FROM pam_readings
		WHERE customer_id=$1 AND status='APPROVED' AND fee_only=false ORDER BY billing_month DESC LIMIT 1`, customerID).Scan(&latestMonth)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fail(w, err)
		return
	}
	if err == nil && !latestMonth.Before(month) {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "sudah ada pembacaan yang disetujui untuk bulan ini atau bulan setelahnya"})
		return
	}

	var previous, rate, fee int64
	if err := tx.QueryRow(r.Context(), `SELECT COALESCE((SELECT current_reading FROM pam_readings
		WHERE customer_id=$1 AND billing_month<$2 AND status='APPROVED' AND fee_only=false ORDER BY billing_month DESC LIMIT 1),0)`,
		customerID, month).Scan(&previous); err != nil {
		fail(w, err)
		return
	}
	if current < previous {
		jsonOut(w, http.StatusConflict, map[string]any{"error": "meter yang diajukan lebih kecil dari pembacaan terakhir yang disetujui", "previous_reading": previous})
		return
	}
	if err := tx.QueryRow(r.Context(), "SELECT rate_per_m3,monthly_fee FROM pam_settings WHERE id=1 FOR SHARE").Scan(&rate, &fee); err != nil {
		fail(w, err)
		return
	}
	if rate == 0 {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "tarif PAM belum diatur administrator"})
		return
	}
	consumption := current - previous
	total, validTotal := pamBillTotal(consumption, rate, fee)
	if !validTotal {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "total tagihan melebihi batas nominal yang dapat diproses"})
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE pam_readings SET previous_reading=$2,consumption=$3,status='APPROVED',
		reviewed_by=$4,reviewed_at=now(),review_note=$5 WHERE id=$1`, readingID, previous, consumption, principal.UserID, note); err != nil {
		fail(w, err)
		return
	}
	if _, err := tx.Exec(r.Context(), "INSERT INTO pam_bills(reading_id,rate_per_m3,monthly_fee,total) VALUES($1,$2,$3,$4)", readingID, rate, fee, total); err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"status": "APPROVED", "total": total})
}

func (a *App) residentPAMReading(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok || !principal.IsResident || principal.ResidentID < 1 {
		jsonOut(w, http.StatusForbidden, map[string]string{"error": "fitur ini hanya tersedia untuk akun warga"})
		return
	}
	var input struct {
		BillingMonth   string `json:"billing_month"`
		CurrentReading int64  `json:"current_reading"`
	}
	if jsonIn(r, &input) != nil || input.CurrentReading < 0 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "bulan dan angka meter non-negatif wajib diisi"})
		return
	}
	month, err := time.Parse("2006-01", input.BillingMonth)
	if err != nil {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "bulan harus berformat YYYY-MM"})
		return
	}
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if month.After(currentMonth) {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "meter untuk bulan mendatang belum dapat dicatat"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var customerID, previous int64
	err = tx.QueryRow(r.Context(), `SELECT id,COALESCE((SELECT current_reading FROM pam_readings
		WHERE customer_id=pam_customers.id AND billing_month<$2 AND status='APPROVED' AND fee_only=false
		ORDER BY billing_month DESC LIMIT 1),0)
		FROM pam_customers WHERE resident_id=$1 AND active=true FOR UPDATE`, principal.ResidentID, month).Scan(&customerID, &previous)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "pelanggan PAM aktif untuk KK Anda tidak ditemukan"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	var latestMonth time.Time
	err = tx.QueryRow(r.Context(), `SELECT billing_month FROM pam_readings
		WHERE customer_id=$1 AND status='APPROVED' AND fee_only=false ORDER BY billing_month DESC LIMIT 1`, customerID).Scan(&latestMonth)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fail(w, err)
		return
	}
	if err == nil && !latestMonth.Before(month) {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "meter hanya dapat dicatat setelah pembacaan terakhir yang disetujui"})
		return
	}
	if input.CurrentReading < previous {
		jsonOut(w, http.StatusBadRequest, map[string]any{"error": "angka meter tidak boleh lebih kecil dari pembacaan terakhir yang disetujui", "previous_reading": previous})
		return
	}
	principalUserID := principal.UserID
	var id int64
	err = tx.QueryRow(r.Context(), `INSERT INTO pam_readings(customer_id,billing_month,previous_reading,current_reading,consumption,recorded_by,status)
		VALUES($1,$2,$3,$4,$5,$6,'PENDING')
		ON CONFLICT(customer_id,billing_month) DO UPDATE SET previous_reading=EXCLUDED.previous_reading,
		current_reading=EXCLUDED.current_reading,consumption=EXCLUDED.consumption,recorded_by=EXCLUDED.recorded_by,
		created_at=now(),status='PENDING',reviewed_by=NULL,reviewed_at=NULL,review_note=''
		WHERE pam_readings.status='REJECTED' RETURNING id`,
		customerID, month, previous, input.CurrentReading, input.CurrentReading-previous, principalUserID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "meter bulan tersebut sudah tercatat atau sedang menunggu verifikasi"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"id": id, "status": "PENDING", "billing_month": month.Format("2006-01"), "previous_reading": previous, "current_reading": input.CurrentReading})
}

func (a *App) residentPAMPaymentChoice(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok || !principal.IsResident || principal.ResidentID < 1 {
		jsonOut(w, http.StatusForbidden, map[string]string{"error": "fitur ini hanya tersedia untuk akun warga"})
		return
	}
	var input struct {
		BillID     int64  `json:"bill_id"`
		MethodCode string `json:"method_code"`
	}
	if jsonIn(r, &input) != nil || input.BillID < 1 || input.MethodCode == "" {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "tagihan dan metode pembayaran wajib dipilih"})
		return
	}
	var rawMethods []byte
	if err := a.db.QueryRow(r.Context(), "SELECT payment_methods FROM pam_settings WHERE id=1").Scan(&rawMethods); err != nil {
		fail(w, err)
		return
	}
	var methods []pamPaymentMethod
	if err := json.Unmarshal(rawMethods, &methods); err != nil {
		fail(w, err)
		return
	}
	methodActive := false
	for _, method := range methods {
		if method.Code == input.MethodCode && method.Active {
			methodActive = true
			break
		}
	}
	if !methodActive {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "metode pembayaran tidak tersedia"})
		return
	}
	var billID int64
	err := a.db.QueryRow(r.Context(), `INSERT INTO pam_payment_choices(bill_id,resident_id,method_code)
		SELECT b.id,$2,$3 FROM pam_bills b
		JOIN pam_readings pr ON pr.id=b.reading_id JOIN pam_customers c ON c.id=pr.customer_id
		WHERE b.id=$1 AND c.resident_id=$2
		AND b.total>COALESCE((SELECT SUM(amount) FROM pam_payments WHERE bill_id=b.id),0)
		ON CONFLICT(bill_id) DO UPDATE SET method_code=EXCLUDED.method_code,selected_at=now()
		WHERE pam_payment_choices.resident_id=EXCLUDED.resident_id
		RETURNING bill_id`, input.BillID, principal.ResidentID, input.MethodCode).Scan(&billID)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "tagihan belum lunas tidak ditemukan untuk KK Anda"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]string{"method_code": input.MethodCode, "message": "Pilihan pembayaran disimpan"})
}

func (a *App) residentPAMPaymentConfirmation(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok || !principal.IsResident || principal.ResidentID < 1 {
		jsonOut(w, http.StatusForbidden, map[string]string{"error": "fitur ini hanya tersedia untuk akun warga"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var input pamPaymentConfirmationInput
	if jsonIn(r, &input) != nil || input.BillID < 1 || input.Amount < 1 ||
		len(input.Note) > 500 || validatePAMImageData(input.ProofData) != nil || input.ProofData == "" {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "tagihan, nominal, dan bukti pembayaran berupa gambar PNG/JPEG/WebP maksimal 1 MB wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var total, paid int64
	if err := tx.QueryRow(r.Context(), `SELECT b.total FROM pam_bills b
		JOIN pam_readings pr ON pr.id=b.reading_id JOIN pam_customers c ON c.id=pr.customer_id
		WHERE b.id=$1 AND c.resident_id=$2 FOR UPDATE OF b`, input.BillID, principal.ResidentID).Scan(&total); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "tagihan PAM tidak ditemukan untuk KK Anda"})
		} else {
			fail(w, err)
		}
		return
	}
	if err := tx.QueryRow(r.Context(), "SELECT COALESCE(SUM(amount),0)::bigint FROM pam_payments WHERE bill_id=$1", input.BillID).Scan(&paid); err != nil {
		fail(w, err)
		return
	}
	if input.Amount > total-paid || total <= paid {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "nominal bukti melebihi atau tidak sesuai sisa tagihan"})
		return
	}
	var methodCode string
	if err := tx.QueryRow(r.Context(), `SELECT method_code FROM pam_payment_choices
		WHERE bill_id=$1 AND resident_id=$2`, input.BillID, principal.ResidentID).Scan(&methodCode); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, http.StatusConflict, map[string]string{"error": "pilih metode pembayaran sebelum mengirim bukti"})
		} else {
			fail(w, err)
		}
		return
	}
	if methodCode != input.MethodCode || methodCode == "collection" {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "metode pembayaran tidak cocok atau memerlukan konfirmasi petugas"})
		return
	}
	var hasPending bool
	if err := tx.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM pam_payment_submissions WHERE bill_id=$1 AND status='PENDING')", input.BillID).Scan(&hasPending); err != nil {
		fail(w, err)
		return
	}
	if hasPending {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "bukti pembayaran tagihan ini sedang menunggu verifikasi"})
		return
	}
	var id int64
	err = tx.QueryRow(r.Context(), `INSERT INTO pam_payment_submissions(bill_id,resident_id,amount,method_code,proof_data,note)
		VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, input.BillID, principal.ResidentID, input.Amount, methodCode, input.ProofData, strings.TrimSpace(input.Note)).Scan(&id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"id": id, "status": "PENDING", "message": "Bukti pembayaran terkirim dan menunggu verifikasi petugas"})
}

func (a *App) pamPaymentSubmissions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT s.id,s.bill_id,s.resident_id,r.house_code,r.head_name,
		pr.billing_month::text,s.amount,s.method_code,s.note,s.status,s.submitted_at::text,
		s.review_note,COALESCE(s.payment_id,0)
		FROM pam_payment_submissions s
		JOIN residents r ON r.id=s.resident_id
		JOIN pam_bills b ON b.id=s.bill_id JOIN pam_readings pr ON pr.id=b.reading_id
		ORDER BY CASE s.status WHEN 'PENDING' THEN 0 ELSE 1 END,s.submitted_at DESC,s.id DESC LIMIT 30`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID           int64  `json:"id"`
		BillID       int64  `json:"bill_id"`
		ResidentID   int64  `json:"resident_id"`
		HouseCode    string `json:"house_code"`
		HeadName     string `json:"head_name"`
		BillingMonth string `json:"billing_month"`
		Amount       int64  `json:"amount"`
		MethodCode   string `json:"method_code"`
		ProofURL     string `json:"proof_url"`
		Note         string `json:"note"`
		Status       string `json:"status"`
		SubmittedAt  string `json:"submitted_at"`
		ReviewNote   string `json:"review_note"`
		PaymentID    int64  `json:"payment_id"`
	}
	out := []item{}
	for rows.Next() {
		var submission item
		if err := rows.Scan(&submission.ID, &submission.BillID, &submission.ResidentID, &submission.HouseCode,
			&submission.HeadName, &submission.BillingMonth, &submission.Amount, &submission.MethodCode,
			&submission.Note, &submission.Status, &submission.SubmittedAt,
			&submission.ReviewNote, &submission.PaymentID); err != nil {
			fail(w, err)
			return
		}
		submission.ProofURL = fmt.Sprintf("/api/pam/payment-submissions/%d/proof", submission.ID)
		out = append(out, submission)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *App) pamPaymentSubmissionProof(w http.ResponseWriter, r *http.Request) {
	submissionID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || submissionID < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "id bukti pembayaran tidak valid"})
		return
	}
	var data string
	if err := a.db.QueryRow(r.Context(), "SELECT proof_data FROM pam_payment_submissions WHERE id=$1", submissionID).Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "bukti pembayaran tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	header, encoded, ok := strings.Cut(data, ",")
	if !ok {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "data bukti pembayaran rusak"})
		return
	}
	contentType := strings.TrimPrefix(strings.TrimSuffix(header, ";base64"), "data:")
	image, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(image) > 1<<20 || http.DetectContentType(image) != contentType {
		jsonOut(w, http.StatusInternalServerError, map[string]string{"error": "data bukti pembayaran rusak"})
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}

func (a *App) pamReviewPaymentSubmission(w http.ResponseWriter, r *http.Request) {
	submissionID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || submissionID < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "id bukti pembayaran tidak valid"})
		return
	}
	var input pamPaymentReviewInput
	if jsonIn(r, &input) != nil || (input.Decision != "approve" && input.Decision != "reject") ||
		len(input.Note) > 500 || (input.Decision == "reject" && strings.TrimSpace(input.Note) == "") {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "pilih setujui atau tolak; alasan penolakan wajib diisi dan catatan maksimal 500 karakter"})
		return
	}
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var billID, residentID, amount int64
	var methodCode, status, note string
	err = tx.QueryRow(r.Context(), `SELECT bill_id,resident_id,amount,method_code,status,note
		FROM pam_payment_submissions WHERE id=$1 FOR UPDATE`, submissionID).
		Scan(&billID, &residentID, &amount, &methodCode, &status, &note)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "bukti pembayaran tidak ditemukan"})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if status != "PENDING" {
		jsonOut(w, http.StatusConflict, map[string]string{"error": "bukti pembayaran sudah ditinjau"})
		return
	}
	reviewNote := strings.TrimSpace(input.Note)
	var paymentID int64
	if input.Decision == "approve" {
		paymentID, _, _, err = recordPAMPaymentTx(r.Context(), tx, billID, amount, methodCode, "Konfirmasi warga: "+note, principal.UserID)
		if err != nil {
			if errors.Is(err, errPAMPaymentConflict) {
				jsonOut(w, http.StatusConflict, map[string]string{"error": "nominal bukti pembayaran melebihi sisa tagihan"})
			} else {
				fail(w, err)
			}
			return
		}
		_, err = tx.Exec(r.Context(), `UPDATE pam_payment_submissions SET status='APPROVED',reviewed_by=$2,
			reviewed_at=now(),review_note=$3,payment_id=$4 WHERE id=$1`, submissionID, principal.UserID, reviewNote, paymentID)
	} else {
		_, err = tx.Exec(r.Context(), `UPDATE pam_payment_submissions SET status='REJECTED',reviewed_by=$2,
			reviewed_at=now(),review_note=$3 WHERE id=$1`, submissionID, principal.UserID, reviewNote)
	}
	if err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"status": strings.ToUpper(input.Decision), "payment_id": paymentID})
}

func (a *App) pamCommissions(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT c.bill_id,r.house_code,r.head_name,pr.billing_month::text,
		c.bill_total,c.rate_basis_points,c.amount,c.earned_at::text,u.full_name
		FROM pam_commissions c JOIN pam_bills b ON b.id=c.bill_id JOIN pam_readings pr ON pr.id=b.reading_id
		JOIN pam_customers pc ON pc.id=pr.customer_id JOIN residents r ON r.id=pc.resident_id
		JOIN app_users u ON u.id=c.staff_user_id ORDER BY c.earned_at DESC,c.bill_id DESC LIMIT 500`)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		BillID       int64  `json:"bill_id"`
		HouseCode    string `json:"house_code"`
		HeadName     string `json:"head_name"`
		BillingMonth string `json:"billing_month"`
		BillTotal    int64  `json:"bill_total"`
		RateBP       int64  `json:"rate_basis_points"`
		Amount       int64  `json:"amount"`
		EarnedAt     string `json:"earned_at"`
		StaffName    string `json:"staff_name"`
	}
	out := []item{}
	for rows.Next() {
		var commission item
		if err := rows.Scan(&commission.BillID, &commission.HouseCode, &commission.HeadName, &commission.BillingMonth,
			&commission.BillTotal, &commission.RateBP, &commission.Amount, &commission.EarnedAt, &commission.StaffName); err != nil {
			fail(w, err)
			return
		}
		out = append(out, commission)
	}
	if err := rows.Err(); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusOK, out)
}

var errPAMPaymentConflict = errors.New("pembayaran PAM melebihi sisa tagihan")

func recordPAMPaymentTx(ctx context.Context, tx pgx.Tx, billID, amount int64, methodCode, note string, staffID int64) (int64, int64, int64, error) {
	var total int64
	if err := tx.QueryRow(ctx, "SELECT total FROM pam_bills WHERE id=$1 FOR UPDATE", billID).Scan(&total); err != nil {
		return 0, 0, 0, err
	}
	var paid int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(SUM(amount),0)::bigint FROM pam_payments WHERE bill_id=$1", billID).Scan(&paid); err != nil {
		return 0, 0, 0, err
	}
	if amount < 1 || amount > total-paid {
		return 0, total - paid, 0, errPAMPaymentConflict
	}
	var paymentID int64
	if err := tx.QueryRow(ctx, `INSERT INTO pam_payments(bill_id,amount,note,received_by,method_code)
		VALUES($1,$2,$3,$4,$5) RETURNING id`, billID, amount, note, staffID, methodCode).Scan(&paymentID); err != nil {
		return 0, 0, 0, err
	}
	remaining := total - paid - amount
	var commission int64
	if remaining == 0 {
		if err := tx.QueryRow(ctx, `INSERT INTO pam_commissions(bill_id,staff_user_id,bill_total,rate_basis_points,amount)
			SELECT $1,$2,$3::bigint,commission_basis_points,
				round(($3::bigint)::numeric*commission_basis_points/10000)::bigint
			FROM pam_settings WHERE id=1
			ON CONFLICT(bill_id) DO NOTHING RETURNING amount`, billID, staffID, total).Scan(&commission); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, 0, err
		}
	}
	return paymentID, remaining, commission, nil
}

func (a *App) pamBills(w http.ResponseWriter, r *http.Request) {
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	query := `SELECT b.id,c.id,r.house_code,r.head_name,c.meter_number,pr.billing_month::text,pr.previous_reading,pr.current_reading,pr.consumption,b.rate_per_m3,b.monthly_fee,b.total,COALESCE(SUM(p.amount),0)::bigint,COALESCE(choice.method_code,''),pr.fee_only
		FROM pam_bills b JOIN pam_readings pr ON pr.id=b.reading_id JOIN pam_customers c ON c.id=pr.customer_id JOIN residents r ON r.id=c.resident_id LEFT JOIN pam_payments p ON p.bill_id=b.id
		LEFT JOIN pam_payment_choices choice ON choice.bill_id=b.id AND choice.resident_id=c.resident_id`
	args := []any{}
	if month != "" {
		query += " WHERE to_char(pr.billing_month,'YYYY-MM')=$1"
		args = append(args, month)
	}
	query += " GROUP BY b.id,c.id,r.id,pr.id,choice.method_code ORDER BY pr.billing_month DESC,r.house_code"
	rows, err := a.db.Query(r.Context(), query, args...)
	if err != nil {
		fail(w, err)
		return
	}
	defer rows.Close()
	type item struct {
		ID              int64  `json:"id"`
		CustomerID      int64  `json:"customer_id"`
		HouseCode       string `json:"house_code"`
		HeadName        string `json:"head_name"`
		MeterNumber     string `json:"meter_number"`
		BillingMonth    string `json:"billing_month"`
		PreviousReading int64  `json:"previous_reading"`
		CurrentReading  int64  `json:"current_reading"`
		Consumption     int64  `json:"consumption"`
		RatePerM3       int64  `json:"rate_per_m3"`
		MonthlyFee      int64  `json:"monthly_fee"`
		Total           int64  `json:"total"`
		Paid            int64  `json:"paid"`
		Remaining       int64  `json:"remaining"`
		PaymentMethod   string `json:"payment_method"`
		FeeOnly         bool   `json:"fee_only"`
	}
	out := []item{}
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.ID, &x.CustomerID, &x.HouseCode, &x.HeadName, &x.MeterNumber, &x.BillingMonth, &x.PreviousReading, &x.CurrentReading, &x.Consumption, &x.RatePerM3, &x.MonthlyFee, &x.Total, &x.Paid, &x.PaymentMethod, &x.FeeOnly); err != nil {
			fail(w, err)
			return
		}
		x.Remaining = x.Total - x.Paid
		out = append(out, x)
	}
	jsonOut(w, http.StatusOK, out)
}

func (a *App) pamPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rows, err := a.db.Query(r.Context(), `SELECT p.id,p.bill_id,r.house_code,r.head_name,pr.billing_month::text,
			p.amount,p.method_code,p.note,p.received_at::text,COALESCE(u.full_name,'Petugas tidak diketahui'),EXISTS(SELECT 1 FROM pam_commissions pc WHERE pc.bill_id=b.id)
			FROM pam_payments p JOIN pam_bills b ON b.id=p.bill_id JOIN pam_readings pr ON pr.id=b.reading_id
			JOIN pam_customers c ON c.id=pr.customer_id JOIN residents r ON r.id=c.resident_id
			LEFT JOIN app_users u ON u.id=p.received_by ORDER BY p.received_at DESC,p.id DESC LIMIT 500`)
		if err != nil {
			fail(w, err)
			return
		}
		defer rows.Close()
		type item struct {
			ID           int64  `json:"id"`
			BillID       int64  `json:"bill_id"`
			HouseCode    string `json:"house_code"`
			HeadName     string `json:"head_name"`
			BillingMonth string `json:"billing_month"`
			Amount       int64  `json:"amount"`
			MethodCode   string `json:"method_code"`
			Note         string `json:"note"`
			ReceivedAt   string `json:"received_at"`
			StaffName    string `json:"staff_name"`
			Protected    bool   `json:"protected"`
		}
		out := []item{}
		for rows.Next() {
			var payment item
			if err := rows.Scan(&payment.ID, &payment.BillID, &payment.HouseCode, &payment.HeadName,
				&payment.BillingMonth, &payment.Amount, &payment.MethodCode, &payment.Note,
				&payment.ReceivedAt, &payment.StaffName, &payment.Protected); err != nil {
				fail(w, err)
				return
			}
			out = append(out, payment)
		}
		if err := rows.Err(); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, http.StatusOK, out)
		return
	}
	var input pamPaymentInput
	if jsonIn(r, &input) != nil || input.BillID < 1 || input.Amount < 1 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "bill_id dan nominal positif wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var methodCode string
	if input.MethodCode != "" {
		methodCode = input.MethodCode
	} else if err := tx.QueryRow(r.Context(), "SELECT COALESCE((SELECT method_code FROM pam_payment_choices WHERE bill_id=$1),'collection')", input.BillID).Scan(&methodCode); err != nil {
		fail(w, err)
		return
	}
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	id, remaining, commission, err := recordPAMPaymentTx(r.Context(), tx, input.BillID, input.Amount, methodCode, strings.TrimSpace(input.Note), principal.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		jsonOut(w, http.StatusNotFound, map[string]string{"error": "tagihan PAM tidak ditemukan"})
		return
	}
	if errors.Is(err, errPAMPaymentConflict) {
		jsonOut(w, http.StatusConflict, map[string]any{"error": "pembayaran melebihi sisa tagihan", "remaining": remaining})
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"id": id, "remaining": remaining, "commission": commission})
}

func (a *App) pamPaymentRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonOut(w, 400, map[string]string{"error": "ID penerimaan tidak valid"})
		return
	}
	var input struct {
		Amount     int64  `json:"amount"`
		Note       string `json:"note"`
		MethodCode string `json:"method_code"`
	}
	if r.Method == http.MethodPut && (jsonIn(r, &input) != nil || input.Amount < 1) {
		jsonOut(w, 400, map[string]string{"error": "nominal positif wajib diisi"})
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var billID, oldAmount, receivedBy int64
	var oldMethod string
	if err = tx.QueryRow(r.Context(), `SELECT bill_id,amount,COALESCE(received_by,0),method_code FROM pam_payments WHERE id=$1 FOR UPDATE`, id).Scan(&billID, &oldAmount, &receivedBy, &oldMethod); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, 404, map[string]string{"error": "penerimaan PAM tidak ditemukan"})
		} else {
			fail(w, err)
		}
		return
	}
	var total int64
	if err = tx.QueryRow(r.Context(), `SELECT total FROM pam_bills WHERE id=$1 FOR UPDATE`, billID).Scan(&total); err != nil {
		fail(w, err)
		return
	}
	var paid int64
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount),0)::bigint FROM pam_payments WHERE bill_id=$1`, billID).Scan(&paid); err != nil {
		fail(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		if _, err = tx.Exec(r.Context(), `UPDATE pam_payment_submissions SET payment_id=NULL,review_note=concat_ws(' · ',NULLIF(review_note,''),'Penerimaan terkait dihapus oleh pengurus') WHERE payment_id=$1`, id); err != nil {
			fail(w, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `DELETE FROM pam_payments WHERE id=$1`, id); err != nil {
			fail(w, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `DELETE FROM pam_commissions WHERE bill_id=$1`, billID); err != nil {
			fail(w, err)
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			fail(w, err)
			return
		}
		jsonOut(w, 200, map[string]string{"message": "Penerimaan PAM dihapus"})
		return
	}
	if input.Amount > total-(paid-oldAmount) {
		jsonOut(w, 409, map[string]any{"error": "nominal melebihi sisa tagihan", "remaining": max(total-(paid-oldAmount), 0)})
		return
	}
	method := strings.TrimSpace(input.MethodCode)
	if method == "" {
		method = oldMethod
	}
	if _, err = tx.Exec(r.Context(), `UPDATE pam_payments SET amount=$1,note=$2,method_code=$3 WHERE id=$4`, input.Amount, strings.TrimSpace(input.Note), method, id); err != nil {
		fail(w, err)
		return
	}
	if paid-oldAmount+input.Amount == total {
		if receivedBy == 0 {
			principal, _ := r.Context().Value(authContextKey{}).(AuthPrincipal)
			receivedBy = principal.UserID
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO pam_commissions(bill_id,staff_user_id,bill_total,rate_basis_points,amount)
			SELECT $1,$2,$3::bigint,commission_basis_points,round(($3::bigint)::numeric*commission_basis_points/10000)::bigint FROM pam_settings WHERE id=1
			ON CONFLICT(bill_id) DO UPDATE SET staff_user_id=EXCLUDED.staff_user_id,bill_total=EXCLUDED.bill_total,
			rate_basis_points=EXCLUDED.rate_basis_points,amount=EXCLUDED.amount,earned_at=now()`, billID, receivedBy, total); err != nil {
			fail(w, err)
			return
		}
	} else {
		if _, err = tx.Exec(r.Context(), `DELETE FROM pam_commissions WHERE bill_id=$1`, billID); err != nil {
			fail(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id, "message": "Penerimaan PAM diperbarui"})
}

func (a *App) pamCollectivePayment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		BillIDs []int64 `json:"bill_ids"`
	}
	if jsonIn(r, &input) != nil || len(input.BillIDs) == 0 || len(input.BillIDs) > 100 {
		jsonOut(w, http.StatusBadRequest, map[string]string{"error": "pilih 1 sampai 100 tagihan untuk pelunasan kolektif"})
		return
	}
	seen := make(map[int64]bool, len(input.BillIDs))
	for _, billID := range input.BillIDs {
		if billID < 1 || seen[billID] {
			jsonOut(w, http.StatusBadRequest, map[string]string{"error": "daftar tagihan tidak valid atau mengandung duplikat"})
			return
		}
		seen[billID] = true
	}
	sort.Slice(input.BillIDs, func(i, j int) bool { return input.BillIDs[i] < input.BillIDs[j] })
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	principal := r.Context().Value(authContextKey{}).(AuthPrincipal)
	var totalPaid, totalCommission int64
	for _, billID := range input.BillIDs {
		var due int64
		var billTotal int64
		err := tx.QueryRow(r.Context(), "SELECT total FROM pam_bills WHERE id=$1 FOR UPDATE", billID).Scan(&billTotal)
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, http.StatusNotFound, map[string]any{"error": "salah satu tagihan tidak ditemukan", "bill_id": billID})
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		if err := tx.QueryRow(r.Context(), "SELECT COALESCE(SUM(amount),0)::bigint FROM pam_payments WHERE bill_id=$1", billID).Scan(&due); err != nil {
			fail(w, err)
			return
		}
		due = billTotal - due
		if due < 1 {
			jsonOut(w, http.StatusConflict, map[string]any{"error": "salah satu tagihan sudah lunas", "bill_id": billID})
			return
		}
		var methodCode string
		if err := tx.QueryRow(r.Context(), "SELECT COALESCE((SELECT method_code FROM pam_payment_choices WHERE bill_id=$1),'collection')", billID).Scan(&methodCode); err != nil {
			fail(w, err)
			return
		}
		_, _, commission, err := recordPAMPaymentTx(r.Context(), tx, billID, due, methodCode, "Pembayaran kolektif PAM", principal.UserID)
		if err != nil {
			fail(w, err)
			return
		}
		totalPaid += due
		totalCommission += commission
	}
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"bill_count": len(input.BillIDs), "total_paid": totalPaid, "commission": totalCommission})
}
