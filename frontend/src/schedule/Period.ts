/**
 * Model domain jadwal kumpulan (Ahad Pon).
 */

export type PeriodStatus = 'TERJADWAL' | 'TERLAKSANA' | 'LIBUR';

export interface MeetingFinanceJSON {
  period_id: number;
  payment_count: number;
  total_collected: number;
  penerangan_jalan: number;
  sosial: number;
  kemasjidan: number;
  ziswaf: number;
  host_consumption: number;
  kas_rt: number;
}

export interface PeriodJSON {
  id: number;
  start_date: string;
  meeting_date: string;
  meeting_time: string;
  status: string;
  note: string;
  finance?: MeetingFinanceJSON | null;
  /** Jumlah tagihan periode ini (dihitung backend). */
  bills_count?: number;
  /** Tagihan yang belum lunas — akan dihapus bila kumpulan dibatalkan. */
  bills_unpaid?: number;
  bills_unpaid_amount?: number;
}

export interface PeriodInput {
  start_date: string;
  meeting_date: string;
  meeting_time: string;
  status: PeriodStatus;
  note: string;
}

export const PERIOD_STATUS_OPTIONS: { value: PeriodStatus; label: string }[] = [
  { value: 'TERJADWAL', label: 'Terjadwal' },
  { value: 'TERLAKSANA', label: 'Terlaksana' },
  { value: 'LIBUR', label: 'Libur' },
];

export const AUTO_SCHEDULE_NOTE = 'Jadwal Ahad Pon otomatis';

export const formatMoney = (value: number) =>
  new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(value);

/** Rincian kas hasil kumpulan malam Ahad Pon. */
export class MeetingFinance {
  readonly periodId: number;
  readonly paymentCount: number;
  readonly totalCollected: number;
  readonly peneranganJalan: number;
  readonly sosial: number;
  readonly kemasjidan: number;
  readonly ziswaf: number;
  readonly hostConsumption: number;
  readonly kasRt: number;

  constructor(data: MeetingFinanceJSON) {
    this.periodId = Number(data.period_id);
    this.paymentCount = Number(data.payment_count ?? 0);
    this.totalCollected = Number(data.total_collected ?? 0);
    this.peneranganJalan = Number(data.penerangan_jalan ?? 0);
    this.sosial = Number(data.sosial ?? 0);
    this.kemasjidan = Number(data.kemasjidan ?? 0);
    this.ziswaf = Number(data.ziswaf ?? 0);
    this.hostConsumption = Number(data.host_consumption ?? 0);
    this.kasRt = Number(data.kas_rt ?? 0);
  }

  /** Total alokasi pos (di luar kas RT). */
  get allocationTotal(): number {
    return this.peneranganJalan + this.sosial + this.kemasjidan + this.ziswaf + this.hostConsumption;
  }

  /** Pos alokasi tetap per kumpulan; sisanya masuk kas Bendahara Umum. */
  get breakdown(): { label: string; amount: number }[] {
    return [
      { label: 'Konsumsi', amount: this.hostConsumption },
      { label: 'Dana sosial', amount: this.sosial },
      { label: 'Penerangan jalan', amount: this.peneranganJalan },
      { label: 'Baitul mal', amount: this.ziswaf },
      { label: 'Kemasjidan', amount: this.kemasjidan },
    ];
  }

}

export class Period {
  readonly id: number;
  readonly startDate: string;
  readonly meetingDate: string;
  readonly meetingTime: string;
  readonly status: string;
  readonly note: string;
  readonly finance: MeetingFinance | null;
  readonly billsCount: number;
  readonly billsUnpaidCount: number;
  readonly billsUnpaidAmount: number;

  constructor(
    id: number,
    startDate: string,
    meetingDate: string,
    meetingTime: string,
    status: string,
    note: string,
    finance: MeetingFinance | null,
    billsCount = 0,
    billsUnpaidCount = 0,
    billsUnpaidAmount = 0,
  ) {
    this.id = id;
    this.startDate = startDate;
    this.meetingDate = meetingDate;
    this.meetingTime = meetingTime;
    this.status = status;
    this.note = note;
    this.finance = finance;
    this.billsCount = billsCount;
    this.billsUnpaidCount = billsUnpaidCount;
    this.billsUnpaidAmount = billsUnpaidAmount;
  }

  static fromJSON(data: PeriodJSON): Period {
    return new Period(
      Number(data.id),
      data.start_date ?? '',
      data.meeting_date ?? '',
      data.meeting_time ?? '',
      (data.status ?? 'TERJADWAL').toUpperCase(),
      data.note ?? '',
      data.finance ? new MeetingFinance(data.finance) : null,
      Number(data.bills_count ?? 0),
      Number(data.bills_unpaid ?? 0),
      Number(data.bills_unpaid_amount ?? 0),
    );
  }

  static fromList(data: PeriodJSON[]): Period[] {
    return data.map((item) => Period.fromJSON(item));
  }

  /** "19:30" dari "19:30:00". */
  get clock(): string {
    return this.meetingTime.slice(0, 5);
  }

  get timeLabel(): string {
    return `${this.clock} WIB`;
  }

  /** Tanggal panjang, mis. "Minggu, 12 Januari 2025". */
  get meetingDateLabel(): string {
    const date = new Date(`${this.meetingDate}T12:00:00`);
    if (Number.isNaN(date.getTime())) return this.meetingDate;
    return date.toLocaleDateString('id-ID', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' });
  }

  /** Bagian hari & tanggal untuk kartu ringkas. */
  get dayNumber(): string {
    return this.meetingDate.slice(8, 10).replace(/^0/, '');
  }

  get monthShort(): string {
    const date = new Date(`${this.meetingDate}T12:00:00`);
    if (Number.isNaN(date.getTime())) return '—';
    return date.toLocaleDateString('id-ID', { month: 'short' });
  }

  get statusLabel(): string {
    switch (this.status) {
      case 'TERLAKSANA':
        return 'Terlaksana';
      case 'LIBUR':
        return 'Libur';
      default:
        return 'Terjadwal';
    }
  }

  get tone(): 'pending' | 'done' | 'off' {
    if (this.status === 'TERLAKSANA') return 'done';
    if (this.status === 'LIBUR') return 'off';
    return 'pending';
  }

  get isScheduled(): boolean {
    return this.status === 'TERJADWAL';
  }

  get isDone(): boolean {
    return this.status === 'TERLAKSANA';
  }

  /** Tagihan yang sudah ada pembayarannya (tetap disimpan saat kumpulan batal). */
  get billsPaidCount(): number {
    return Math.max(this.billsCount - this.billsUnpaidCount, 0);
  }

  get isHoliday(): boolean {
    return this.status === 'LIBUR';
  }

  /** Jadwal hasil generate otomatis (35 hari). */
  get isAutoGenerated(): boolean {
    return this.note === AUTO_SCHEDULE_NOTE;
  }

  /** Hanya jadwal yang belum terlaksana boleh dihapus. */
  get canDelete(): boolean {
    return this.isScheduled;
  }

  /** Tagihan hanya boleh dibuat untuk kumpulan yang sudah terlaksana. */
  get canGenerateBills(): boolean {
    return this.isDone;
  }

  /** Undangan hanya untuk kumpulan yang masih terjadwal. */
  get canInvite(): boolean {
    return this.isScheduled;
  }

  matches(keyword: string): boolean {
    const needle = keyword.trim().toLowerCase();
    if (!needle) return true;
    return [this.meetingDate, this.status, this.statusLabel, this.note, this.clock, this.meetingDateLabel]
      .some((value) => value.toLowerCase().includes(needle));
  }

  withStatus(status: PeriodStatus): Period {
    return new Period(this.id, this.startDate, this.meetingDate, this.meetingTime, status, this.note, this.finance);
  }

  toPayload(): PeriodInput {
    return {
      start_date: this.startDate,
      meeting_date: this.meetingDate,
      meeting_time: this.clock,
      status: this.status as PeriodStatus,
      note: this.note,
    };
  }
}

export default Period;
