/**
 * Model domain warga/rumah.
 *
 * Semua perilaku yang berkaitan dengan satu data warga (format tampilan,
 * pencarian, tautan WhatsApp, payload API) berada di kelas ini supaya
 * komponen tampilan tidak perlu tahu detailnya.
 */
export interface ResidentJSON {
  id: number;
  house_code: string;
  head_name: string;
  address: string;
  qr_token: string;
  phone: string;
  savings_class: number;
  dues_required?: boolean;
}

export interface ResidentInput {
  house_code: string;
  head_name: string;
  address: string;
  qr_token: string;
  phone: string;
  savings_class: number;
  dues_required: boolean;
}

const SAVINGS_TARIFF: Record<number, number> = { 1: 75000, 2: 50000, 3: 25000, 4: 0 };

export const SAVINGS_CLASS_OPTIONS = [
  { value: 1, label: 'Kelas 1 · Rp75.000/tahap' },
  { value: 2, label: 'Kelas 2 · Rp50.000/tahap' },
  { value: 3, label: 'Kelas 3 · Rp25.000/tahap' },
  { value: 4, label: 'Kelas 4 · Tidak wajib ikut tabungan jalan' },
];

export class Resident {
  readonly id: number;
  readonly houseCode: string;
  readonly headName: string;
  readonly address: string;
  readonly qrToken: string;
  readonly phone: string;
  readonly savingsClass: number;
  readonly duesRequired: boolean;

  constructor(id: number, houseCode: string, headName: string, address: string, qrToken: string, phone: string, savingsClass: number, duesRequired = true) {
    this.id = id;
    this.houseCode = houseCode;
    this.headName = headName;
    this.address = address;
    this.qrToken = qrToken;
    this.phone = phone;
    this.savingsClass = savingsClass;
    this.duesRequired = duesRequired;
  }

  static fromJSON(data: ResidentJSON): Resident {
    return new Resident(
      Number(data.id),
      data.house_code ?? '',
      data.head_name ?? '',
      data.address ?? '',
      data.qr_token ?? '',
      data.phone ?? '',
      Number(data.savings_class ?? 3),
      data.dues_required !== false,
    );
  }

  static fromList(data: ResidentJSON[]): Resident[] {
    return data.map((item) => Resident.fromJSON(item));
  }

  /** Inisial kepala keluarga, maksimal dua huruf. */
  get initials(): string {
    const parts = this.headName.trim().split(/\s+/).filter(Boolean);
    if (!parts.length) return '?';
    return parts.slice(0, 2).map((part) => part[0].toUpperCase()).join('');
  }

  /** Tarif tabungan jalan per tahap sesuai kelas. */
  get savingsTariff(): number {
    return SAVINGS_TARIFF[this.savingsClass] ?? 0;
  }

  /** Label tarif siap tampil, mis. "Kelas 2 · Rp50.000". */
  get savingsLabel(): string {
    if (this.savingsClass === 4) return 'Kelas 4 · Tidak wajib tabungan jalan';
    const formatted = new Intl.NumberFormat('id-ID', {
      style: 'currency',
      currency: 'IDR',
      maximumFractionDigits: 0,
    }).format(this.savingsTariff);
    return `Kelas ${this.savingsClass} · ${formatted}`;
  }

  /** Nomor WhatsApp ternormalisasi (62…), null bila kosong/tidak valid. */
  get whatsappNumber(): string | null {
    const digits = this.phone.replace(/\D/g, '');
    // "0" atau nomor terlalu pendek adalah placeholder, bukan nomor nyata.
    if (digits.length < 8) return null;
    return digits.startsWith('0') ? `62${digits.slice(1)}` : digits;
  }

  /** Tautan wa.me, null bila nomor tidak tersedia. */
  get whatsappLink(): string | null {
    const number = this.whatsappNumber;
    return number ? `https://wa.me/${number}` : null;
  }

  /** True bila kata kunci cocok dengan kode rumah, nama, alamat, telepon, atau token QR. */
  matches(keyword: string): boolean {
    const needle = keyword.trim().toLowerCase();
    if (!needle) return true;
    return [this.houseCode, this.headName, this.address, this.phone, this.qrToken]
      .some((value) => value.toLowerCase().includes(needle));
  }

  /** Salinan dengan id baru (hasil penyimpanan). */
  withId(id: number): Resident {
    return new Resident(id, this.houseCode, this.headName, this.address, this.qrToken, this.phone, this.savingsClass, this.duesRequired);
  }

  /** Payload untuk POST/PUT /api/residents. */
  toPayload(): ResidentInput {
    return {
      house_code: this.houseCode,
      head_name: this.headName,
      address: this.address,
      qr_token: this.qrToken,
      phone: this.phone,
      savings_class: this.savingsClass,
      dues_required: this.duesRequired,
    };
  }
}

export default Resident;
