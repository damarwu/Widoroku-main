export type PamPaymentJSON = { id:number; bill_id:number; house_code:string; head_name:string; billing_month:string; amount:number; method_code:string; note:string; received_at:string; staff_name:string; protected:boolean };

export class PamPayment {
  constructor(public id:number, public billId:number, public houseCode:string, public headName:string, public billingMonth:string, public amount:number, public methodCode:string, public note:string, public receivedAt:string, public staffName:string, public protectedPayment:boolean) {}
  get bill_id(){return this.billId} get house_code(){return this.houseCode} get head_name(){return this.headName} get billing_month(){return this.billingMonth} get method_code(){return this.methodCode} get received_at(){return this.receivedAt} get staff_name(){return this.staffName}
  get isProtected(){return this.protectedPayment}
  static fromJSON(value:PamPaymentJSON):PamPayment { return new PamPayment(value.id,value.bill_id,value.house_code,value.head_name,value.billing_month,value.amount,value.method_code,value.note,value.received_at,value.staff_name,value.protected); }
}
