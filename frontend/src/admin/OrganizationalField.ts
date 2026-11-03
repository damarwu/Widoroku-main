export interface OrganizationalFieldJSON { id:number; name:string; description:string }
export interface OrganizationalFieldInput { name:string; description:string }

/** Model bidang organisasi RT. */
export class OrganizationalField {
  constructor(public readonly id:number, public readonly name:string, public readonly description:string) {}
  static fromJSON(data:OrganizationalFieldJSON):OrganizationalField { return new OrganizationalField(Number(data.id),data.name||'',data.description||''); }
  static fromList(data:OrganizationalFieldJSON[]):OrganizationalField[] { return data.map(OrganizationalField.fromJSON); }
  toInput():OrganizationalFieldInput { return {name:this.name.trim(),description:this.description.trim()}; }
}
