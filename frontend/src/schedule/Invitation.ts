import Period, { formatMoney, type PeriodJSON } from './Period';

export interface RecipientJSON {
  id: number;
  house_code: string;
  head_name: string;
  phone: string;
  savings_class: number;
  cash_due: number;
  savings_due: number;
}

export interface InvitationJSON {
  period: PeriodJSON;
  reminder_date: string;
  savings_stage: number;
  recipients: RecipientJSON[];
}

/** Satu warga penerima undangan beserta kewajibannya. */
export class Recipient {
  readonly id: number;
  readonly houseCode: string;
  readonly headName: string;
  readonly phone: string;
  readonly savingsClass: number;
  readonly cashDue: number;
  readonly savingsDue: number;

  constructor(data: RecipientJSON) {
    this.id = Number(data.id);
    this.houseCode = data.house_code ?? '';
    this.headName = data.head_name ?? '';
    this.phone = data.phone ?? '';
    this.savingsClass = Number(data.savings_class ?? 3);
    this.cashDue = Number(data.cash_due ?? 0);
    this.savingsDue = Number(data.savings_due ?? 0);
  }

  static fromList(data: RecipientJSON[]): Recipient[] {
    return data.map((item) => new Recipient(item));
  }

  get hasPhone(): boolean {
    return this.whatsappNumber !== null;
  }

  /** Nomor ternormalisasi (62…), null bila kosong atau tidak valid. */
  get whatsappNumber(): string | null {
    const digits = this.phone.replace(/\D/g, '');
    // "0" atau nomor terlalu pendek adalah placeholder, bukan nomor nyata.
    if (digits.length < 8) return null;
    return digits.startsWith('0') ? `62${digits.slice(1)}` : digits;
  }

  get cashDueLabel(): string {
    return formatMoney(this.cashDue);
  }

  get savingsDueLabel(): string {
    return formatMoney(this.savingsDue);
  }
}

/**
 * Undangan kumpulan: menggabungkan periode, tanggal pengingat H-5, dan daftar
 * penerima. Semua logika pesan WhatsApp dan baris cetak PDF ada di sini agar
 * komponen tampilan tetap ringkas.
 */
export class Invitation {
  readonly period: Period;
  readonly reminderDate: string;
  readonly savingsStage: number;
  readonly recipients: Recipient[];

  constructor(period: Period, reminderDate: string, savingsStage: number, recipients: Recipient[]) {
    this.period = period;
    this.reminderDate = reminderDate;
    this.savingsStage = savingsStage;
    this.recipients = recipients;
  }

  static fromJSON(data: InvitationJSON): Invitation {
    return new Invitation(
      Period.fromJSON(data.period),
      data.reminder_date ?? '',
      Number(data.savings_stage ?? 0),
      Recipient.fromList(data.recipients ?? []),
    );
  }

  get totalCashDue(): number {
    return this.recipients.reduce((total, recipient) => total + recipient.cashDue, 0);
  }

  get totalSavingsDue(): number {
    return this.recipients.reduce((total, recipient) => total + recipient.savingsDue, 0);
  }

  get withPhone(): Recipient[] {
    return this.recipients.filter((recipient) => recipient.hasPhone);
  }

  get withoutPhone(): Recipient[] {
    return this.recipients.filter((recipient) => !recipient.hasPhone);
  }

  /** Undangan boleh dikirim mulai H-5 sebelum kumpulan. */
  isSendWindowOpen(today = new Date()): boolean {
    const iso = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`;
    return this.reminderDate !== '' && iso >= this.reminderDate;
  }

  /** Teks pesan WhatsApp untuk satu penerima. */
  messageFor(recipient: Recipient): string {
    const savingsLine = this.savingsStage
      ? `Tabungan jalan tahap ${this.savingsStage} (kelas ${recipient.savingsClass}): ${recipient.savingsDueLabel}`
      : 'Tabungan jalan: belum terhubung ke tahap.';
    return [
      `Assalamu'alaikum ${recipient.headName}, undangan Kumpulan Ahad Pon ${this.period.meetingDate} pukul ${this.period.clock} WIB.`,
      '',
      `Tunggakan iuran kas: ${recipient.cashDueLabel}`,
      savingsLine,
      '',
      'Mohon hadir. Matur nuwun.',
    ].join('\n');
  }

  /** Tautan wa.me untuk satu penerima, null bila nomor kosong. */
  whatsappLinkFor(recipient: Recipient): string | null {
    const number = recipient.whatsappNumber;
    if (!number) return null;
    return `https://wa.me/${number}?text=${encodeURIComponent(this.messageFor(recipient))}`;
  }

  /** Baris data untuk dokumen cetak/PDF. */
  get printRows(): { no: number; houseCode: string; headName: string; cashDue: string; savingsDue: string }[] {
    return this.recipients.map((recipient, index) => ({
      no: index + 1,
      houseCode: recipient.houseCode,
      headName: recipient.headName,
      cashDue: recipient.cashDueLabel,
      savingsDue: this.savingsStage ? recipient.savingsDueLabel : '—',
    }));
  }
}

export default Invitation;
