import { useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  AlertCircle,
  CalendarDays,
  Check,
  CheckCircle2,
  Info,
  Pencil,
  Trash2,
  ReceiptText,
  RefreshCw,
  Search,
  Wallet,
} from 'lucide-react';
import {
  BillBook,
  formatRupiah,
  type BillJSON,
  type ResidentOption,
} from './Bill';
import BillRepository, { BillError, type FinanceReport, type PaymentRecord } from './BillRepository';
import Modal from '../shared/Modal';
import '../shared/modal.css';
import '../shared/list.css';
import './bills.css';

export interface BillsPageProps {
  residents: ResidentOption[];
  bills: BillJSON[];
  /** Periode TERLAKSANA yang belum punya tagihan (untuk tombol sinkronisasi). */
  missingPeriodCount: number;
  onChanged: () => void;
  onToast: (message: string) => void;
  canManage: boolean;
  canManagePayments: boolean;
}

type Filter = 'all' | 'unpaid' | 'paid';

const FILTERS: { value: Filter; label: string }[] = [
  { value: 'all', label: 'Semua' },
  { value: 'unpaid', label: 'Belum lunas' },
  { value: 'paid', label: 'Lunas' },
];

/**
 * Halaman Tagihan & pembayaran.
 *
 * Desktop : datatable tagihan per warga + filter.
 * Mobile  : kartu tagihan.
 * Aksi    : terima pembayaran (semua tagihan / periode tertentu) dan
 *           sinkronisasi tagihan untuk kumpulan yang sudah terlaksana.
 */
export default function BillsPage({
  residents,
  bills,
  missingPeriodCount,
  onChanged,
  onToast,
  canManage,
  canManagePayments,
}: BillsPageProps) {
  const repository = useMemo(() => new BillRepository(), []);
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  const [dialog, setDialog] = useState<{ kind: 'none' } | { kind: 'pay'; residentId?: number }>({ kind: 'none' });
  const [syncing, setSyncing] = useState(false);
  const [payments, setPayments] = useState<PaymentRecord[]>([]);
  const [paymentsVersion, setPaymentsVersion] = useState(0);
  const [editingPayment, setEditingPayment] = useState<number | null>(null);
  const [editingAmount, setEditingAmount] = useState('');
  const [paymentError, setPaymentError] = useState('');
  const [activeTab, setActiveTab] = useState<'bills' | 'history' | 'finance'>('bills');

  useEffect(() => {
    repository.listPayments().then(setPayments).catch((cause) => setPaymentError(cause instanceof Error ? cause.message : 'Riwayat pembayaran gagal dimuat.'));
  }, [repository, paymentsVersion]);

  const savePaymentAmount = async (payment: PaymentRecord) => {
    const amount = Number(editingAmount);
    if (!Number.isInteger(amount) || amount < 1) { setPaymentError('Nominal harus berupa bilangan positif.'); return; }
    try {
      await repository.updatePayment(payment.id, amount);
      setPaymentError(''); setEditingPayment(null); setPaymentsVersion((version) => version + 1); onChanged(); onToast('Nominal pembayaran diperbarui.');
    } catch (cause) { setPaymentError(cause instanceof Error ? cause.message : 'Nominal gagal diperbarui.'); }
  };

  const removePayment = async (payment: PaymentRecord) => {
    if (!window.confirm(`Hapus pembayaran ${formatRupiah(payment.amount)} dari ${payment.house_code}? Sisa tagihan terkait akan terbuka kembali.`)) return;
    try {
      await repository.deletePayment(payment.id);
      setPaymentError(''); setPaymentsVersion((version) => version + 1); onChanged(); onToast('Catatan pembayaran dihapus.');
    } catch (cause) { setPaymentError(cause instanceof Error ? cause.message : 'Pembayaran gagal dihapus.'); }
  };

  const book = useMemo(() => BillBook.fromList(bills), [bills]);
  const ledgers = useMemo(() => book.filterLedgers(query, filter), [book, query, filter]);
  const filteredBills = useMemo(() => book.filter(query, filter), [book, query, filter]);
  const close = () => setDialog({ kind: 'none' });

  const syncBills = async () => {
    setSyncing(true);
    try {
      const result = await repository.syncMeetingBills();
      onToast(
        result.created > 0
          ? `${result.created} tagihan dibuat untuk ${result.periods} kumpulan terlaksana.`
          : 'Semua kumpulan terlaksana sudah punya tagihan.',
      );
      onChanged();
    } catch (cause) {
      onToast(cause instanceof BillError ? cause.message : 'Tagihan gagal disinkronkan');
    } finally {
      setSyncing(false);
    }
  };

  return (
    <section className="panel bills-panel">
      <div className="panel-heading">
        <div>
          <h3>Tagihan &amp; pembayaran</h3>
          <p>
            Iuran kumpulan {formatRupiah(20000)} per kumpulan · {book.bills.length} tagihan · sisa{' '}
            {formatRupiah(book.outstanding)}
          </p>
        </div>
        {(canManage || canManagePayments) && (
          <div className="bills-heading-actions">
            {missingPeriodCount > 0 && (
              <button type="button" className="ghost-btn" onClick={() => void syncBills()} disabled={syncing}>
                <RefreshCw size={15} /> {syncing ? 'Menyinkronkan…' : `Sinkronkan ${missingPeriodCount} kumpulan`}
              </button>
            )}
            {canManagePayments && <button type="button" className="primary-btn" onClick={() => setDialog({ kind: 'pay' })}>
              <Wallet size={16} /> Terima pembayaran
            </button>}
          </div>
        )}
      </div>

      <div className="invitation-tabs" role="tablist" aria-label="Kelola pembayaran">
        {([['bills','Tagihan Pembayaran'],['history','Riwayat Pembayaran'],['finance','Rekap Pembayaran']] as const).map(([key,label])=><button key={key} type="button" role="tab" aria-selected={activeTab===key} className={activeTab===key?'active':''} onClick={()=>setActiveTab(key)}>{label}</button>)}
      </div>

      {activeTab === 'bills' && <>
      <div className="bills-summary">
        <div>
          <span>Total tagihan</span>
          <b>{book.bills.length}</b>
          <small>{formatRupiah(book.totalBilled)}</small>
        </div>
        <div>
          <span>Sudah dibayar</span>
          <b>{formatRupiah(book.totalPaid)}</b>
          <small>{book.paidCount} tagihan lunas</small>
        </div>
        <div>
          <span>Tunggakan</span>
          <b className={book.outstanding > 0 ? 'warn' : 'good'}>{formatRupiah(book.outstanding)}</b>
          <small>{book.bills.length - book.paidCount} tagihan terbuka</small>
        </div>
        <div>
          <span>KK menunggak</span>
          <b>{book.debtorCount}</b>
          <small>dari {book.ledgers.length} KK bertagihan</small>
        </div>
      </div>

      <div className="bills-toolbar">
        <div className="residents-search">
          <Search />
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Cari kode rumah, nama, atau tanggal kumpulan…"
            aria-label="Cari tagihan"
          />
        </div>
        <div className="bills-filters">
          {FILTERS.map((item) => (
            <button
              key={item.value}
              type="button"
              className={filter === item.value ? 'active' : ''}
              onClick={() => setFilter(item.value)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <span className="residents-count">
          {ledgers.length === book.ledgers.length
            ? `${book.ledgers.length} KK · ${book.bills.length} tagihan`
            : `${ledgers.length} dari ${book.ledgers.length} KK`}
        </span>
      </div>

      {filteredBills.length === 0 ? (
        <div className="empty">
          <div>
            <ReceiptText size={22} />
          </div>
          <p>
            {book.bills.length === 0
              ? 'Belum ada tagihan. Tagihan muncul otomatis setelah jadwal kumpulan ditandai terlaksana.'
              : 'Tidak ada KK yang cocok dengan filter.'}
          </p>
        </div>
      ) : (
        <>
          {/* Desktop: datatable per KK */}
          <div className="bills-table-wrap">
            <table className="bills-table">
              <thead>
                <tr>
                  <th>Warga</th>
                  <th>Kumpulan</th>
                  <th>Tagihan</th>
                  <th>Dibayar</th>
                  <th>Sisa</th>
                  <th>Status</th>
                  <th>Aksi</th>
                </tr>
              </thead>
              <tbody>
                {ledgers.map((ledger) => (
                  <tr key={ledger.residentId}>
                    <td>
                      <div className="bill-identity">
                        <div className="bill-avatar">{initialsOf(ledger.headName, ledger.houseCode)}</div>
                        <div>
                          <b>{ledger.houseCode}</b>
                          <small>{ledger.headName}</small>
                        </div>
                      </div>
                    </td>
                    <td>
                      <div className="bill-period">
                        <b>{ledger.periodRangeLabel}</b>
                        <small>
                          {ledger.bills.length} kumpulan · {ledger.unpaid.length} belum lunas
                        </small>
                      </div>
                    </td>
                    <td className="bill-money">{formatRupiah(ledger.billed)}</td>
                    <td className="bill-money">{formatRupiah(ledger.paidTotal)}</td>
                    <td className="bill-money">
                      <b className={ledger.isSettled ? 'text-good' : 'text-warn'}>{formatRupiah(ledger.outstanding)}</b>
                    </td>
                    <td>
                      <span className={`status ${ledger.isSettled ? 'done' : 'off'}`}>
                        {ledger.isSettled ? 'Lunas' : 'Menunggak'}
                      </span>
                    </td>
                    <td>
                      {!ledger.isSettled && canManagePayments ? (
                        <button
                          type="button"
                          className="bill-pay-btn"
                          onClick={() => setDialog({ kind: 'pay', residentId: ledger.residentId })}
                        >
                          <Wallet size={13} /> Bayar {formatRupiah(ledger.outstanding)}
                        </button>
                      ) : (
                        <span className="resident-chip muted">{ledger.isSettled ? 'Selesai' : '—'}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile: kartu per warga */}
          <div className="bills-cards">
            {ledgers.map((ledger) => (
              <article className="bill-card" key={ledger.residentId}>
                <div className="bill-card-head">
                  <div className="bill-avatar">{initialsOf(ledger.headName, ledger.houseCode)}</div>
                  <div>
                    <b>{ledger.houseCode}</b>
                    <small>{ledger.headName}</small>
                  </div>
                  <span className={`status ${ledger.isSettled ? 'done' : 'off'}`}>
                    {ledger.isSettled ? 'Lunas' : `${ledger.unpaid.length} tagihan`}
                  </span>
                </div>
                <div className="bill-card-meta">
                  <span>
                    <CalendarDays /> {ledger.periodRangeLabel}
                  </span>
                  <span>
                    <ReceiptText /> Tagihan {formatRupiah(ledger.billed)} · dibayar {formatRupiah(ledger.paidTotal)}
                  </span>
                  <span>
                    <Wallet /> Sisa <b>{formatRupiah(ledger.outstanding)}</b>
                  </span>
                </div>
                <div className="bill-card-actions">
                  <button
                    type="button"
                    disabled={ledger.isSettled || !canManagePayments}
                    onClick={() => setDialog({ kind: 'pay', residentId: ledger.residentId })}
                  >
                    <Wallet size={14} /> {ledger.isSettled ? 'Sudah lunas' : 'Bayar'}
                  </button>
                </div>
              </article>
            ))}
          </div>
        </>
      )}
      </>}

      {activeTab === 'history' && <section className="panel" style={{ marginTop: 16 }}>
        <div className="panel-heading"><div><h3>Riwayat pembayaran</h3><p>300 pembayaran terbaru · ubah nominal atau hapus catatan</p></div></div>
        {paymentError && <p className="form-error"><AlertCircle size={14}/>{paymentError}</p>}
        <div className="table-wrap"><table><thead><tr><th>Tanggal</th><th>Warga / KK</th><th>Catatan</th><th>Nominal</th><th>Dialokasikan</th><th>Aksi</th></tr></thead><tbody>
          {payments.map((payment) => <tr key={payment.id}>
            <td>{payment.received_at.replace('T',' ').slice(0,16)}</td><td><b>{payment.house_code}</b><small>{payment.head_name}</small></td><td>{payment.note || '—'}</td>
            <td>{editingPayment===payment.id ? <input aria-label="Nominal pembayaran" type="number" min="1" value={editingAmount} onChange={(event)=>setEditingAmount(event.target.value)}/> : formatRupiah(payment.amount)}</td>
            <td>{formatRupiah(payment.allocated)}</td>
            <td>{canManagePayments && <div className="payment-inline">{editingPayment===payment.id ? <><button className="text-link" onClick={()=>void savePaymentAmount(payment)}>Simpan</button><button className="text-link" onClick={()=>setEditingPayment(null)}>Batal</button></> : <><button className="text-link" aria-label={`Ubah pembayaran ${payment.house_code}`} onClick={()=>{setEditingPayment(payment.id);setEditingAmount(String(payment.amount));setPaymentError('')}}><Pencil size={14}/></button><button className="text-link" aria-label={`Hapus pembayaran ${payment.house_code}`} onClick={()=>void removePayment(payment)}><Trash2 size={14}/></button></>}</div>}</td>
          </tr>)}
          {!payments.length && <tr><td colSpan={6}>Belum ada pembayaran tercatat.</td></tr>}
        </tbody></table></div>
      </section>}

      {activeTab === 'finance' && <FinanceTab repository={repository} canManage={canManagePayments} onChanged={onChanged} onToast={onToast}/>}

      {dialog.kind === 'pay' && (
        <PaymentModal
          residents={residents}
          book={book}
          initialResidentId={dialog.residentId}
          repository={repository}
          onClose={close}
          onToast={onToast}
          onPaid={() => {
            onChanged();
            close();
          }}
        />
      )}
    </section>
  );
}

/* ------------------------------------------------------------------ */
/* Modal terima pembayaran                                             */
/* ------------------------------------------------------------------ */

function PaymentModal({
  residents,
  book,
  initialResidentId,
  repository,
  onClose,
  onToast,
  onPaid,
}: {
  residents: ResidentOption[];
  book: BillBook;
  initialResidentId?: number;
  repository: BillRepository;
  onClose: () => void;
  onToast: (message: string) => void;
  onPaid: () => void;
}) {
  const [residentId, setResidentId] = useState<string>(initialResidentId ? String(initialResidentId) : '');
  const [selected, setSelected] = useState<number[]>([]);
  const [amount, setAmount] = useState('');
  const [amountTouched, setAmountTouched] = useState(false);
  const [note, setNote] = useState('Pembayaran saat kumpulan');
  const [voluntary, setVoluntary] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const ledger = residentId ? book.ledgerFor(Number(residentId)) : null;
  const unpaid = ledger?.unpaid ?? [];
  const options = useMemo(
    () => [...residents].sort((a, b) => a.house_code.localeCompare(b.house_code)),
    [residents],
  );

  // Setiap ganti warga: pilih semua tagihan terbuka (default lunas semua).
  useEffect(() => {
    if (!ledger) {
      setSelected([]);
      setAmount('');
      setAmountTouched(false);
      setVoluntary(true);
      return;
    }
    const ids = ledger.unpaid.map((bill) => bill.id);
    setSelected(ids);
    setAmount(String(ledger.outstanding));
    setAmountTouched(false);
    setVoluntary(ids.length === 0);
  }, [residentId, book]);

  const selectedBills = unpaid.filter((bill) => selected.includes(bill.id));
  const selectedTotal = selectedBills.reduce((total, bill) => total + bill.remaining, 0);
  const projected = voluntary || amountTouched ? Number(amount || 0) : selectedTotal;
  const isFullPayment = projected >= selectedTotal && selectedTotal > 0;

  const toggle = (billId: number) => {
    setSelected((current) => {
      const next = current.includes(billId) ? current.filter((id) => id !== billId) : [...current, billId];
      const total = unpaid.filter((bill) => next.includes(bill.id)).reduce((sum, bill) => sum + bill.remaining, 0);
      if (!amountTouched) setAmount(String(total));
      return next;
    });
  };

  const selectAll = () => {
    const ids = unpaid.map((bill) => bill.id);
    setSelected(ids);
    if (!amountTouched) setAmount(String(unpaid.reduce((total, bill) => total + bill.remaining, 0)));
  };

  const clearAll = () => {
    setSelected([]);
    if (!amountTouched) setAmount('');
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!residentId) {
      setError('Pilih warga terlebih dahulu.');
      return;
    }
    if (!voluntary && selectedBills.length === 0) {
      setError('Pilih minimal satu tagihan yang akan dibayar.');
      return;
    }
    const nominal = Number(amount);
    if (!Number.isFinite(nominal) || nominal < 1) {
      setError('Nominal pembayaran wajib diisi.');
      return;
    }
    if (!voluntary && nominal > selectedTotal) {
      setError(`Nominal melebihi total tagihan terpilih (${formatRupiah(selectedTotal)}).`);
      return;
    }
    setSaving(true);
    setError('');
    try {
      const result = await repository.pay({
        residentId: Number(residentId),
        amount: nominal,
        note,
        billIds: selectedBills.map((bill) => bill.id),
        voluntary,
      });
      onToast(voluntary
        ? `Pembayaran sukarela ${formatRupiah(nominal)} dicatat tanpa tagihan.`
        : (
        isFullPayment
          ? `Pembayaran ${formatRupiah(nominal)} diterima, ${result.bills_paid} tagihan lunas.`
          : `Pembayaran ${formatRupiah(nominal)} diterima dan dialokasikan ke tagihan terlama.`
        ));
      onPaid();
    } catch (cause) {
      setError(cause instanceof BillError ? cause.message : 'Pembayaran gagal disimpan.');
      setSaving(false);
    }
  };

  return (
    <Modal
      title="Terima pembayaran"
      subtitle="Pilih warga, lalu bayar semua tagihan atau periode tertentu."
      onClose={onClose}
      size="medium"
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Batal
          </button>
          <button type="submit" form="payment-form" className="primary-btn" disabled={saving || !residentId}>
            {saving ? 'Menyimpan…' : `Bayar ${formatRupiah(projected)}`}
          </button>
        </>
      }
    >
      <form id="payment-form" className="form-grid" onSubmit={submit}>
        {error && (
          <p className="form-error span-2">
            <AlertCircle size={14} /> {error}
          </p>
        )}

        <label className="span-2">
          Warga / KK
          <select value={residentId} onChange={(event) => setResidentId(event.target.value)} required>
            <option value="">Pilih warga</option>
            {options.map((resident) => {
              const ledgerItem = book.ledgerFor(resident.id);
              const outstanding = ledgerItem?.outstanding ?? 0;
              return (
                <option key={resident.id} value={resident.id}>
                  {resident.house_code} — {resident.head_name}
                  {outstanding > 0 ? ` · sisa ${formatRupiah(outstanding)}` : ' · lunas'}
                </option>
              );
            })}
          </select>
        </label>

        {!residentId && (
          <p className="hint span-2">
            <Info size={13} style={{ flexShrink: 0, marginTop: 1 }} />
            Tagihan muncul otomatis ketika jadwal kumpulan ditandai terlaksana ({formatRupiah(20000)} per kumpulan).
          </p>
        )}

        {residentId && (
          <label className="span-2">
            <span><input type="checkbox" checked={voluntary} onChange={(event) => {
              const next = event.target.checked;
              setVoluntary(next);
              if (next) { setSelected([]); setAmount(''); setAmountTouched(true); }
              else { setSelected(unpaid.map((bill) => bill.id)); setAmount(String(ledger?.outstanding ?? 0)); setAmountTouched(false); }
            }} /> Catat sebagai pembayaran sukarela tanpa alokasi tagihan</span>
          </label>
        )}

        {!voluntary && ledger && unpaid.length === 0 && (
          <p className="form-success span-2">
            <CheckCircle2 size={14} /> Semua tagihan {ledger.houseCode} sudah lunas.
          </p>
        )}

        {!voluntary && ledger && unpaid.length > 0 && (
          <>
            <div className="span-2 pay-select-all">
              <span>
                <b>{unpaid.length} tagihan terbuka</b> · total {formatRupiah(ledger.outstanding)}
              </span>
              <div>
                <button type="button" className="chip-btn" onClick={selectAll}>
                  Pilih semua ({formatRupiah(ledger.outstanding)})
                </button>
                <button type="button" className="chip-btn" onClick={clearAll}>
                  Kosongkan
                </button>
              </div>
            </div>

            <div className="pay-bill-list span-2">
              {unpaid.map((bill) => {
                const checked = selected.includes(bill.id);
                return (
                  <label key={bill.id} className={`pay-bill${checked ? ' checked' : ''}`}>
                    <input type="checkbox" checked={checked} onChange={() => toggle(bill.id)} />
                    <span className="pay-bill-box">{checked && <Check size={12} />}</span>
                    <span className="pay-bill-info">
                      <b>Kumpulan {bill.meetingDateLabel}</b>
                      <small>{bill.description}</small>
                    </span>
                    <span className="pay-bill-amount">
                      <b>{bill.remainingLabel}</b>
                      {bill.paid > 0 && <small>dari {bill.amountLabel}</small>}
                    </span>
                  </label>
                );
              })}
            </div>

            <label>
              Nominal diterima (Rp)
              <input
                type="number"
                min="1"
                max={selectedTotal || undefined}
                value={amount}
                onChange={(event) => {
                  setAmountTouched(true);
                  setAmount(event.target.value);
                }}
                placeholder={String(selectedTotal || '')}
              />
            </label>
            <label>
              Catatan
              <input value={note} onChange={(event) => setNote(event.target.value)} placeholder="Opsional" />
            </label>

            <p className="hint span-2">
              {isFullPayment
                ? `Nominal sesuai total tagihan terpilih, seluruh tagihan akan lunas.`
                : `Nominal lebih kecil dari total terpilih (${formatRupiah(selectedTotal)}) — dicatat sebagai cicilan dan dialokasikan ke tagihan terlama.`}
            </p>
          </>
        )}
        {voluntary && residentId && (
          <>
            <p className="hint span-2">Pembayaran dicatat ke riwayat penerimaan tanpa melunasi atau membuat tagihan.</p>
            <label>
              Nominal diterima (Rp)
              <input type="number" min="1" value={amount} onChange={(event) => setAmount(event.target.value)} required />
            </label>
            <label>Catatan<input value={note} onChange={(event) => setNote(event.target.value)} placeholder="Contoh: iuran sukarela" /></label>
          </>
        )}
      </form>
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Utilitas tampilan                                                   */
/* ------------------------------------------------------------------ */

function FinanceTab({ repository, canManage, onChanged, onToast }: {
  repository: BillRepository; canManage: boolean; onChanged: () => void; onToast: (message: string) => void;
}) {
  const [dates, setDates] = useState<string[]>([]);
  const [receiptDate, setReceiptDate] = useState('');
  const [report, setReport] = useState<FinanceReport | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { repository.financeReceiptDates().then((items) => { setDates(items); if (items.length) setReceiptDate((current) => current || items[0]); }).catch((cause) => setError(cause instanceof Error ? cause.message : 'Tanggal penerimaan gagal dimuat.')); }, [repository]);
  const load = async (date = receiptDate) => {
    if (!date) { setReport(null); return; }
    setLoading(true); setError('');
    try { setReport(await repository.dailyFinance(date)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Rekap tidak dapat dimuat.'); setReport(null); }
    finally { setLoading(false); }
  };
  useEffect(() => { if (receiptDate) void load(receiptDate); }, [receiptDate]);
  const prepare = async () => {
    if (!receiptDate) return;
    setSaving(true); setError('');
    try { await repository.prepareDailyFinance(receiptDate); await load(receiptDate); onChanged(); onToast('Alokasi penerimaan kas sudah dibuat.'); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Alokasi gagal dibuat.'); }
    finally { setSaving(false); }
  };
  const confirm = async (key: string) => {
    try { await repository.confirmDailyFinance(receiptDate,key); await load(receiptDate); onToast('Penerimaan bidang dikonfirmasi.'); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Konfirmasi gagal.'); }
  };
  const locked = report?.allocations.some((allocation)=>allocation.confirmed) ?? false;
  return <section className="panel" style={{ marginTop: 16 }}>
    <div className="panel-heading"><div><h3>Rekap dan alokasi penerimaan kas</h3><p>Penerimaan pada tanggal terpilih dibagi sekali, lalu tiap pengurus bidang mengonfirmasi penerimaan.</p></div></div>
    {!dates.length ? <div className="empty"><p>Belum ada pembayaran yang tercatat.</p></div> : <>
      <label className="span-2">Tanggal penerimaan pembayaran
        <select value={receiptDate} onChange={(event)=>setReceiptDate(event.target.value)}>{dates.map(date=><option key={date} value={date}>{date}</option>)}</select>
      </label>
      {error && <p className="form-error"><AlertCircle size={14}/>{error}</p>}
      {loading ? <p>Memuat rekap...</p> : report && <>
        <div className="bills-summary" style={{marginTop:14}}>
          <div><span>Total penerimaan</span><b>{formatRupiah(report.finance.total_collected)}</b><small>{report.finance.payment_count} transaksi pada {receiptDate}</small></div>
          <div><span>Alokasi bidang</span><b>{formatRupiah(report.required_amount)}</b><small>Konsumsi, sosial, jalan, baitul mal, kemasjidan</small></div>
          <div><span>Kas Bendahara Umum</span><b>{formatRupiah(report.finance.kas_rt)}</b><small>Sisa setelah pembagian bidang</small></div>
          <div><span>Status</span><b>{report.prepared?'Sudah dialokasikan':'Belum dialokasikan'}</b><small>{report.shortfall>0?`Kurang ${formatRupiah(report.shortfall)}`:'Dana alokasi mencukupi'}</small></div>
        </div>
        {report.current_total!==report.finance.total_collected && <p className="form-error">Total penerimaan pada {receiptDate} berubah menjadi {formatRupiah(report.current_total)} setelah alokasi disiapkan. {locked?'Sebagian bidang sudah mengonfirmasi, jadi alokasi terkunci.':'Perbarui alokasi sebelum meminta konfirmasi bidang.'}</p>}
        {canManage && <div style={{display:'flex',justifyContent:'flex-end',margin:'14px 0'}}><button className="primary-btn" type="button" disabled={saving||report.shortfall>0||locked} onClick={()=>void prepare()}>{saving?'Memproses...':locked?'Alokasi telah dikonfirmasi':report.prepared?'Perbarui alokasi':'Bagikan penerimaan sesuai alokasi'}</button></div>}
        {report.prepared && <div className="table-wrap"><table><thead><tr><th>Alokasi</th><th>Bidang penerima</th><th>Jumlah</th><th>Konfirmasi</th><th>Aksi</th></tr></thead><tbody>
          {report.allocations.map((allocation)=><tr key={allocation.key}><td><b>{allocation.label}</b></td><td>{allocation.field}</td><td>{formatRupiah(allocation.amount)}</td><td>{allocation.confirmed?<span className="status done">Diterima<br/><small>{allocation.confirmed_by} · {allocation.confirmed_at.replace('T',' ').slice(0,16)}</small></span>:<span className="status off">Menunggu penerimaan</span>}</td><td>{!allocation.confirmed&&allocation.can_confirm&&<button type="button" className="primary-btn" onClick={()=>void confirm(allocation.key)}>Konfirmasi diterima</button>}</td></tr>)}
        </tbody></table></div>}
        {!report.prepared && <p className="hint">Alokasi tetap Rp640.000 untuk pembayaran yang diterima pada tanggal terpilih. Sisa masuk Kas Bendahara Umum. Proses alokasi tersedia setelah penerimaan hari itu mencapai Rp640.000.</p>}
      </>}
    </>}
  </section>;
}

function initialsOf(headName: string, houseCode: string): string {
  const initials = headName
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? '')
    .join('');
  return initials || houseCode.slice(-1);
}
