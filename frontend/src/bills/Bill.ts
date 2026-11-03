/**
 * Model domain tagihan iuran kumpulan dan pembayarannya.
 */

export interface BillJSON {
  id: number;
  resident_id: number;
  house_code: string;
  head_name: string;
  period_id: number;
  meeting_date: string;
  period_status: string;
  description: string;
  amount: number;
  paid: number;
  remaining: number;
}

export interface ResidentOption {
  id: number;
  house_code: string;
  head_name: string;
}

export const formatRupiah = (value: number) =>
  new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(value);

const MONTHS_SHORT = ['JAN', 'FEB', 'MAR', 'APR', 'MEI', 'JUN', 'JUL', 'AGU', 'SEP', 'OKT', 'NOV', 'DES'];

/** Satu baris tagihan milik satu warga untuk satu kumpulan. */
export class Bill {
  readonly id: number;
  readonly residentId: number;
  readonly houseCode: string;
  readonly headName: string;
  readonly periodId: number;
  readonly meetingDate: string;
  readonly periodStatus: string;
  readonly description: string;
  readonly amount: number;
  readonly paid: number;
  readonly remaining: number;

  constructor(data: BillJSON) {
    this.id = Number(data.id);
    this.residentId = Number(data.resident_id);
    this.houseCode = data.house_code ?? '';
    this.headName = data.head_name ?? '';
    this.periodId = Number(data.period_id);
    this.meetingDate = data.meeting_date ?? '';
    this.periodStatus = (data.period_status ?? '').toUpperCase();
    this.description = data.description ?? 'Iuran kumpulan';
    this.amount = Number(data.amount ?? 0);
    this.paid = Number(data.paid ?? 0);
    this.remaining = Number(data.remaining ?? Math.max(this.amount - this.paid, 0));
  }

  static fromJSON(data: BillJSON): Bill {
    return new Bill(data);
  }

  static fromList(data: BillJSON[]): Bill[] {
    return data.map((item) => Bill.fromJSON(item));
  }

  get isPaid(): boolean {
    return this.remaining <= 0;
  }

  get isPartial(): boolean {
    return this.paid > 0 && this.remaining > 0;
  }

  get statusLabel(): string {
    if (this.isPaid) return 'Lunas';
    return this.isPartial ? 'Cicilan' : 'Belum bayar';
  }

  get tone(): 'done' | 'pending' | 'off' {
    if (this.isPaid) return 'done';
    return this.isPartial ? 'pending' : 'off';
  }

  /** "12 Jan 2025" untuk kolom kumpulan. */
  get meetingDateLabel(): string {
    const date = new Date(`${this.meetingDate}T12:00:00`);
    if (Number.isNaN(date.getTime())) return this.meetingDate;
    return date.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' });
  }

  get dayNumber(): string {
    return this.meetingDate.slice(8, 10).replace(/^0/, '');
  }

  get monthShort(): string {
    const month = Number(this.meetingDate.slice(5, 7));
    return MONTHS_SHORT[month - 1] ?? '—';
  }

  get amountLabel(): string {
    return formatRupiah(this.amount);
  }

  get paidLabel(): string {
    return formatRupiah(this.paid);
  }

  get remainingLabel(): string {
    return formatRupiah(this.remaining);
  }

  matches(keyword: string): boolean {
    const needle = keyword.trim().toLowerCase();
    if (!needle) return true;
    return [this.houseCode, this.headName, this.description, this.meetingDate, this.meetingDateLabel, this.statusLabel]
      .some((value) => value.toLowerCase().includes(needle));
  }
}

/** Ringkasan keuangan satu warga. */
export class ResidentLedger {
  readonly residentId: number;
  readonly houseCode: string;
  readonly headName: string;
  readonly bills: Bill[];

  constructor(residentId: number, houseCode: string, headName: string, bills: Bill[]) {
    this.residentId = residentId;
    this.houseCode = houseCode;
    this.headName = headName;
    this.bills = bills;
  }

  get unpaid(): Bill[] {
    return this.bills.filter((bill) => !bill.isPaid);
  }

  get paid(): Bill[] {
    return this.bills.filter((bill) => bill.isPaid);
  }

  get outstanding(): number {
    return this.unpaid.reduce((total, bill) => total + bill.remaining, 0);
  }

  get isSettled(): boolean {
    return this.outstanding === 0;
  }

  get billed(): number {
    return this.bills.reduce((total, bill) => total + bill.amount, 0);
  }

  get paidTotal(): number {
    return this.bills.reduce((total, bill) => total + bill.paid, 0);
  }

  /** Tagihan diurutkan dari kumpulan terlama. */
  get sortedBills(): Bill[] {
    return [...this.bills].sort((a, b) => a.meetingDate.localeCompare(b.meetingDate));
  }

  /** "Jan 2025 – Des 2029" atau satu tanggal bila hanya satu kumpulan. */
  get periodRangeLabel(): string {
    const list = this.sortedBills;
    if (list.length === 0) return '—';
    const first = list[0];
    const last = list[list.length - 1];
    if (first.meetingDateLabel === last.meetingDateLabel) return first.meetingDateLabel;
    return `${first.meetingDateLabel} – ${last.meetingDateLabel}`;
  }

  get initials(): string {
    return this.headName
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() ?? '')
      .join('') || this.houseCode.slice(-1);
  }

  matches(keyword: string): boolean {
    const needle = keyword.trim().toLowerCase();
    if (!needle) return true;
    return [this.houseCode, this.headName].some((value) => value.toLowerCase().includes(needle));
  }
}

/**
 * Kumpulan seluruh tagihan: menyediakan rekap dan pengelompokan per warga
 * sehingga komponen tampilan tidak perlu menghitung manual.
 */
export class BillBook {
  readonly bills: Bill[];

  constructor(bills: Bill[]) {
    this.bills = bills;
  }

  static fromList(data: BillJSON[]): BillBook {
    return new BillBook(Bill.fromList(data));
  }

  get totalBilled(): number {
    return this.bills.reduce((total, bill) => total + bill.amount, 0);
  }

  get totalPaid(): number {
    return this.bills.reduce((total, bill) => total + bill.paid, 0);
  }

  get outstanding(): number {
    return this.bills.reduce((total, bill) => total + bill.remaining, 0);
  }

  get paidCount(): number {
    return this.bills.filter((bill) => bill.isPaid).length;
  }

  /** Warga yang masih punya tunggakan. */
  get ledgers(): ResidentLedger[] {
    const grouped = new Map<number, ResidentLedger>();
    for (const bill of this.bills) {
      const existing = grouped.get(bill.residentId);
      if (existing) {
        existing.bills.push(bill);
        continue;
      }
      grouped.set(bill.residentId, new ResidentLedger(bill.residentId, bill.houseCode, bill.headName, [bill]));
    }
    return [...grouped.values()].sort((a, b) => a.houseCode.localeCompare(b.houseCode));
  }

  ledgerFor(residentId: number): ResidentLedger | null {
    return this.ledgers.find((ledger) => ledger.residentId === residentId) ?? null;
  }

  get debtors(): ResidentLedger[] {
    return this.ledgers.filter((ledger) => !ledger.isSettled);
  }

  get debtorCount(): number {
    return this.debtors.length;
  }

  filter(query: string, mode: 'all' | 'unpaid' | 'paid'): Bill[] {
    return this.bills.filter((bill) => {
      if (mode === 'unpaid' && bill.isPaid) return false;
      if (mode === 'paid' && !bill.isPaid) return false;
      return bill.matches(query);
    });
  }

  /** Daftar warga (satu baris per KK) siap ditampilkan pada tabel/kartu. */
  filterLedgers(query: string, mode: 'all' | 'unpaid' | 'paid'): ResidentLedger[] {
    return this.ledgers.filter((ledger) => {
      if (mode === 'unpaid' && ledger.isSettled) return false;
      if (mode === 'paid' && !ledger.isSettled) return false;
      return ledger.matches(query);
    });
  }
}

export default Bill;
