CREATE TABLE IF NOT EXISTS residents (
 id BIGSERIAL PRIMARY KEY, house_code TEXT NOT NULL UNIQUE, head_name TEXT NOT NULL,
 address TEXT NOT NULL DEFAULT '', qr_token TEXT NOT NULL UNIQUE, phone TEXT NOT NULL DEFAULT '',
 savings_class INTEGER NOT NULL DEFAULT 3 CHECK(savings_class BETWEEN 1 AND 4), dues_required BOOLEAN NOT NULL DEFAULT TRUE, active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS periods (
 id BIGSERIAL PRIMARY KEY, start_date DATE NOT NULL, meeting_date DATE NOT NULL UNIQUE,
 meeting_time TIME NOT NULL DEFAULT '19:30',
 status TEXT NOT NULL CHECK(status IN ('TERJADWAL','TERLAKSANA','LIBUR')), note TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS meeting_finances (
 period_id BIGINT PRIMARY KEY REFERENCES periods(id) ON DELETE CASCADE,
 payment_count INTEGER NOT NULL DEFAULT 0,
 collected_amount BIGINT NOT NULL DEFAULT 0,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS meeting_finance_receipts (
 period_id BIGINT NOT NULL REFERENCES periods(id) ON DELETE CASCADE,
 allocation_key TEXT NOT NULL, label TEXT NOT NULL, responsible_field TEXT NOT NULL,
 amount BIGINT NOT NULL CHECK(amount>=0), confirmed_by BIGINT,
 confirmed_at TIMESTAMPTZ, note TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(period_id,allocation_key)
);
CREATE TABLE IF NOT EXISTS daily_finance_snapshots (
 receipt_date DATE PRIMARY KEY, payment_count INTEGER NOT NULL, collected_amount BIGINT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS daily_finance_allocations (
 receipt_date DATE NOT NULL REFERENCES daily_finance_snapshots(receipt_date) ON DELETE CASCADE,
 allocation_key TEXT NOT NULL, label TEXT NOT NULL, responsible_field TEXT NOT NULL, amount BIGINT NOT NULL CHECK(amount>=0),
 confirmed_by BIGINT, confirmed_at TIMESTAMPTZ, note TEXT NOT NULL DEFAULT '', PRIMARY KEY(receipt_date,allocation_key)
);
CREATE TABLE IF NOT EXISTS cashbook_transactions (
 id BIGSERIAL PRIMARY KEY, field_key TEXT NOT NULL, field_label TEXT NOT NULL DEFAULT '', direction TEXT NOT NULL CHECK(direction IN ('MASUK','KELUAR')), transaction_date DATE NOT NULL,
 amount BIGINT NOT NULL CHECK(amount>0), note TEXT NOT NULL DEFAULT '', recorded_by BIGINT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cashbook_transactions_date ON cashbook_transactions(transaction_date DESC,id DESC);
CREATE TABLE IF NOT EXISTS bills (
 id BIGSERIAL PRIMARY KEY, resident_id BIGINT NOT NULL REFERENCES residents(id), period_id BIGINT NOT NULL REFERENCES periods(id),
 description TEXT NOT NULL, amount BIGINT NOT NULL CHECK(amount>=0), waived BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(resident_id,period_id,description)
);
CREATE TABLE IF NOT EXISTS payments (
 id BIGSERIAL PRIMARY KEY, resident_id BIGINT NOT NULL REFERENCES residents(id), amount BIGINT NOT NULL CHECK(amount>0),
 note TEXT NOT NULL DEFAULT '', received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS payment_allocations (
 id BIGSERIAL PRIMARY KEY, payment_id BIGINT NOT NULL REFERENCES payments(id), bill_id BIGINT NOT NULL REFERENCES bills(id),
 amount BIGINT NOT NULL CHECK(amount>0), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jimpitan_transactions (
 id BIGSERIAL PRIMARY KEY, resident_id BIGINT NOT NULL REFERENCES residents(id), session_date DATE NOT NULL,
 amount BIGINT NOT NULL CHECK(amount>=0), status TEXT NOT NULL CHECK(status IN ('TERAMBIL','KOSONG','TIDAK_DAPAT_DIAMBIL')),
 note TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(resident_id,session_date)
);
CREATE TABLE IF NOT EXISTS tabungan_transactions (
 id BIGSERIAL PRIMARY KEY, group_name TEXT NOT NULL, member_name TEXT NOT NULL,
 amount BIGINT NOT NULL CHECK(amount<>0), type TEXT NOT NULL CHECK(type IN ('SETOR','TARIK')),
 resident_id BIGINT REFERENCES residents(id), period_id BIGINT REFERENCES periods(id), stage_number INTEGER,
 note TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_bills_resident_period ON bills(resident_id,period_id);
CREATE INDEX IF NOT EXISTS idx_jimpitan_session ON jimpitan_transactions(session_date);
CREATE TABLE IF NOT EXISTS jimpitan_handovers (
 id BIGSERIAL PRIMARY KEY, session_date DATE NOT NULL UNIQUE,
 total_amount BIGINT NOT NULL CHECK(total_amount>=0),
 coordinator_one TEXT NOT NULL, coordinator_two TEXT NOT NULL,
 note TEXT NOT NULL DEFAULT '', handed_over_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS ronda_schedule (
 day_of_week SMALLINT NOT NULL CHECK(day_of_week BETWEEN 1 AND 7),
 officer_name TEXT NOT NULL,
 PRIMARY KEY(day_of_week,officer_name)
);
CREATE INDEX IF NOT EXISTS idx_tabungan_resident_stage ON tabungan_transactions(resident_id,stage_number);
CREATE TABLE IF NOT EXISTS organizational_fields (
 id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS positions (
 id BIGSERIAL PRIMARY KEY, field_id BIGINT REFERENCES organizational_fields(id) ON DELETE SET NULL,
 name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS position_permissions (
 position_id BIGINT NOT NULL REFERENCES positions(id) ON DELETE CASCADE, menu_key TEXT NOT NULL,
 can_view BOOLEAN NOT NULL DEFAULT FALSE, can_manage BOOLEAN NOT NULL DEFAULT FALSE, PRIMARY KEY(position_id,menu_key)
);
CREATE TABLE IF NOT EXISTS app_users (
 id BIGSERIAL PRIMARY KEY, username TEXT NOT NULL UNIQUE, full_name TEXT NOT NULL, password_hash TEXT NOT NULL,
 position_id BIGINT NOT NULL REFERENCES positions(id), resident_id BIGINT REFERENCES residents(id) ON DELETE SET NULL,
 active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS auth_sessions (
 token_hash TEXT PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS ronda_attendance (
 session_date DATE NOT NULL, officer_name TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('HADIR','ALPA')),
 note TEXT NOT NULL DEFAULT '', recorded_by BIGINT REFERENCES app_users(id) ON DELETE SET NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(session_date,officer_name)
);
CREATE TABLE IF NOT EXISTS ronda_schedule_audit (
 id BIGSERIAL PRIMARY KEY, changed_by BIGINT REFERENCES app_users(id) ON DELETE SET NULL,
 before_schedule JSONB NOT NULL, after_schedule JSONB NOT NULL,
 changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expiry ON auth_sessions(expires_at);
CREATE TABLE IF NOT EXISTS pam_settings (
 id SMALLINT PRIMARY KEY CHECK(id=1), rate_per_m3 BIGINT NOT NULL DEFAULT 0 CHECK(rate_per_m3>=0),
 monthly_fee BIGINT NOT NULL DEFAULT 0 CHECK(monthly_fee>=0), updated_by BIGINT REFERENCES app_users(id), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 payment_methods JSONB NOT NULL DEFAULT '[{"code":"collection","name":"Diambil petugas","instructions":"Petugas akan menghubungi Anda untuk mengambil pembayaran.","active":true},{"code":"bank_transfer","name":"Transfer Bank","instructions":"Hubungi pengurus untuk mendapatkan informasi rekening resmi.","active":true},{"code":"qris","name":"QRIS","instructions":"Hubungi pengurus untuk mendapatkan kode QRIS resmi.","active":true}]'::jsonb,
 commission_basis_points INT NOT NULL DEFAULT 0 CHECK(commission_basis_points BETWEEN 0 AND 10000)
);
INSERT INTO pam_settings(id) VALUES(1) ON CONFLICT(id) DO NOTHING;
ALTER TABLE pam_settings ADD COLUMN IF NOT EXISTS payment_methods JSONB NOT NULL DEFAULT '[{"code":"collection","name":"Diambil petugas","instructions":"Petugas akan menghubungi Anda untuk mengambil pembayaran.","active":true},{"code":"bank_transfer","name":"Transfer Bank","instructions":"Hubungi pengurus untuk mendapatkan informasi rekening resmi.","active":true},{"code":"qris","name":"QRIS","instructions":"Hubungi pengurus untuk mendapatkan kode QRIS resmi.","active":true}]'::jsonb;
ALTER TABLE pam_settings ADD COLUMN IF NOT EXISTS commission_basis_points INT NOT NULL DEFAULT 0 CHECK(commission_basis_points BETWEEN 0 AND 10000);
CREATE TABLE IF NOT EXISTS pam_customers (
 id BIGSERIAL PRIMARY KEY, resident_id BIGINT NOT NULL UNIQUE REFERENCES residents(id), meter_number TEXT NOT NULL UNIQUE,
 active BOOLEAN NOT NULL DEFAULT TRUE, enrolled_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS pam_readings (
 id BIGSERIAL PRIMARY KEY, customer_id BIGINT NOT NULL REFERENCES pam_customers(id), billing_month DATE NOT NULL,
 previous_reading BIGINT NOT NULL CHECK(previous_reading>=0), current_reading BIGINT NOT NULL CHECK(current_reading>=previous_reading),
 consumption BIGINT NOT NULL CHECK(consumption>=0), recorded_by BIGINT REFERENCES app_users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 status TEXT NOT NULL DEFAULT 'APPROVED' CHECK(status IN ('PENDING','APPROVED','REJECTED')),
 reviewed_by BIGINT REFERENCES app_users(id), reviewed_at TIMESTAMPTZ, review_note TEXT NOT NULL DEFAULT '',
 UNIQUE(customer_id,billing_month)
);
ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'APPROVED' CHECK(status IN ('PENDING','APPROVED','REJECTED'));
ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS reviewed_by BIGINT REFERENCES app_users(id);
ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
ALTER TABLE pam_readings ADD COLUMN IF NOT EXISTS review_note TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS pam_bills (
 id BIGSERIAL PRIMARY KEY, reading_id BIGINT NOT NULL UNIQUE REFERENCES pam_readings(id), rate_per_m3 BIGINT NOT NULL CHECK(rate_per_m3>=0),
 monthly_fee BIGINT NOT NULL CHECK(monthly_fee>=0), total BIGINT NOT NULL CHECK(total>=0), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS pam_payments (
 id BIGSERIAL PRIMARY KEY, bill_id BIGINT NOT NULL REFERENCES pam_bills(id), amount BIGINT NOT NULL CHECK(amount>0),
 note TEXT NOT NULL DEFAULT '', received_by BIGINT REFERENCES app_users(id), received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 method_code TEXT NOT NULL DEFAULT 'collection'
);
ALTER TABLE pam_payments ADD COLUMN IF NOT EXISTS method_code TEXT NOT NULL DEFAULT 'collection';
CREATE TABLE IF NOT EXISTS pam_payment_choices (
 bill_id BIGINT PRIMARY KEY REFERENCES pam_bills(id) ON DELETE CASCADE,
 resident_id BIGINT NOT NULL REFERENCES residents(id), method_code TEXT NOT NULL, selected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS pam_payment_submissions (
 id BIGSERIAL PRIMARY KEY, bill_id BIGINT NOT NULL REFERENCES pam_bills(id), resident_id BIGINT NOT NULL REFERENCES residents(id),
 amount BIGINT NOT NULL CHECK(amount>0), method_code TEXT NOT NULL, proof_data TEXT NOT NULL, note TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','APPROVED','REJECTED')),
 submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(), reviewed_by BIGINT REFERENCES app_users(id),
 reviewed_at TIMESTAMPTZ, review_note TEXT NOT NULL DEFAULT '', payment_id BIGINT REFERENCES pam_payments(id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_pam_one_pending_submission_per_bill ON pam_payment_submissions(bill_id) WHERE status='PENDING';
CREATE INDEX IF NOT EXISTS idx_pam_payment_submissions_status ON pam_payment_submissions(status,submitted_at);
CREATE TABLE IF NOT EXISTS pam_commissions (
 bill_id BIGINT PRIMARY KEY REFERENCES pam_bills(id), staff_user_id BIGINT NOT NULL REFERENCES app_users(id),
 bill_total BIGINT NOT NULL, rate_basis_points INT NOT NULL CHECK(rate_basis_points BETWEEN 0 AND 10000),
 amount BIGINT NOT NULL CHECK(amount>=0), earned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_pam_commissions_staff ON pam_commissions(staff_user_id,earned_at DESC);
CREATE INDEX IF NOT EXISTS idx_pam_readings_customer_month ON pam_readings(customer_id,billing_month DESC);
CREATE INDEX IF NOT EXISTS idx_pam_payments_bill ON pam_payments(bill_id);
