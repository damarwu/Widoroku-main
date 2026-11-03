import { apiFetch } from '../api';
import { SavingsStage } from './SavingsStage';
import { SavingsLedgerEntry } from './SavingsLedgerEntry';
import { SavingsTransaction } from './SavingsTransaction';
import { SavingsSummary } from './SavingsSummary';

export type SavingsMeeting = { id: number; meeting_date: string; status: string; stage_number: number };
export type SavingsResident = { id: number; house_code: string; head_name: string; savings_class: number };
export type SavingsPage = { items: SavingsLedgerEntry[]; page: number; page_size: number; total: number };
export type SavingsTransactionPage = { items: SavingsTransaction[]; page: number; page_size: number; total: number };

export class SavingsRepository {
  async loadSummary(): Promise<SavingsSummary> {
    const response = await apiFetch('/api/tabungan/summary');
    if (!response.ok) throw new Error('Gagal memuat ringkasan Tabungan Jalan');
    return SavingsSummary.fromJSON(await response.json());
  }
  async loadStages(): Promise<{ stages: SavingsStage[]; meetings: SavingsMeeting[]; residents: SavingsResident[] }> {
    const response = await apiFetch('/api/tabungan/stages');
    if (!response.ok) throw new Error('Gagal memuat pengaturan tahap tabungan');
    const value = await response.json() as { stages: { stage_number: number; meeting_count: number; billed_residents?: number }[]; meetings: SavingsMeeting[]; residents: SavingsResident[] };
    return { stages: value.stages.map(SavingsStage.fromJSON), meetings: value.meetings, residents: value.residents };
  }
  async loadLedger(stage: number, classFilter: number, page: number, pageSize: number): Promise<SavingsPage> {
    const response = await apiFetch(`/api/tabungan?stage=${stage}&class=${classFilter}&page=${page}&page_size=${pageSize}`);
    if (!response.ok) throw new Error('Gagal memuat buku tabungan');
    const value = await response.json() as { items: Record<string, number | string>[]; page: number; page_size: number; total: number };
    return { ...value, items: value.items.map(SavingsLedgerEntry.fromJSON) };
  }
  async loadTransactions(stage: number, page: number, pageSize: number): Promise<SavingsTransactionPage> {
    const response = await apiFetch(`/api/tabungan/transactions?stage=${stage}&page=${page}&page_size=${pageSize}`);
    if (!response.ok) throw new Error('Gagal memuat riwayat setoran');
    const value = await response.json() as { items: Record<string, number | string>[]; page: number; page_size: number; total: number };
    return { ...value, items: value.items.map(SavingsTransaction.fromJSON) };
  }
  async createStage(meetingCount: number): Promise<void> {
    const response = await apiFetch('/api/tabungan/stages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ meeting_count: meetingCount }) });
    const value = await response.json() as { error?: string };
    if (!response.ok) throw new Error(value.error || 'Gagal menambah tahap');
  }
  async updateStage(stage: SavingsStage): Promise<void> {
    const response = await apiFetch(`/api/tabungan/stages/${stage.stageNumber}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ meeting_count: stage.meetingCount }) });
    const value = await response.json() as { error?: string };
    if (!response.ok) throw new Error(value.error || 'Gagal mengubah tahap');
  }
  async deleteStage(stage: number): Promise<void> {
    const response = await apiFetch(`/api/tabungan/stages/${stage}`, { method: 'DELETE' });
    const value = await response.json() as { error?: string };
    if (!response.ok) throw new Error(value.error || 'Gagal menghapus tahap');
  }
  async deposit(residentId: number, periodId: number, amount: number): Promise<void> {
    const response = await apiFetch('/api/tabungan', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ resident_id: residentId, period_id: periodId, amount, type: 'SETOR' }) });
    const value = await response.json() as { error?: string };
    if (!response.ok) throw new Error(value.error || 'Gagal mencatat setoran');
  }
  async updateTransaction(id: number, amount: number, note: string): Promise<void> {
    const response = await apiFetch(`/api/tabungan/transactions/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ amount, note }) });
    const value = await response.json() as { error?: string };
    if (!response.ok) throw new Error(value.error || 'Gagal mengubah setoran');
  }
  async deleteTransaction(id: number): Promise<void> {
    const response = await apiFetch(`/api/tabungan/transactions/${id}`, { method: 'DELETE' });
    const value = await response.json() as { error?: string };
    if (!response.ok) throw new Error(value.error || 'Gagal menghapus setoran');
  }
}
