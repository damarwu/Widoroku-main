import { apiFetch } from '../api';
import Period, { type PeriodInput, type PeriodJSON, type PeriodStatus } from './Period';
import Invitation, { type InvitationJSON } from './Invitation';

/** Kesalahan API jadwal kumpulan dengan pesan siap tampil. */
export class PeriodError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'PeriodError';
  }
}

export interface GenerateSeriesInput {
  start_date: string;
  through_date: string;
  meeting_time: string;
}

export interface GenerateSeriesResult {
  created: number;
  skipped: number;
  first_date: string;
  last_date: string;
  meeting_time: string;
}

/**
 * Lapisan akses data jadwal kumpulan.
 * Seluruh endpoint /api/periods dibungkus di sini.
 */
export class PeriodRepository {
  private async request(url: string, init?: RequestInit): Promise<Response> {
    let response: Response;
    try {
      response = await apiFetch(url, init);
    } catch {
      throw new PeriodError('Tidak dapat menghubungi server. Periksa koneksi dan alamat server.');
    }
    if (!response.ok) {
      const payload = await response.json().catch(() => ({} as { error?: string }));
      throw new PeriodError(payload.error || `Permintaan gagal (${response.status}).`);
    }
    return response;
  }

  async list(): Promise<Period[]> {
    const response = await this.request('/api/periods');
    return Period.fromList((await response.json()) as PeriodJSON[]);
  }

  async create(input: PeriodInput): Promise<Period> {
    const response = await this.request('/api/periods', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    return Period.fromJSON((await response.json()) as PeriodJSON);
  }

  async update(id: number, input: PeriodInput): Promise<Period> {
    const response = await this.request(`/api/periods/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    return Period.fromJSON({ ...((await response.json()) as PeriodJSON), id });
  }

  async changeStatus(id: number, period: Period, status: PeriodStatus): Promise<void> {
    await this.update(id, { ...period.toPayload(), status });
  }

  async remove(id: number): Promise<void> {
    await this.request(`/api/periods/${id}`, { method: 'DELETE' });
  }

  /** Membuat jadwal berulang tiap 35 hari sampai tanggal akhir yang dipilih. */
  async generateSeries(input: GenerateSeriesInput): Promise<GenerateSeriesResult> {
    const response = await this.request('/api/periods/generate-series', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    return (await response.json()) as GenerateSeriesResult;
  }

  async generateBills(id: number): Promise<void> {
    await this.request(`/api/periods/${id}/generate-bills`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    });
  }

  async invitation(id: number): Promise<Invitation> {
    const response = await this.request(`/api/periods/${id}/invitation`);
    return Invitation.fromJSON((await response.json()) as InvitationJSON);
  }

  async finance(id: number): Promise<{ finance: unknown; status: string; shortfall: number }> {
    const response = await this.request(`/api/periods/${id}/finance`);
    return (await response.json()) as { finance: unknown; status: string; shortfall: number };
  }

  async saveFinance(id: number): Promise<void> {
    await this.request(`/api/periods/${id}/finance`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
    });
  }
}

export default PeriodRepository;
