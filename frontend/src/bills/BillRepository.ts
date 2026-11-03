import { apiFetch } from '../api';
import { Bill, type BillJSON } from './Bill';

/** Kesalahan API tagihan dengan pesan siap tampil. */
export class BillError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'BillError';
  }
}

export interface PaymentInput {
  residentId: number;
  amount: number;
  note?: string;
  /** Kosong = bayar semua tagihan belum lunas milik warga. */
  billIds?: number[];
  voluntary?: boolean;
}

export interface PaymentResult {
  payment_id: number;
  received: number;
  allocated: number;
  bills_paid: number;
  message: string;
}

export interface PaymentRecord {
  id: number; resident_id: number; house_code: string; head_name: string;
  amount: number; note: string; received_at: string; allocated: number;
}
export interface FinanceAllocation {
  key: string; label: string; field: string; amount: number; confirmed: boolean;
  confirmed_by: string; confirmed_at: string; note: string; can_confirm: boolean;
}
export interface FinanceReport {
  finance: { period_id: number; payment_count: number; total_collected: number; host_consumption: number; sosial: number; penerangan_jalan: number; ziswaf: number; kemasjidan: number; kas_rt: number };
  status: string; prepared: boolean; current_total: number; current_count: number; required_amount: number; shortfall: number; allocations: FinanceAllocation[];
}

/**
 * Lapisan akses data tagihan & pembayaran.
 */
export class BillRepository {
  private async request(url: string, init?: RequestInit): Promise<Response> {
    let response: Response;
    try {
      response = await apiFetch(url, init);
    } catch {
      throw new BillError('Tidak dapat menghubungi server. Periksa koneksi dan alamat server.');
    }
    if (!response.ok) {
      const payload = await response.json().catch(() => ({} as { error?: string }));
      throw new BillError(payload.error || `Permintaan gagal (${response.status}).`);
    }
    return response;
  }

  async list(residentId?: number): Promise<Bill[]> {
    const suffix = residentId ? `?resident_id=${residentId}` : '';
    const response = await this.request(`/api/bills${suffix}`);
    return Bill.fromList((await response.json()) as BillJSON[]);
  }

  async pay(input: PaymentInput): Promise<PaymentResult> {
    const response = await this.request('/api/payments', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        resident_id: input.residentId,
        amount: input.amount,
        note: input.note ?? '',
        bill_ids: input.billIds ?? [],
        voluntary: input.voluntary ?? false,
      }),
    });
    return (await response.json()) as PaymentResult;
  }

  async listPayments(): Promise<PaymentRecord[]> {
    const response = await this.request('/api/payments');
    return (await response.json()) as PaymentRecord[];
  }

  async updatePayment(id: number, amount: number): Promise<void> {
    await this.request(`/api/payments/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ amount }) });
  }

  async deletePayment(id: number): Promise<void> {
    await this.request(`/api/payments/${id}`, { method: 'DELETE' });
  }

  async finance(id: number): Promise<FinanceReport> {
    const response = await this.request(`/api/periods/${id}/finance`);
    return (await response.json()) as FinanceReport;
  }

  async prepareFinance(id: number): Promise<void> {
    await this.request(`/api/periods/${id}/finance`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
  }

  async confirmFinanceReceipt(id: number, key: string): Promise<void> {
    await this.request(`/api/periods/${id}/finance/confirm`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key }) });
  }

  async financeReceiptDates(): Promise<string[]> {
    const response = await this.request('/api/finance/dates');
    return (await response.json()) as string[];
  }

  async dailyFinance(date: string): Promise<FinanceReport> {
    const response = await this.request(`/api/finance/daily/${date}`);
    return (await response.json()) as FinanceReport;
  }

  async prepareDailyFinance(date: string): Promise<void> {
    await this.request(`/api/finance/daily/${date}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' });
  }

  async confirmDailyFinance(date: string, key: string): Promise<void> {
    await this.request(`/api/finance/daily/${date}/confirm`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ key }) });
  }

  /** Membuat tagihan yang belum ada pada semua kumpulan TERLAKSANA. */
  async syncMeetingBills(): Promise<{ periods: number; created: number; amount: number }> {
    const response = await this.request('/api/bills/sync', { method: 'POST' });
    return (await response.json()) as { periods: number; created: number; amount: number };
  }
}

export default BillRepository;
