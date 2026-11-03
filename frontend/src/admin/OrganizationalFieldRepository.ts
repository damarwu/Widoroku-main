import { apiFetch } from '../api';
import { OrganizationalField, type OrganizationalFieldInput, type OrganizationalFieldJSON } from './OrganizationalField';

export class OrganizationalFieldRepository {
  private async request<T>(url:string,init?:RequestInit):Promise<T> {
    const response=await apiFetch(url,init);const payload=await response.json().catch(()=>({}));
    if(!response.ok)throw new Error(payload.error||'Permintaan bidang gagal');
    return payload as T;
  }
  async list():Promise<OrganizationalField[]> { return OrganizationalField.fromList(await this.request<OrganizationalFieldJSON[]>('/api/admin/fields')); }
  async save(input:OrganizationalFieldInput,id?:number):Promise<void> {
    await this.request(id?`/api/admin/fields/${id}`:'/api/admin/fields',{method:id?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(input)});
  }
  async delete(id:number):Promise<void> { await this.request(`/api/admin/fields/${id}`,{method:'DELETE'}); }
}
