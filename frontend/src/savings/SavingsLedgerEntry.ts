export class SavingsLedgerEntry {
  constructor(
    public residentId: number, public houseCode: string, public name: string,
    public savingsClass: number, public stage: number, public tariff: number,
    public deposited: number, public remaining: number,
  ) {}
  static fromJSON(value: Record<string, number | string>): SavingsLedgerEntry {
    return new SavingsLedgerEntry(Number(value.resident_id), String(value.house_code), String(value.name), Number(value.savings_class), Number(value.stage), Number(value.tariff), Number(value.deposited), Number(value.remaining));
  }
}
