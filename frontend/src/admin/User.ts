export interface UserJSON {
  id: number;
  username: string;
  full_name: string;
  position_id: number;
  position: string;
  resident_id: number;
  active: boolean;
}

export interface UserInput {
  username: string;
  full_name: string;
  password: string;
  position_id: number;
  resident_id: number;
  active: boolean;
}

/** Model akun pengurus/warga dan helper payload API. */
export class AdminUser {
  constructor(
    public readonly id: number,
    public readonly username: string,
    public readonly fullName: string,
    public readonly positionId: number,
    public readonly position: string,
    public readonly residentId: number,
    public readonly active: boolean,
  ) {}

  static fromJSON(data: UserJSON): AdminUser {
    return new AdminUser(Number(data.id), data.username, data.full_name, Number(data.position_id), data.position, Number(data.resident_id || 0), data.active);
  }

  static fromList(data: UserJSON[]): AdminUser[] {
    return data.map(AdminUser.fromJSON);
  }

  get isResidentAccount(): boolean {
    return this.position === 'Warga';
  }

  get residentLabel(): string {
    return this.residentId ? `KK #${this.residentId}` : 'Tidak ditautkan ke KK';
  }

  toInput(input: Omit<UserInput, 'active'>): UserInput {
    return { ...input, active: true };
  }
}
