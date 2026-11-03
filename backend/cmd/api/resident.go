package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type residentDashboardData struct {
	Resident          residentProfile          `json:"resident"`
	Bills             []residentBill           `json:"bills"`
	BillTotal         int64                    `json:"bill_total"`
	BillDue           int64                    `json:"bill_due"`
	PAMBills          []residentPAMBill        `json:"pam_bills"`
	PAMPayments       []residentPAMPayment     `json:"pam_payment_history"`
	PAMDue            int64                    `json:"pam_due"`
	PAMCustomer       *residentPAMCustomer     `json:"pam_customer,omitempty"`
	PAMSubmission     *residentPAMSubmission   `json:"pam_submission,omitempty"`
	PAMPaymentMethods []pamPaymentMethod       `json:"pam_payment_methods"`
	SavingsBalance    int64                    `json:"savings_balance"`
	SavingsDue        int64                    `json:"savings_due"`
	ActiveStage       int                      `json:"active_savings_stage"`
	SavingsStages     []residentSavingsStage   `json:"savings_stages"`
	Payments          []residentPayment        `json:"payments"`
	UpcomingMeetings  []residentMeeting        `json:"upcoming_meetings"`
	RondaSchedule     []scheduleDay            `json:"ronda_schedule"`
	Jimpitan          []residentJimpitanRecord `json:"jimpitan"`
}

type residentProfile struct {
	ID           int64  `json:"id"`
	HouseCode    string `json:"house_code"`
	HeadName     string `json:"head_name"`
	Address      string `json:"address"`
	SavingsClass int    `json:"savings_class"`
}

type residentBill struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	MeetingDate string `json:"meeting_date"`
	Amount      int64  `json:"amount"`
	Paid        int64  `json:"paid"`
	Remaining   int64  `json:"remaining"`
}

type residentPAMBill struct {
	ID                int64  `json:"id"`
	BillingMonth      string `json:"billing_month"`
	MeterNumber       string `json:"meter_number"`
	PreviousReading   int64  `json:"previous_reading"`
	CurrentReading    int64  `json:"current_reading"`
	Consumption       int64  `json:"consumption"`
	Total             int64  `json:"total"`
	Paid              int64  `json:"paid"`
	Remaining         int64  `json:"remaining"`
	PaymentMethod     string `json:"payment_method"`
	PaymentStatus     string `json:"payment_status"`
	PaymentReviewNote string `json:"payment_review_note"`
}

type residentPAMPayment struct {
	ID           int64  `json:"id"`
	BillID       int64  `json:"bill_id"`
	BillingMonth string `json:"billing_month"`
	Amount       int64  `json:"amount"`
	MethodCode   string `json:"method_code"`
	Note         string `json:"note"`
	ReceivedAt   string `json:"received_at"`
}

type residentPAMCustomer struct {
	MeterNumber      string `json:"meter_number"`
	LastReading      int64  `json:"last_reading"`
	LastBillingMonth string `json:"last_billing_month"`
}

type residentPAMSubmission struct {
	BillingMonth   string `json:"billing_month"`
	CurrentReading int64  `json:"current_reading"`
	Status         string `json:"status"`
	ReviewNote     string `json:"review_note"`
}

type residentSavingsStage struct {
	Stage     int   `json:"stage"`
	Tariff    int64 `json:"tariff"`
	Deposited int64 `json:"deposited"`
	Remaining int64 `json:"remaining"`
}

type residentPayment struct {
	ID     int64  `json:"id"`
	Amount int64  `json:"amount"`
	Note   string `json:"note"`
	Date   string `json:"date"`
}

type residentMeeting struct {
	Date string `json:"date"`
	Time string `json:"time"`
}

type residentJimpitanRecord struct {
	Date   string `json:"date"`
	Amount int64  `json:"amount"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (a *App) residentDashboard(w http.ResponseWriter, r *http.Request) {
	principal, ok := r.Context().Value(authContextKey{}).(AuthPrincipal)
	if !ok || !principal.IsResident || principal.ResidentID < 1 {
		jsonOut(w, http.StatusForbidden, map[string]string{"error": "dashboard hanya tersedia untuk akun Warga yang ditautkan ke KK"})
		return
	}

	ctx := r.Context()
	data := residentDashboardData{
		Bills:             []residentBill{},
		PAMBills:          []residentPAMBill{},
		PAMPayments:       []residentPAMPayment{},
		PAMPaymentMethods: []pamPaymentMethod{},
		SavingsStages:     []residentSavingsStage{},
		Payments:          []residentPayment{},
		UpcomingMeetings:  []residentMeeting{},
		RondaSchedule:     make([]scheduleDay, len(weekdayNames)),
		Jimpitan:          []residentJimpitanRecord{},
	}
	if err := a.db.QueryRow(ctx, `SELECT id,house_code,head_name,address,savings_class
		FROM residents WHERE id=$1 AND active=true`, principal.ResidentID).
		Scan(&data.Resident.ID, &data.Resident.HouseCode, &data.Resident.HeadName, &data.Resident.Address, &data.Resident.SavingsClass); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonOut(w, http.StatusNotFound, map[string]string{"error": "data KK akun ini tidak ditemukan atau tidak aktif"})
		} else {
			fail(w, err)
		}
		return
	}

	rows, err := a.db.Query(ctx, `SELECT b.id,b.description,p.meeting_date::text,b.amount,
		COALESCE(SUM(pa.amount),0)::bigint,(b.amount-COALESCE(SUM(pa.amount),0))::bigint
		FROM bills b JOIN periods p ON p.id=b.period_id
		LEFT JOIN payment_allocations pa ON pa.bill_id=b.id
		WHERE b.resident_id=$1 AND b.waived=false GROUP BY b.id,p.meeting_date ORDER BY p.meeting_date DESC,b.id DESC`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var bill residentBill
		if err := rows.Scan(&bill.ID, &bill.Description, &bill.MeetingDate, &bill.Amount, &bill.Paid, &bill.Remaining); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.BillTotal += bill.Amount
		data.BillDue += bill.Remaining
		data.Bills = append(data.Bills, bill)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	var rawPaymentMethods []byte
	if err := a.db.QueryRow(ctx, "SELECT payment_methods FROM pam_settings WHERE id=1").Scan(&rawPaymentMethods); err != nil {
		fail(w, err)
		return
	}
	if err := json.Unmarshal(rawPaymentMethods, &data.PAMPaymentMethods); err != nil {
		fail(w, err)
		return
	}
	rows, err = a.db.Query(ctx, `SELECT c.meter_number,
		COALESCE(last.current_reading,0),COALESCE(last.billing_month::text,'')
		FROM pam_customers c
		LEFT JOIN LATERAL (SELECT current_reading,billing_month FROM pam_readings
			WHERE customer_id=c.id AND status='APPROVED' ORDER BY billing_month DESC LIMIT 1) last ON true
		WHERE c.resident_id=$1 AND c.active=true`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	if rows.Next() {
		var customer residentPAMCustomer
		if err := rows.Scan(&customer.MeterNumber, &customer.LastReading, &customer.LastBillingMonth); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.PAMCustomer = &customer
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()
	if data.PAMCustomer != nil {
		var submission residentPAMSubmission
		err := a.db.QueryRow(ctx, `SELECT billing_month::text,current_reading,status,review_note
			FROM pam_readings pr JOIN pam_customers c ON c.id=pr.customer_id
			WHERE c.resident_id=$1 AND c.active=true AND pr.status<>'APPROVED'
			AND NOT EXISTS (SELECT 1 FROM pam_readings approved
				WHERE approved.customer_id=pr.customer_id AND approved.status='APPROVED'
				AND approved.billing_month>pr.billing_month)
			ORDER BY pr.created_at DESC,pr.id DESC LIMIT 1`, principal.ResidentID).
			Scan(&submission.BillingMonth, &submission.CurrentReading, &submission.Status, &submission.ReviewNote)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			fail(w, err)
			return
		}
		if err == nil {
			data.PAMSubmission = &submission
		}
	}

	rows, err = a.db.Query(ctx, `SELECT b.id,pr.billing_month::text,c.meter_number,
		pr.previous_reading,pr.current_reading,pr.consumption,b.total,
		COALESCE(SUM(pay.amount),0)::bigint,(b.total-COALESCE(SUM(pay.amount),0))::bigint,
		COALESCE(choice.method_code,''),COALESCE(submission.status,''),COALESCE(submission.review_note,'')
		FROM pam_customers c
		JOIN pam_readings pr ON pr.customer_id=c.id
		JOIN pam_bills b ON b.reading_id=pr.id
		LEFT JOIN pam_payments pay ON pay.bill_id=b.id
		LEFT JOIN pam_payment_choices choice ON choice.bill_id=b.id AND choice.resident_id=c.resident_id
		LEFT JOIN LATERAL (SELECT status,review_note FROM pam_payment_submissions
			WHERE bill_id=b.id ORDER BY submitted_at DESC,id DESC LIMIT 1) submission ON true
		WHERE c.resident_id=$1 AND pr.status='APPROVED'
		GROUP BY b.id,pr.id,c.id,choice.method_code,submission.status,submission.review_note
		ORDER BY pr.billing_month DESC,b.id DESC`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var bill residentPAMBill
		if err := rows.Scan(&bill.ID, &bill.BillingMonth, &bill.MeterNumber, &bill.PreviousReading,
			&bill.CurrentReading, &bill.Consumption, &bill.Total, &bill.Paid, &bill.Remaining,
			&bill.PaymentMethod, &bill.PaymentStatus, &bill.PaymentReviewNote); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.PAMDue += bill.Remaining
		data.PAMBills = append(data.PAMBills, bill)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	rows, err = a.db.Query(ctx, `SELECT pay.id,b.id,pr.billing_month::text,pay.amount,pay.method_code,pay.note,pay.received_at::text
		FROM pam_payments pay
		JOIN pam_bills b ON b.id=pay.bill_id
		JOIN pam_readings pr ON pr.id=b.reading_id
		JOIN pam_customers c ON c.id=pr.customer_id
		WHERE c.resident_id=$1
		ORDER BY pay.received_at DESC,pay.id DESC LIMIT 50`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var payment residentPAMPayment
		if err := rows.Scan(&payment.ID, &payment.BillID, &payment.BillingMonth, &payment.Amount, &payment.MethodCode, &payment.Note, &payment.ReceivedAt); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.PAMPayments = append(data.PAMPayments, payment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	var latestMeeting time.Time
	if err := a.db.QueryRow(ctx, `SELECT MAX(meeting_date) FROM periods WHERE note='Jadwal Ahad Pon otomatis' AND status='TERLAKSANA'`).Scan(&latestMeeting); err != nil {
		fail(w, err)
		return
	}
	if !latestMeeting.IsZero() {
		data.ActiveStage, err = savingsStageForMeeting(ctx, a.db, latestMeeting)
		if err != nil {
			fail(w, err)
			return
		}
	}
	if err := a.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount),0)::bigint FROM tabungan_transactions WHERE resident_id=$1`, principal.ResidentID).Scan(&data.SavingsBalance); err != nil {
		fail(w, err)
		return
	}
	rows, err = a.db.Query(ctx, `SELECT b.stage_number,b.amount,COALESCE(SUM(t.amount) FILTER (WHERE t.type='SETOR'),0)::bigint
		FROM savings_stage_bills b LEFT JOIN tabungan_transactions t ON t.resident_id=b.resident_id AND t.stage_number=b.stage_number
		WHERE b.resident_id=$1 GROUP BY b.stage_number,b.amount ORDER BY b.stage_number DESC`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	type stageBalance struct {
		tariff    int64
		deposited int64
	}
	stageDeposits := make(map[int]stageBalance)
	for rows.Next() {
		var stage int
		var balance stageBalance
		if err := rows.Scan(&stage, &balance.tariff, &balance.deposited); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		stageDeposits[stage] = balance
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()
	for stage := data.ActiveStage; stage > 0; stage-- {
		balance, hasBill := stageDeposits[stage]
		if !hasBill {
			continue
		}
		remaining := max(balance.tariff-balance.deposited, 0)
		data.SavingsStages = append(data.SavingsStages, residentSavingsStage{Stage: stage, Tariff: balance.tariff, Deposited: balance.deposited, Remaining: remaining})
		if stage == data.ActiveStage {
			data.SavingsDue = remaining
		}
	}

	rows, err = a.db.Query(ctx, `SELECT id,amount,note,timezone('Asia/Jakarta',received_at)::date::text
		FROM payments WHERE resident_id=$1 ORDER BY received_at DESC,id DESC LIMIT 10`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var payment residentPayment
		if err := rows.Scan(&payment.ID, &payment.Amount, &payment.Note, &payment.Date); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.Payments = append(data.Payments, payment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	rows, err = a.db.Query(ctx, `SELECT meeting_date::text,meeting_time::text FROM periods
		WHERE status='TERJADWAL' AND meeting_date>=timezone('Asia/Jakarta',now())::date
		ORDER BY meeting_date LIMIT 5`)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var meeting residentMeeting
		if err := rows.Scan(&meeting.Date, &meeting.Time); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.UpcomingMeetings = append(data.UpcomingMeetings, meeting)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	for i, day := range weekdayNames {
		data.RondaSchedule[i] = scheduleDay{DayOfWeek: i + 1, Day: day, Officers: []string{}}
	}
	rows, err = a.db.Query(ctx, "SELECT day_of_week,officer_name FROM ronda_schedule ORDER BY day_of_week,officer_name")
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var day int
		var officer string
		if err := rows.Scan(&day, &officer); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.RondaSchedule[day-1].Officers = append(data.RondaSchedule[day-1].Officers, officer)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	rows, err = a.db.Query(ctx, `SELECT session_date::text,amount,status,note FROM jimpitan_transactions
		WHERE resident_id=$1 ORDER BY session_date DESC LIMIT 5`, principal.ResidentID)
	if err != nil {
		fail(w, err)
		return
	}
	for rows.Next() {
		var entry residentJimpitanRecord
		if err := rows.Scan(&entry.Date, &entry.Amount, &entry.Status, &entry.Note); err != nil {
			rows.Close()
			fail(w, err)
			return
		}
		data.Jimpitan = append(data.Jimpitan, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		fail(w, err)
		return
	}
	rows.Close()

	jsonOut(w, http.StatusOK, data)
}
