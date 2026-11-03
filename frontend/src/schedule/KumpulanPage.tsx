import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { createPortal } from 'react-dom';
import {
  AlertCircle,
  AlertTriangle,
  CalendarDays,
  Check,
  Eye,
  Info,
  Mail,
  Pencil,
  Printer,
  RefreshCw,
  ReceiptText,
  Search,
  Send,
  Trash2,
  Plus,
} from 'lucide-react';
import Period, {
  PERIOD_STATUS_OPTIONS,
  formatMoney,
  type PeriodInput,
  type PeriodJSON,
  type PeriodStatus,
} from './Period';
import Invitation, { type Recipient } from './Invitation';
import PeriodRepository, { PeriodError } from './PeriodRepository';
import Modal from '../shared/Modal';
import '../shared/modal.css';
import './schedule.css';

type Dialog =
  | { kind: 'none' }
  | { kind: 'create' }
  | { kind: 'edit'; period: Period }
  | { kind: 'detail'; period: Period }
  | { kind: 'generate' }
  | { kind: 'invitation'; period: Period };

export interface KumpulanPageProps {
  periods: PeriodJSON[];
  onChanged: () => void;
  onToast: (message: string) => void;
  canManage: boolean;
}

const AUTO_START_DATE = '2025-01-25';
const AUTO_END_DATE = '2029-12-31';

/** Selisih hari antar tanggal ISO. */
const daysBetween = (from: string, to: string) => {
  const start = new Date(`${from}T00:00:00`).getTime();
  const end = new Date(`${to}T00:00:00`).getTime();
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return 0;
  return Math.floor((end - start) / 86_400_000);
};

/**
 * Halaman Kumpulan Ahad Pon.
 *
 * Desktop : datatable jadwal + pencarian.
 * Mobile  : kartu jadwal.
 * Aksi    : tambah, detail, ubah, hapus, buat tagihan, undangan (WA massal + PDF).
 */
export default function KumpulanPage({ periods, onChanged, onToast, canManage }: KumpulanPageProps) {
  const repository = useMemo(() => new PeriodRepository(), []);
  const [query, setQuery] = useState('');
  const [dialog, setDialog] = useState<Dialog>({ kind: 'none' });
  const [busyId, setBusyId] = useState<number | null>(null);

  const models = useMemo(() => Period.fromList(periods), [periods]);
  const filtered = useMemo(() => models.filter((period) => period.matches(query)), [models, query]);
  const close = () => setDialog({ kind: 'none' });

  const handleSaved = (message: string) => {
    onToast(message);
    onChanged();
    close();
  };

  const generateBills = async (period: Period) => {
    setBusyId(period.id);
    try {
      await repository.generateBills(period.id);
      onToast(`Tagihan periode ${period.meetingDate} berhasil dibuat`);
      onChanged();
      close();
    } catch (cause) {
      onToast(cause instanceof PeriodError ? cause.message : 'Tagihan gagal dibuat');
    } finally {
      setBusyId(null);
    }
  };

  const removePeriod = async (period: Period) => {
    if (!window.confirm(`Hapus jadwal kumpulan ${period.meetingDate}?`)) return;
    setBusyId(period.id);
    try {
      await repository.remove(period.id);
      onToast('Jadwal berhasil dihapus');
      onChanged();
      close();
    } catch (cause) {
      onToast(cause instanceof PeriodError ? cause.message : 'Jadwal gagal dihapus');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <section className="panel kumpulan-panel">
      <div className="panel-heading">
        <div>
          <h3>Jadwal Kumpulan Ahad Pon</h3>
          <p>{models.length} jadwal · siklus rutin setiap 35 hari</p>
        </div>
        {canManage && (
          <div className="kumpulan-heading-actions">
            <button type="button" className="ghost-btn" onClick={() => setDialog({ kind: 'generate' })}>
              <RefreshCw size={15} /> Generate jadwal
            </button>
            <button type="button" className="primary-btn" onClick={() => setDialog({ kind: 'create' })}>
              <Plus size={16} /> Tambah jadwal
            </button>
          </div>
        )}
      </div>

      <div className="kumpulan-toolbar">
        <div className="residents-search">
          <Search />
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Cari tanggal, status, atau catatan…"
            aria-label="Cari jadwal kumpulan"
          />
        </div>
        <span className="residents-count">
          {filtered.length === models.length ? `${models.length} jadwal` : `${filtered.length} dari ${models.length} jadwal`}
        </span>
      </div>

      {filtered.length === 0 ? (
        <div className="empty">
          <div>
            <CalendarDays size={22} />
          </div>
          <p>{models.length === 0 ? 'Belum ada jadwal kumpulan. Buat periode pertama.' : 'Tidak ada jadwal yang cocok dengan pencarian.'}</p>
        </div>
      ) : (
        <>
          {/* Desktop: datatable */}
          <div className="residents-table-wrap">
            <table className="residents-table kumpulan-table">
              <thead>
                <tr>
                  <th>Kumpulan</th>
                  <th>Periode</th>
                  <th>Status</th>
                  <th>Rekap kas</th>
                  <th>Aksi</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((period) => (
                  <tr key={period.id}>
                    <td>
                      <div className="kumpulan-date">
                        <div className="date-block">
                          <b>{period.dayNumber}</b>
                          <span>{period.monthShort}</span>
                        </div>
                        <div>
                          <b>Malam Ahad Pon</b>
                          <small>
                            {period.meetingDate} · {period.timeLabel}
                          </small>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span className="kumpulan-period">
                        {period.startDate} <span>→</span> {period.meetingDate}
                      </span>
                      {period.isAutoGenerated && <small className="kumpulan-auto">Otomatis 35 hari</small>}
                    </td>
                    <td>
                      <span className={`status ${period.tone}`}>{period.statusLabel}</span>
                    </td>
                    <td>
                      {period.finance ? (
                        <>
                          <b>{formatMoney(period.finance.totalCollected)}</b>
                          <small>
                            {period.finance.paymentCount} transaksi · Kas Bendahara Umum {formatMoney(period.finance.kasRt)}
                          </small>
                        </>
                      ) : (
                        <span className="resident-chip muted">Belum ada rekap</span>
                      )}
                    </td>
                    <td>
                      <div className="resident-actions">
                        <button
                          type="button"
                          className="resident-action"
                          title="Lihat detail"
                          aria-label={`Detail ${period.meetingDate}`}
                          onClick={() => setDialog({ kind: 'detail', period })}
                        >
                          <Eye />
                        </button>
                        {period.canInvite && (
                          <button
                            type="button"
                            className="resident-action"
                            title="Undangan"
                            aria-label={`Undangan ${period.meetingDate}`}
                            onClick={() => setDialog({ kind: 'invitation', period })}
                          >
                            <Mail />
                          </button>
                        )}
                        {canManage && (
                          <button
                            type="button"
                            className="resident-action"
                            title="Ubah jadwal"
                            aria-label={`Ubah ${period.meetingDate}`}
                            onClick={() => setDialog({ kind: 'edit', period })}
                          >
                            <Pencil />
                          </button>
                        )}
                        {canManage && period.canDelete && (
                          <button
                            type="button"
                            className="resident-action danger"
                            title="Hapus jadwal"
                            aria-label={`Hapus ${period.meetingDate}`}
                            onClick={() => void removePeriod(period)}
                          >
                            <Trash2 />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile: card */}
          <div className="residents-cards">
            {filtered.map((period) => (
              <article className="resident-card" key={period.id}>
                <div className="resident-card-head">
                  <div className="date-block">
                    <b>{period.dayNumber}</b>
                    <span>{period.monthShort}</span>
                  </div>
                  <div>
                    <b>Malam Ahad Pon</b>
                    <small>
                      {period.meetingDateLabel} · {period.timeLabel}
                    </small>
                  </div>
                  <span className={`status ${period.tone}`}>{period.statusLabel}</span>
                </div>
                <div className="resident-card-meta">
                  <span>
                    <CalendarDays /> Periode {period.startDate} → {period.meetingDate}
                  </span>
                  {period.finance ? (
                    <span>
                      <ReceiptText /> {period.finance.paymentCount} transaksi · {formatMoney(period.finance.totalCollected)}
                    </span>
                  ) : (
                    <span>
                      <ReceiptText /> Rekap kas belum tersedia
                    </span>
                  )}
                </div>
                <div className="resident-card-actions">
                  <button type="button" onClick={() => setDialog({ kind: 'detail', period })}>
                    <Eye /> Detail
                  </button>
                  <button
                    type="button"
                    disabled={!period.canInvite}
                    onClick={() => setDialog({ kind: 'invitation', period })}
                  >
                    <Mail /> Undangan
                  </button>
                  <button
                    type="button"
                    onClick={() => (canManage ? setDialog({ kind: 'edit', period }) : setDialog({ kind: 'detail', period }))}
                  >
                    <Pencil /> {canManage ? 'Ubah' : 'Lihat'}
                  </button>
                </div>
              </article>
            ))}
          </div>
        </>
      )}

      {dialog.kind === 'create' && (
        <PeriodFormModal
          mode="create"
          onClose={close}
          onSubmit={async (input) => {
            await repository.create(input);
            handleSaved(`Jadwal kumpulan ${input.meeting_date} tersimpan`);
          }}
        />
      )}

      {dialog.kind === 'edit' && (
        <PeriodFormModal
          mode="edit"
          period={dialog.period}
          onClose={close}
          onSubmit={async (input) => {
            const removed = dialog.period.status === 'TERLAKSANA' && input.status !== 'TERLAKSANA'
              ? dialog.period.billsUnpaidCount
              : 0;
            await repository.update(dialog.period.id, input);
            handleSaved(
              removed > 0
                ? `Jadwal ${input.meeting_date} diperbarui, ${removed} tagihan belum dibayar dihapus`
                : `Jadwal ${input.meeting_date} berhasil diperbarui`,
            );
          }}
        />
      )}

      {dialog.kind === 'detail' && (
        <PeriodDetailModal
          period={dialog.period}
          canManage={canManage}
          busy={busyId === dialog.period.id}
          onClose={close}
          onEdit={() => setDialog({ kind: 'edit', period: dialog.period })}
          onInvite={() => setDialog({ kind: 'invitation', period: dialog.period })}
          onGenerateBills={() => void generateBills(dialog.period)}
          onDelete={() => void removePeriod(dialog.period)}
        />
      )}

      {dialog.kind === 'generate' && (
        <GenerateSeriesModal
          onClose={close}
          onSubmit={async (input) => {
            const result = await repository.generateSeries(input);
            handleSaved(
              `Jadwal dibuat: ${result.created} baru, ${result.skipped} dilewati (${result.first_date} – ${result.last_date}, pukul ${result.meeting_time}).`,
            );
          }}
        />
      )}

      {dialog.kind === 'invitation' && (
        <InvitationModal
          period={dialog.period}
          repository={repository}
          onClose={close}
          onToast={onToast}
        />
      )}
    </section>
  );
}

/* ------------------------------------------------------------------ */
/* Modal tambah / ubah jadwal                                          */
/* ------------------------------------------------------------------ */

function PeriodFormModal({
  mode,
  period,
  onClose,
  onSubmit,
}: {
  mode: 'create' | 'edit';
  period?: Period;
  onClose: () => void;
  onSubmit: (input: PeriodInput) => Promise<void>;
}) {
  const [form, setForm] = useState<PeriodInput>(
    period
      ? period.toPayload()
      : { start_date: '', meeting_date: '', meeting_time: '19:30', status: 'TERJADWAL', note: '' },
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [confirming, setConfirming] = useState(false);

  const update = <K extends keyof PeriodInput>(key: K, value: PeriodInput[K]) =>
    setForm((current) => ({ ...current, [key]: value }));

  const statusLabel = (status: PeriodStatus) =>
    PERIOD_STATUS_OPTIONS.find((option) => option.value === status)?.label ?? status;

  /** Kumpulan terlaksana yang dibatalkan akan kehilangan tagihan belum dibayar. */
  const needsBillWarning = mode === 'edit'
    && !!period
    && period.status === 'TERLAKSANA'
    && form.status !== 'TERLAKSANA'
    && period.billsUnpaidCount > 0;

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      await onSubmit(form);
    } catch (cause) {
      setError(cause instanceof PeriodError ? cause.message : 'Jadwal gagal disimpan.');
      setSaving(false);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!form.start_date || !form.meeting_date || !form.meeting_time) {
      setError('Tanggal awal, tanggal kumpulan, dan jam wajib diisi.');
      return;
    }
    if (form.meeting_date < form.start_date) {
      setError('Tanggal kumpulan tidak boleh sebelum tanggal awal periode.');
      return;
    }
    if (needsBillWarning) {
      setConfirming(true);
      return;
    }
    await save();
  };

  return (
    <>
    <Modal
      title={mode === 'create' ? 'Tambah jadwal kumpulan' : `Ubah jadwal ${period?.meetingDate ?? ''}`}
      subtitle="Siklus kumpulan berulang setiap 35 hari mengikuti kalender Jawa."
      onClose={onClose}
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Batal
          </button>
          <button type="submit" form="period-form" className="primary-btn" disabled={saving}>
            {saving ? 'Menyimpan…' : mode === 'create' ? 'Simpan jadwal' : 'Simpan perubahan'}
          </button>
        </>
      }
    >
      <form id="period-form" className="form-grid" onSubmit={submit}>
        {error && (
          <p className="form-error span-2">
            <AlertCircle size={14} /> {error}
          </p>
        )}
        <label>
          Tanggal awal periode
          <input type="date" required value={form.start_date} onChange={(event) => update('start_date', event.target.value)} />
        </label>
        <label>
          Tanggal kumpulan
          <input type="date" required value={form.meeting_date} onChange={(event) => update('meeting_date', event.target.value)} />
        </label>
        <label>
          Jam kumpulan
          <input type="time" required value={form.meeting_time} onChange={(event) => update('meeting_time', event.target.value)} />
        </label>
        <label>
          Status kumpulan
          <select value={form.status} onChange={(event) => update('status', event.target.value as PeriodStatus)}>
            {PERIOD_STATUS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label className="span-2">
          Catatan
          <input
            value={form.note}
            onChange={(event) => update('note', event.target.value)}
            placeholder="Opsional, mis. kumpulan di rumah Pak RT"
          />
        </label>
        <p className="hint span-2">
          {mode === 'edit' && period?.status === 'TERLAKSANA'
            ? 'Mengubah status ke Libur atau Terjadwal akan menghapus tagihan periode ini yang belum dibayar.'
            : 'Status LIBUR membuat sistem menolak pembuatan tagihan baru untuk periode ini.'}
        </p>
      </form>
    </Modal>

    {confirming && period && (
      <Modal
        title="Batalkan kumpulan ini?"
        subtitle={`Status diubah dari ${period.statusLabel} menjadi ${statusLabel(form.status)}.`}
        onClose={() => setConfirming(false)}
        size="small"
        footer={
          <>
            <button type="button" className="ghost-btn" onClick={() => setConfirming(false)} disabled={saving}>
              Kembali
            </button>
            <button type="button" className="primary-btn" onClick={() => void save()} disabled={saving}>
              {saving ? 'Menyimpan…' : 'Ya, ubah status'}
            </button>
          </>
        }
      >
        <div className="bill-warning">
          <p className="bill-warning-lead">
            <AlertTriangle size={16} />
            <span>
              <b>{period.billsUnpaidCount} tagihan</b> belum dibayar ({formatMoney(period.billsUnpaidAmount)})
              akan dihapus dari daftar tagihan.
            </span>
          </p>
          <ul>
            <li>
              {period.billsPaidCount > 0
                ? `${period.billsPaidCount} tagihan yang sudah dibayar tetap disimpan sebagai riwayat.`
                : 'Belum ada tagihan yang dibayar pada periode ini.'}
            </li>
            <li>Pembayaran yang sudah tercatat tidak dikembalikan otomatis.</li>
            <li>Bila status dikembalikan ke Terlaksana, tagihan dibuat ulang otomatis.</li>
          </ul>
        </div>
      </Modal>
    )}
    </>
  );
}

/* ------------------------------------------------------------------ */
/* Modal detail jadwal                                                 */
/* ------------------------------------------------------------------ */

function PeriodDetailModal({
  period,
  canManage,
  busy,
  onClose,
  onEdit,
  onInvite,
  onGenerateBills,
  onDelete,
}: {
  period: Period;
  canManage: boolean;
  busy: boolean;
  onClose: () => void;
  onEdit: () => void;
  onInvite: () => void;
  onGenerateBills: () => void;
  onDelete: () => void;
}) {
  return (
    <Modal
      title="Detail jadwal kumpulan"
      subtitle={period.meetingDateLabel}
      onClose={onClose}
      size="medium"
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Tutup
          </button>
          {canManage && period.canDelete && (
            <button type="button" className="ghost-btn danger" onClick={onDelete} disabled={busy}>
              <Trash2 size={15} /> Hapus
            </button>
          )}
          {period.canInvite && (
            <button type="button" className="ghost-btn" onClick={onInvite}>
              <Mail size={15} /> Undangan
            </button>
          )}
          {canManage && (
            <button type="button" className="ghost-btn" onClick={onEdit}>
              <Pencil size={15} /> Ubah
            </button>
          )}
          {canManage && period.canGenerateBills && (
            <button type="button" className="primary-btn" onClick={onGenerateBills} disabled={busy}>
              <ReceiptText size={15} /> {busy ? 'Memproses…' : 'Buat tagihan'}
            </button>
          )}
        </>
      }
    >
      <div className="detail-hero">
        <div className="date-block">
          <b>{period.dayNumber}</b>
          <span>{period.monthShort}</span>
        </div>
        <div>
          <b>Malam Ahad Pon</b>
          <small>
            {period.timeLabel} · {period.statusLabel}
          </small>
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-item">
          <span>Tanggal awal periode</span>
          <b>{period.startDate}</b>
        </div>
        <div className="detail-item">
          <span>Tanggal kumpulan</span>
          <b>{period.meetingDate}</b>
        </div>
        <div className="detail-item">
          <span>Jam kumpulan</span>
          <b>{period.timeLabel}</b>
        </div>
        <div className="detail-item">
          <span>Status</span>
          <b>
            <span className={`status ${period.tone}`}>{period.statusLabel}</span>
          </b>
        </div>
        <div className="detail-item wide">
          <span>Catatan</span>
          <b>{period.note || 'Tanpa catatan'}</b>
        </div>
      </div>

      {period.finance ? (
        <div className="kumpulan-finance">
          <div className="kumpulan-finance-head">
            <div>
              <h4>Rekap kas kumpulan</h4>
              <p>{period.finance.paymentCount} transaksi pembayaran masuk</p>
            </div>
            <b>{formatMoney(period.finance.totalCollected)}</b>
          </div>
          <div className="kumpulan-finance-rows">
            {period.finance.breakdown.map((row) => (
              <div key={row.label}>
                <span>{row.label}</span>
                <b>{formatMoney(row.amount)}</b>
              </div>
            ))}
            <div className="total">
              <span>Kas Bendahara Umum</span>
              <b>{formatMoney(period.finance.kasRt)}</b>
            </div>
          </div>
        </div>
      ) : (
        <p className="hint" style={{ display: 'flex', gap: 6 }}>
          <Info size={13} style={{ flexShrink: 0, marginTop: 1 }} />
          Rekap kas muncul setelah kumpulan ditandai TERLAKSANA dan pembayaran warga tercatat.
        </p>
      )}
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Modal generate jadwal                                               */
/* ------------------------------------------------------------------ */

function GenerateSeriesModal({
  onClose,
  onSubmit,
}: {
  onClose: () => void;
  onSubmit: (input: { start_date: string; through_date: string; meeting_time: string }) => Promise<void>;
}) {
  const [startDate, setStartDate] = useState(AUTO_START_DATE);
  const [throughDate, setThroughDate] = useState(AUTO_END_DATE);
  const [meetingTime, setMeetingTime] = useState('19:30');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const estimate = useMemo(() => Math.floor(daysBetween(startDate, throughDate) / 35) + 1, [startDate, throughDate]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!startDate || !throughDate) {
      setError('Tanggal awal dan tanggal akhir wajib diisi.');
      return;
    }
    if (throughDate < startDate) {
      setError('Tanggal akhir tidak boleh sebelum tanggal awal.');
      return;
    }
    setSaving(true);
    setError('');
    try {
      await onSubmit({ start_date: startDate, through_date: throughDate, meeting_time: meetingTime });
    } catch (cause) {
      setError(cause instanceof PeriodError ? cause.message : 'Jadwal gagal dibuat.');
      setSaving(false);
    }
  };

  return (
    <Modal
      title="Generate jadwal Ahad Pon"
      subtitle="Jadwal berulang setiap 35 hari. Jadwal yang sudah ada dilewati, bukan digandakan."
      onClose={onClose}
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Batal
          </button>
          <button type="submit" form="generate-form" className="primary-btn" disabled={saving}>
            {saving ? 'Membuat…' : `Buat ${estimate} jadwal`}
          </button>
        </>
      }
    >
      <form id="generate-form" className="form-grid" onSubmit={submit}>
        {error && (
          <p className="form-error span-2">
            <AlertCircle size={14} /> {error}
          </p>
        )}
        <label>
          Tanggal awal (acuan Ahad Pon)
          <input type="date" required value={startDate} onChange={(event) => setStartDate(event.target.value)} />
        </label>
        <label>
          Tanggal akhir
          <input type="date" required value={throughDate} onChange={(event) => setThroughDate(event.target.value)} />
        </label>
        <label>
          Jam kumpulan
          <input type="time" required value={meetingTime} onChange={(event) => setMeetingTime(event.target.value)} />
        </label>
        <div className="detail-item">
          <span>Perkiraan jumlah jadwal</span>
          <b>{estimate} kumpulan</b>
        </div>
        <p className="hint span-2">
          Ubah <b>tanggal akhir</b> bila ingin memperpanjang atau membatasi rentang jadwal. Tanggal awal harus mengikuti malam Ahad Pon
          yang benar agar pola 35 hari tetap tepat.
        </p>
      </form>
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Modal undangan: WhatsApp massal + cetak PDF                         */
/* ------------------------------------------------------------------ */

/* ------------------------------------------------------------------ */
/* Dokumen undangan (dipakai untuk pratinjau dan untuk cetak/PDF)      */
/* ------------------------------------------------------------------ */

function InvitationSheet({ id, invitation }: { id: string; invitation: Invitation }) {
  return (
    <div id={id} className="invitation-sheet">
      <div className="sheet-head">
        <h1>UNDANGAN KUMPULAN AHAD PON</h1>
        <p>Rukun Tetangga 03 · Rukun Warga 05 · Widoro, Potronayan</p>
      </div>
      <div className="sheet-meta">
        <div>
          <span>Hari / tanggal</span>
          <b>{invitation.period.meetingDateLabel}</b>
        </div>
        <div>
          <span>Waktu</span>
          <b>{invitation.period.timeLabel}</b>
        </div>
        <div>
          <span>Tabungan jalan</span>
          <b>{invitation.savingsStage ? `Tahap ${invitation.savingsStage}` : 'Belum terhubung'}</b>
        </div>
      </div>
      <p className="sheet-lead">
        Dengan hormat, kami mengundang Bapak/Ibu/Saudara untuk hadir pada kumpulan warga. Berikut rincian kewajiban masing-masing
        kepala keluarga:
      </p>
      <table className="sheet-table">
        <thead>
          <tr>
            <th>No</th>
            <th>Kode rumah</th>
            <th>Nama kepala keluarga</th>
            <th>Tunggakan kas</th>
            <th>Tabungan jalan</th>
          </tr>
        </thead>
        <tbody>
          {invitation.printRows.map((row) => (
            <tr key={row.no}>
              <td>{row.no}</td>
              <td>{row.houseCode}</td>
              <td>{row.headName}</td>
              <td>{row.cashDue}</td>
              <td>{row.savingsDue}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          <tr>
            <td colSpan={3}>Total</td>
            <td>{formatMoney(invitation.totalCashDue)}</td>
            <td>{invitation.savingsStage ? formatMoney(invitation.totalSavingsDue) : '—'}</td>
          </tr>
        </tfoot>
      </table>
      <div className="sheet-sign">
        <div>
          <span>Ketua RT</span>
          <b>………………………………</b>
        </div>
        <div>
          <span>Bendahara</span>
          <b>………………………………</b>
        </div>
      </div>
    </div>
  );
}

function InvitationModal({
  period,
  repository,
  onClose,
  onToast,
}: {
  period: Period;
  repository: PeriodRepository;
  onClose: () => void;
  onToast: (message: string) => void;
}) {
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [sent, setSent] = useState<number[]>([]);
  const [tab, setTab] = useState<'recipients' | 'print'>('recipients');
  const [filter, setFilter] = useState<'all' | 'pending' | 'nophone'>('all');

  useEffect(() => {
    let active = true;
    setLoading(true);
    repository
      .invitation(period.id)
      .then((result) => {
        if (active) setInvitation(result);
      })
      .catch((cause) => {
        if (active) setError(cause instanceof PeriodError ? cause.message : 'Undangan gagal dimuat');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [period.id, repository]);

  const windowOpen = invitation?.isSendWindowOpen() ?? false;
  const withPhone = invitation?.withPhone ?? [];
  const pending = useMemo(() => withPhone.filter((recipient) => !sent.includes(recipient.id)), [withPhone, sent]);

  const rows = useMemo(() => {
    if (!invitation) return [];
    if (filter === 'pending') return invitation.recipients.filter((recipient) => !sent.includes(recipient.id));
    if (filter === 'nophone') return invitation.withoutPhone;
    return invitation.recipients;
  }, [invitation, filter, sent]);

  const sendOne = (recipient: Recipient) => {
    if (!invitation) return;
    const link = invitation.whatsappLinkFor(recipient);
    if (!link) {
      onToast(`${recipient.headName} belum punya nomor WhatsApp`);
      return;
    }
    window.open(link, '_blank', 'noopener');
    setSent((current) => (current.includes(recipient.id) ? current : [...current, recipient.id]));
  };

  const sendNext = () => {
    const next = pending[0];
    if (next) sendOne(next);
  };

  const print = () => {
    setTab('print');
    window.setTimeout(() => window.print(), 120);
  };

  const primaryLabel = !invitation
    ? 'Menyiapkan…'
    : withPhone.length === 0
      ? 'Tidak ada nomor WA'
      : pending.length === 0
        ? 'Semua terkirim'
        : `Kirim WhatsApp (${sent.length}/${withPhone.length})`;

  return (
    <Modal
      title="Undangan kumpulan"
      subtitle={`${period.meetingDateLabel} · ${period.timeLabel}`}
      onClose={onClose}
      size="wide"
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Tutup
          </button>
          <button type="button" className="ghost-btn" onClick={print} disabled={!invitation}>
            <Printer size={15} /> Cetak / PDF
          </button>
          <button
            type="button"
            className="primary-btn"
            onClick={sendNext}
            disabled={!invitation || !windowOpen || pending.length === 0}
          >
            <Send size={15} /> {primaryLabel}
          </button>
        </>
      }
    >
      {loading && (
        <p className="hint" style={{ display: 'flex', gap: 6 }}>
          <RefreshCw size={13} className="spin" /> Memuat daftar penerima…
        </p>
      )}

      {error && (
        <p className="form-error">
          <AlertCircle size={14} /> {error}
        </p>
      )}

      {invitation && (
        <>
          <div className="invitation-summary">
            <div>
              <span>Penerima</span>
              <b>{invitation.recipients.length} KK</b>
            </div>
            <div>
              <span>Tunggakan kas</span>
              <b>{formatMoney(invitation.totalCashDue)}</b>
            </div>
            <div>
              <span>Tabungan tahap {invitation.savingsStage || '—'}</span>
              <b>{formatMoney(invitation.totalSavingsDue)}</b>
            </div>
            <div>
              <span>Pengingat H-5</span>
              <b>{invitation.reminderDate}</b>
            </div>
          </div>

          <div className="invitation-tabs" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'recipients'}
              className={tab === 'recipients' ? 'active' : ''}
              onClick={() => setTab('recipients')}
            >
              <Send size={14} /> Penerima &amp; WhatsApp
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={tab === 'print'}
              className={tab === 'print' ? 'active' : ''}
              onClick={() => setTab('print')}
            >
              <Printer size={14} /> Dokumen undangan
            </button>
          </div>

          {tab === 'recipients' && (
            <>
              {!windowOpen && (
                <p className="form-error">
                  <AlertCircle size={14} /> Tautan WhatsApp baru aktif pada {invitation.reminderDate} (H-5). Dokumen PDF tetap bisa
                  dicetak sekarang.
                </p>
              )}

              {invitation.withoutPhone.length > 0 && (
                <p className="hint">
                  <Info size={13} style={{ flexShrink: 0, marginTop: 1 }} />
                  {invitation.withoutPhone.length} KK belum punya nomor WhatsApp yang valid, undangannya hanya bisa dicetak. Lengkapi
                  nomor di halaman Data warga.
                </p>
              )}

              <p className="hint">
                Tombol <b>Kirim WhatsApp</b> membuka satu percakapan per klik agar tidak diblokir browser: kirim pesannya, tutup tab,
                lalu klik lagi untuk KK berikutnya ({sent.length} sudah dibuka).
              </p>

              <div className="invitation-toolbar">
                {(
                  [
                    ['all', `Semua (${invitation.recipients.length})`],
                    ['pending', `Belum dibuka (${invitation.recipients.length - sent.length})`],
                    ['nophone', `Tanpa nomor (${invitation.withoutPhone.length})`],
                  ] as const
                ).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    className={`chip-btn${filter === value ? ' active' : ''}`}
                    onClick={() => setFilter(value)}
                  >
                    {label}
                  </button>
                ))}
              </div>

              <div className="invitation-list">
                <table>
                  <thead>
                    <tr>
                      <th>KK</th>
                      <th>Tunggakan kas</th>
                      <th>Tabungan</th>
                      <th>Status</th>
                      <th>Aksi</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((recipient) => {
                      const isSent = sent.includes(recipient.id);
                      return (
                        <tr key={recipient.id}>
                          <td>
                            <b>{recipient.houseCode}</b>
                            <small>{recipient.headName}</small>
                            <small className="inv-savings-mobile">
                              Tabungan {invitation.savingsStage ? recipient.savingsDueLabel : '—'}
                            </small>
                          </td>
                          <td>{recipient.cashDueLabel}</td>
                          <td>{invitation.savingsStage ? recipient.savingsDueLabel : '—'}</td>
                          <td>
                            {isSent ? (
                              <span className="resident-chip">
                                <Check size={11} /> Dibuka
                              </span>
                            ) : recipient.hasPhone ? (
                              <span className="resident-chip muted">Belum</span>
                            ) : (
                              <span className="resident-chip muted">Tanpa nomor</span>
                            )}
                          </td>
                          <td>
                            <button
                              type="button"
                              className="text-link"
                              disabled={!windowOpen || !recipient.hasPhone}
                              onClick={() => sendOne(recipient)}
                            >
                              <Send size={13} /> Kirim
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                    {rows.length === 0 && (
                      <tr>
                        <td colSpan={5} className="invitation-empty">
                          Tidak ada KK pada filter ini.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {/* Pratinjau dokumen di dalam modal. */}
          <div className={`invitation-sheet-wrap${tab === 'print' ? ' is-visible' : ''}`}>
            <p className="hint print-hint">
              <Info size={13} /> Pratinjau dokumen. Pakai tombol <b>Cetak / PDF</b> lalu pilih “Save as PDF” pada dialog cetak.
            </p>
            <InvitationSheet id="invitation-preview" invitation={invitation} />
          </div>

          {/* Salinan khusus cetak, ditempel langsung ke <body> agar bisa pecah halaman. */}
          {createPortal(
            <div className="print-doc">
              <InvitationSheet id="invitation-print-area" invitation={invitation} />
            </div>,
            document.body,
          )}
        </>
      )}
    </Modal>
  );
}
