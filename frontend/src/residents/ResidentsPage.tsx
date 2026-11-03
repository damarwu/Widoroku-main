import { useMemo, useState, type FormEvent } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import {
  AlertCircle,
  Eye,
  Hash,
  Info,
  MapPin,
  Pencil,
  Phone,
  Plus,
  Printer,
  QrCode,
  Search,
  Users,
  Wallet,
  X,
} from 'lucide-react';
import Resident, { SAVINGS_CLASS_OPTIONS, type ResidentInput, type ResidentJSON } from './Resident';
import ResidentRepository, { ResidentError } from './ResidentRepository';
import Modal from '../shared/Modal';
import '../shared/modal.css';
import '../shared/list.css';
import './residents.css';

type Dialog =
  | { kind: 'none' }
  | { kind: 'create' }
  | { kind: 'edit'; resident: Resident }
  | { kind: 'detail'; resident: Resident }
  | { kind: 'qr'; resident: Resident };

export interface ResidentsPageProps {
  /** Data mentah dari App (sumber tunggal agar halaman lain ikut ter-update). */
  residents: ResidentJSON[];
  /** Dipanggil setiap ada perubahan data agar seluruh aplikasi menyegarkan diri. */
  onChanged: () => void;
  onToast: (message: string) => void;
  canManage: boolean;
}

const money = (value: number) =>
  new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(value);

/**
 * Halaman Data Warga.
 *
 * Desktop  : datatable (tabel penuh + pencarian cepat).
 * Mobile   : daftar card yang nyaman disentuh.
 * Keduanya memakai model Resident yang sama, jadi perilaku konsisten.
 */
export default function ResidentsPage({ residents, onChanged, onToast, canManage }: ResidentsPageProps) {
  const repository = useMemo(() => new ResidentRepository(), []);
  const [query, setQuery] = useState('');
  const [dialog, setDialog] = useState<Dialog>({ kind: 'none' });

  const models = useMemo(() => Resident.fromList(residents), [residents]);
  const filtered = useMemo(() => models.filter((resident) => resident.matches(query)), [models, query]);
  const close = () => setDialog({ kind: 'none' });

  const handleSaved = (message: string) => {
    onToast(message);
    onChanged();
    close();
  };

  return (
    <section className="panel">
      <div className="panel-heading">
        <div>
          <h3>Data warga &amp; rumah</h3>
          <p>{models.length} kepala keluarga terdaftar · QR token dipakai untuk scan jimpitan</p>
        </div>
        {canManage && (
          <button type="button" className="primary-btn" onClick={() => setDialog({ kind: 'create' })}>
            <Plus size={16} /> Tambah warga
          </button>
        )}
      </div>

      <div className="residents-toolbar">
        <div className="residents-search">
          <Search />
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Cari kode rumah, nama, alamat, atau nomor WA…"
            aria-label="Cari warga"
          />
        </div>
        <span className="residents-count">
          {filtered.length === models.length ? `${models.length} rumah` : `${filtered.length} dari ${models.length} rumah`}
        </span>
      </div>

      {filtered.length === 0 ? (
        <div className="empty">
          <div>
            <Users size={22} />
          </div>
          <p>{models.length === 0 ? 'Belum ada data warga. Tambahkan rumah pertama.' : 'Tidak ada warga yang cocok dengan pencarian.'}</p>
        </div>
      ) : (
        <>
          {/* Desktop: datatable */}
          <div className="residents-table-wrap">
            <table className="residents-table">
              <thead>
                <tr>
                  <th>Rumah / KK</th>
                  <th>Alamat</th>
                  <th>WhatsApp</th>
                  <th>Kelas tabungan</th>
                  <th>Jenis iuran</th>
                  <th>Aksi</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((resident) => (
                  <tr key={resident.id}>
                    <td>
                      <div className="resident-identity">
                        <span className="resident-avatar">{resident.initials}</span>
                        <div>
                          <b>{resident.houseCode}</b>
                          <small>{resident.headName}</small>
                        </div>
                      </div>
                    </td>
                    <td>{resident.address || <span className="resident-chip muted">Belum diisi</span>}</td>
                    <td>
                      {resident.whatsappLink ? (
                        <a className="text-link" href={resident.whatsappLink} target="_blank" rel="noreferrer">
                          {resident.phone}
                        </a>
                      ) : (
                        <span className="resident-chip muted">Belum ada</span>
                      )}
                    </td>
                    <td>
                      <span className="resident-chip">
                        <Wallet size={11} /> {resident.savingsLabel}
                      </span>
                    </td>
                    <td><span className={`resident-chip${resident.duesRequired ? '' : ' muted'}`}>{resident.duesRequired ? 'Wajib iuran' : 'Khusus · bebas iuran'}</span></td>
                    <td>
                      <div className="resident-actions">
                        <button
                          type="button"
                          className="resident-action"
                          title="Lihat detail"
                          aria-label={`Detail ${resident.houseCode}`}
                          onClick={() => setDialog({ kind: 'detail', resident })}
                        >
                          <Eye />
                        </button>
                        {canManage && (
                          <button
                            type="button"
                            className="resident-action"
                            title="Ubah data"
                            aria-label={`Ubah ${resident.houseCode}`}
                            onClick={() => setDialog({ kind: 'edit', resident })}
                          >
                            <Pencil />
                          </button>
                        )}
                        <button
                          type="button"
                          className="resident-action"
                          title="QR rumah"
                          aria-label={`QR ${resident.houseCode}`}
                          onClick={() => setDialog({ kind: 'qr', resident })}
                        >
                          <QrCode />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Mobile: card */}
          <div className="residents-cards">
            {filtered.map((resident) => (
              <article className="resident-card" key={resident.id}>
                <div className="resident-card-head">
                  <span className="resident-avatar large">{resident.initials}</span>
                  <div>
                    <b>{resident.houseCode}</b>
                    <small>{resident.headName}</small>
                  </div>
                  <span className="resident-chip">Kelas {resident.savingsClass}</span>
                </div>
                <div className="resident-card-meta">
                  <span>
                    <MapPin /> {resident.address || 'Alamat belum diisi'}
                  </span>
                  <span>
                    <Phone /> {resident.phone || 'Nomor WA belum diisi'}
                  </span>
                  <span>
                    <Wallet /> {resident.savingsClass===4?'Tidak wajib tabungan jalan':`${money(resident.savingsTariff)} per tahap`}
                  </span>
                </div>
                <div className="resident-card-actions">
                  <button type="button" onClick={() => setDialog({ kind: 'detail', resident })}>
                    <Eye /> Detail
                  </button>
                  <button type="button" onClick={() => setDialog({ kind: 'qr', resident })}>
                    <QrCode /> QR
                  </button>
                  <button
                    type="button"
                    onClick={() => (canManage ? setDialog({ kind: 'edit', resident }) : setDialog({ kind: 'detail', resident }))}
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
        <ResidentFormModal
          mode="create"
          onClose={close}
          onSubmit={async (input) => {
            await repository.create(input);
            handleSaved('Warga baru berhasil ditambahkan');
          }}
        />
      )}

      {dialog.kind === 'edit' && (
        <ResidentFormModal
          mode="edit"
          resident={dialog.resident}
          onClose={close}
          onSubmit={async (input) => {
            await repository.update(dialog.resident.id, input);
            handleSaved(`Data ${input.house_code} berhasil diperbarui`);
          }}
        />
      )}

      {dialog.kind === 'detail' && (
        <ResidentDetailModal
          resident={dialog.resident}
          canManage={canManage}
          onClose={close}
          onEdit={() => setDialog({ kind: 'edit', resident: dialog.resident })}
          onShowQr={() => setDialog({ kind: 'qr', resident: dialog.resident })}
        />
      )}

      {dialog.kind === 'qr' && <ResidentQrModal resident={dialog.resident} onClose={close} />}
    </section>
  );
}

/* ------------------------------------------------------------------ */
/* Modal bingkai                                                       */
/* ------------------------------------------------------------------ */

/* ------------------------------------------------------------------ */
/* Modal tambah / ubah                                                 */
/* ------------------------------------------------------------------ */

const emptyInput: ResidentInput = {
  house_code: '',
  head_name: '',
  address: '',
  qr_token: '',
  phone: '',
  savings_class: 3,
  dues_required: true,
};

function ResidentFormModal({
  mode,
  resident,
  onClose,
  onSubmit,
}: {
  mode: 'create' | 'edit';
  resident?: Resident;
  onClose: () => void;
  onSubmit: (input: ResidentInput) => Promise<void>;
}) {
  const [form, setForm] = useState<ResidentInput>(
    resident
      ? {
          house_code: resident.houseCode,
          head_name: resident.headName,
          address: resident.address,
          qr_token: resident.qrToken,
          phone: resident.phone,
          savings_class: resident.savingsClass,
          dues_required: resident.duesRequired,
        }
      : emptyInput,
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const update = <K extends keyof ResidentInput>(key: K, value: ResidentInput[K]) =>
    setForm((current) => ({ ...current, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!form.house_code.trim() || !form.head_name.trim()) {
      setError('Kode rumah dan nama kepala keluarga wajib diisi.');
      return;
    }
    setSaving(true);
    setError('');
    try {
      await onSubmit({ ...form, house_code: form.house_code.trim(), head_name: form.head_name.trim() });
    } catch (cause) {
      setError(cause instanceof ResidentError ? cause.message : 'Data gagal disimpan.');
      setSaving(false);
    }
  };

  return (
    <Modal
      title={mode === 'create' ? 'Tambah warga / rumah' : `Ubah data ${resident?.houseCode ?? ''}`}
      subtitle={mode === 'create' ? 'Satu rumah diwakili satu kepala keluarga dan satu QR token.' : 'Perubahan kode rumah akan mengubah label QR yang tercetak.'}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Batal
          </button>
          <button type="submit" form="resident-form" className="primary-btn" disabled={saving}>
            {saving ? 'Menyimpan…' : mode === 'create' ? 'Simpan warga' : 'Simpan perubahan'}
          </button>
        </>
      }
    >
      <form id="resident-form" className="form-grid" onSubmit={submit}>
        {error && (
          <p className="form-error span-2">
            <AlertCircle size={14} /> {error}
          </p>
        )}
        <label>
          Kode rumah
          <input
            required
            value={form.house_code}
            onChange={(event) => update('house_code', event.target.value)}
            placeholder="Contoh: RT03-025"
            autoFocus
          />
        </label>
        <label>
          Nama kepala keluarga
          <input
            required
            value={form.head_name}
            onChange={(event) => update('head_name', event.target.value)}
            placeholder="Nama lengkap"
          />
        </label>
        <label className="span-2">
          Alamat rumah
          <input
            value={form.address}
            onChange={(event) => update('address', event.target.value)}
            placeholder="Nama jalan / nomor rumah"
          />
        </label>
        <label>
          Nomor WhatsApp
          <input
            value={form.phone}
            onChange={(event) => update('phone', event.target.value)}
            placeholder="081234567890"
            inputMode="tel"
          />
        </label>
        <label>
          Kelas tabungan jalan
          <select value={form.savings_class} onChange={(event) => update('savings_class', Number(event.target.value))}>
            {SAVINGS_CLASS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Jenis warga
          <select value={form.dues_required ? 'wajib' : 'khusus'} onChange={(event) => update('dues_required', event.target.value === 'wajib')}>
            <option value="wajib">Wajib iuran kumpulan</option>
            <option value="khusus">Khusus · tidak wajib iuran</option>
          </select>
        </label>
        <label className="span-2">
          Token QR (opsional)
          <input
            value={form.qr_token}
            onChange={(event) => update('qr_token', event.target.value)}
            placeholder={form.house_code ? `WDR-${form.house_code.toUpperCase().replace(/\s+/g, '-')}` : 'Otomatis dari kode rumah'}
          />
          <span className="hint">Kosongkan agar sistem membuat token dari kode rumah.</span>
        </label>
      </form>
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Modal detail                                                        */
/* ------------------------------------------------------------------ */

function ResidentDetailModal({
  resident,
  canManage,
  onClose,
  onEdit,
  onShowQr,
}: {
  resident: Resident;
  canManage: boolean;
  onClose: () => void;
  onEdit: () => void;
  onShowQr: () => void;
}) {
  return (
    <Modal
      title="Detail warga"
      subtitle={`Data rumah ${resident.houseCode}`}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Tutup
          </button>
          <button type="button" className="ghost-btn" onClick={onShowQr}>
            <QrCode size={15} /> QR rumah
          </button>
          {canManage && (
            <button type="button" className="primary-btn" onClick={onEdit}>
              <Pencil size={15} /> Ubah data
            </button>
          )}
        </>
      }
    >
      <div className="detail-hero">
        <span className="resident-avatar large">{resident.initials}</span>
        <div>
          <b>{resident.headName}</b>
          <small>
            {resident.houseCode} · {resident.savingsLabel}
          </small>
        </div>
      </div>

      <div className="detail-grid">
        <div className="detail-item wide">
          <span>Alamat</span>
          <b>{resident.address || 'Belum diisi'}</b>
        </div>
        <div className="detail-item">
          <span>WhatsApp</span>
          <b>{resident.whatsappLink ? <a className="text-link" href={resident.whatsappLink} target="_blank" rel="noreferrer">{resident.phone}</a> : 'Belum diisi'}</b>
        </div>
        <div className="detail-item">
          <span>Kelas tabungan</span>
          <b>{resident.savingsLabel}</b>
        </div>
        <div className="detail-item wide">
          <span>Token QR</span>
          <b>
            <code>{resident.qrToken}</code>
          </b>
        </div>
      </div>

      <div className="detail-qr">
        <QRCodeSVG value={resident.qrToken} size={116} level="H" />
      </div>
      <p className="hint" style={{ display: 'flex', gap: 6 }}>
        <Info size={13} style={{ flexShrink: 0, marginTop: 1 }} />
        QR ini dipindai petugas jimpitan lewat aplikasi mobile.
      </p>
    </Modal>
  );
}

/* ------------------------------------------------------------------ */
/* Modal QR                                                            */
/* ------------------------------------------------------------------ */

function ResidentQrModal({ resident, onClose }: { resident: Resident; onClose: () => void }) {
  return (
    <Modal
      title="QR rumah warga"
      subtitle={`${resident.houseCode} · ${resident.headName}`}
      onClose={onClose}
      size="small"
      footer={
        <>
          <button type="button" className="ghost-btn" onClick={onClose}>
            Tutup
          </button>
          <button type="button" className="primary-btn" onClick={() => window.print()}>
            <Printer size={16} /> Cetak
          </button>
        </>
      }
    >
      <div className="qr-sheet" id="qr-print-area">
        <div className="qr-code">
          <QRCodeSVG value={resident.qrToken} size={190} level="H" />
        </div>
        <div className="qr-house">{resident.houseCode}</div>
        <div className="qr-head">{resident.headName}</div>
        <div className="qr-token">{resident.qrToken}</div>
      </div>
      <p className="qr-note">
        <Hash size={14} />
        Tempel label ini di depan rumah. Satu QR mewakili satu rumah dan dipakai saat pencatatan jimpitan.
      </p>
    </Modal>
  );
}
