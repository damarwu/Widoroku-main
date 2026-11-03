export class SavingsTransaction {
  constructor(public id: number, public houseCode: string, public name: string, public stage: number, public amount: number, public note: string, public meetingDate: string) {}
  static fromJSON(value: Record<string, number | string>): SavingsTransaction {
    return new SavingsTransaction(Number(value.id), String(value.house_code), String(value.name), Number(value.stage), Number(value.amount), String(value.note || ''), String(value.meeting_date || ''));
  }
}
