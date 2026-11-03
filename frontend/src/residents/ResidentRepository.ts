import { apiFetch } from '../api';
import Resident, { type ResidentInput, type ResidentJSON } from './Resident';

/** Kesalahan API dengan pesan yang siap ditampilkan ke pengguna. */
export class ResidentError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ResidentError';
  }
}

/**
 * Lapisan akses data warga.
 *
 * Seluruh komunikasi HTTP ke endpoint /api/residents dibungkus di sini agar
 * komponen tampilan hanya berurusan dengan objek Resident.
 */
export class ResidentRepository {
  private async request(url: string, init?: RequestInit): Promise<Response> {
    let response: Response;
    try {
      response = await apiFetch(url, init);
    } catch {
      throw new ResidentError('Tidak dapat menghubungi server. Periksa koneksi dan alamat server.');
    }
    if (!response.ok) {
      const payload = await response.json().catch(() => ({} as { error?: string }));
      throw new ResidentError(payload.error || `Permintaan gagal (${response.status}).`);
    }
    return response;
  }

  async list(): Promise<Resident[]> {
    const response = await this.request('/api/residents');
    const data = (await response.json()) as ResidentJSON[];
    return Resident.fromList(data);
  }

  async create(input: ResidentInput): Promise<Resident> {
    const response = await this.request('/api/residents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    const data = (await response.json()) as ResidentJSON;
    return Resident.fromJSON(data);
  }

  async update(id: number, input: ResidentInput): Promise<Resident> {
    const response = await this.request(`/api/residents/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
    const data = (await response.json()) as ResidentJSON;
    return Resident.fromJSON({ ...data, id });
  }

  async updateContact(id: number, phone: string, savingsClass: number): Promise<void> {
    await this.request(`/api/residents/${id}/contact`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ phone, savings_class: savingsClass }),
    });
  }
}

export default ResidentRepository;
