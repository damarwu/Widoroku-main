import { apiFetch } from '../api';
import { PamCustomer, type PamCustomerJSON } from './PamCustomer';
import { PamPayment, type PamPaymentJSON } from './PamPayment';

export class PamRepository {
  async get<T>(url:string):Promise<T>{
    const response=await apiFetch(url);const value=await response.json();
    if(!response.ok)throw new Error(value.error||'Gagal memuat data PAM');
    return value as T;
  }
  async request<T>(url:string,method:string,body?:unknown):Promise<T>{
    const response=await apiFetch(url,{method,headers:{'Content-Type':'application/json'},...(body===undefined?{}:{body:JSON.stringify(body)})});
    const value=await response.json();if(!response.ok)throw new Error(value.error||'Permintaan PAM gagal');return value as T;
  }
  async loadCustomers():Promise<PamCustomer[]>{return (await this.get<PamCustomerJSON[]>('/api/pam/customers')).map(PamCustomer.fromJSON)}
  async loadPayments():Promise<PamPayment[]>{return (await this.get<PamPaymentJSON[]>('/api/pam/payments')).map(PamPayment.fromJSON)}
  async updateCustomer(id:number,meterNumber:string,active:boolean):Promise<void>{await this.request(`/api/pam/customers/${id}`,'PUT',{meter_number:meterNumber,active})}
  async deactivateCustomer(id:number):Promise<void>{await this.request(`/api/pam/customers/${id}`,'DELETE')}
  async updatePayment(id:number,amount:number,note:string,methodCode:string):Promise<void>{await this.request(`/api/pam/payments/${id}`,'PUT',{amount,note,method_code:methodCode})}
  async deletePayment(id:number):Promise<void>{await this.request(`/api/pam/payments/${id}`,'DELETE')}
}
