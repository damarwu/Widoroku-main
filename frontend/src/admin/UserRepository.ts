import { apiFetch } from '../api';
import { AdminUser, type UserInput, type UserJSON } from './User';

/** Akses API khusus akun pengguna. */
export class UserRepository {
  private async request<T>(url: string, init?: RequestInit): Promise<T> {
    const response = await apiFetch(url, init);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || 'Permintaan akun pengguna gagal');
    return payload as T;
  }

  async list(): Promise<AdminUser[]> {
    return AdminUser.fromList(await this.request<UserJSON[]>('/api/admin/users'));
  }

  async save(input: UserInput, id?: number): Promise<void> {
    await this.request(id ? `/api/admin/users/${id}` : '/api/admin/users', {
      method: id ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    });
  }

  async delete(id: number): Promise<void> {
    await this.request(`/api/admin/users/${id}`, { method: 'DELETE' });
  }
}
