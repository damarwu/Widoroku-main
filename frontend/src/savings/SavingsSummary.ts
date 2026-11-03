export class SavingsStageSummary {
  constructor(public stageNumber: number, public meetingCount: number, public target: number, public collected: number, public remaining: number, public unpaidResidents: number) {}
  static fromJSON(value: Record<string, number>): SavingsStageSummary {
    return new SavingsStageSummary(value.stage_number, value.meeting_count, value.target, value.collected, value.remaining, value.unpaid_residents);
  }
}

export class SavingsRealization {
  constructor(public id: number, public date: string, public amount: number, public note: string) {}
  static fromJSON(value: Record<string, number | string>): SavingsRealization {
    return new SavingsRealization(Number(value.id), String(value.date), Number(value.amount), String(value.note || ''));
  }
}

export class SavingsSummary {
  constructor(
    public residentCount: number, public targetPerStage: number, public allStagesTarget: number,
    public stageCount: number, public totalCollected: number, public totalSpent: number,
    public endingBalance: number, public stages: SavingsStageSummary[], public realizations: SavingsRealization[],
  ) {}
  static fromJSON(value: {
    resident_count: number; target_per_stage: number; all_stages_target: number; stage_count: number;
    total_collected: number; total_spent: number; ending_balance: number;
    stages: Record<string, number>[]; realizations: Record<string, number | string>[];
  }): SavingsSummary {
    return new SavingsSummary(value.resident_count, value.target_per_stage, value.all_stages_target, value.stage_count, value.total_collected, value.total_spent, value.ending_balance, value.stages.map(SavingsStageSummary.fromJSON), value.realizations.map(SavingsRealization.fromJSON));
  }
}
