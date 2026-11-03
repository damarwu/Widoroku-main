import { apiFetch } from '../api';
import { OrganizationalPosition, type OrganizationalPositionInput, type OrganizationalPositionJSON } from './OrganizationalPosition';

export class OrganizationalPositionRepository {
  private async request<T>(url:string,init?:RequestInit):Promise<T> {
    const response=await apiFetch(url,init);const payload=await response.json().catch(()=>({}));
    if(!response.ok)throw new Error(payload.error||'Permintaan jabatan gagal');
    return payload as T;
  }
  async list():Promise<OrganizationalPosition[]> {
    const result=await this.request<{items:OrganizationalPositionJSON[]}>('/api/admin/positions');
    return OrganizationalPosition.fromList(result.items||[]);
  }
  async save(input:OrganizationalPositionInput,id?:number):Promise<void> {
    await this.request(id?`/api/admin/positions/${id}`:'/api/admin/positions',{method:id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(input)});
  }
  async delete(id:number):Promise<void> { await this.request(`/api/admin/positions/${id}`,{method:'DELETE'}); }
}
