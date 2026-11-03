import type { PamCustomer } from './PamCustomer';
import type { PamPayment } from './PamPayment';

export class PamSummary {
  constructor(public activeCustomers:number, public billCount:number, public billedAmount:number, public collected:number, public outstanding:number, public pendingReadings:number, public pendingPayments:number, public commissions:number) {}
  static fromData(data:{customers:PamCustomer[];bills:{total:number;paid:number;remaining:number}[];pendingReadings:unknown[];paymentSubmissions:{status:string}[];commissions:{amount:number}[]}):PamSummary {
    const bills=data.bills;
    return new PamSummary(data.customers.filter(customer=>customer.active).length,bills.length,bills.reduce((sum,bill)=>sum+bill.total,0),bills.reduce((sum,bill)=>sum+bill.paid,0),bills.reduce((sum,bill)=>sum+bill.remaining,0),data.pendingReadings.length,data.paymentSubmissions.filter(item=>item.status==='PENDING').length,data.commissions.reduce((sum,item)=>sum+item.amount,0));
  }
}
