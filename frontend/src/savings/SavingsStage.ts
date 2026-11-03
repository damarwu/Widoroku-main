export class SavingsStage {
  constructor(public stageNumber: number, public meetingCount: number, public billedResidents = 0) {}
  get label(): string { return `Tahap ${this.stageNumber}`; }
  static fromJSON(value: { stage_number: number; meeting_count: number; billed_residents?: number }): SavingsStage {
    return new SavingsStage(value.stage_number, value.meeting_count, value.billed_residents ?? 0);
  }
}
