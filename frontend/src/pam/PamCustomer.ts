export type PamCustomerJSON = { id:number; resident_id:number; house_code:string; head_name:string; meter_number:string; active:boolean; last_reading:number; last_month:string; last_due:number };

export class PamCustomer {
  constructor(public id:number, public residentId:number, public houseCode:string, public headName:string, public meterNumber:string, public active:boolean, public lastReading:number, public lastMonth:string, public lastDue:number) {}
  get resident_id(){return this.residentId} get house_code(){return this.houseCode} get head_name(){return this.headName} get meter_number(){return this.meterNumber} get last_reading(){return this.lastReading} get last_month(){return this.lastMonth} get last_due(){return this.lastDue}
  static fromJSON(value:PamCustomerJSON):PamCustomer { return new PamCustomer(value.id,value.resident_id,value.house_code,value.head_name,value.meter_number,value.active,value.last_reading,value.last_month,value.last_due); }
}
