export interface OrganizationalPositionJSON {
  id:number; field_id:number|null; field:string; name:string; description:string; permissions:Record<string,boolean>;
}
export interface OrganizationalPositionInput { field_id:number; name:string; description:string; permissions:Record<string,boolean> }

/** Model jabatan, bidang penugasan, dan hak akses menu. */
export class OrganizationalPosition {
  constructor(public readonly id:number,public readonly fieldId:number|null,public readonly field:string,public readonly name:string,public readonly description:string,public readonly permissions:Record<string,boolean>) {}
  static fromJSON(data:OrganizationalPositionJSON):OrganizationalPosition { return new OrganizationalPosition(Number(data.id),data.field_id?Number(data.field_id):null,data.field||'',data.name||'',data.description||'',data.permissions||{}); }
  static fromList(data:OrganizationalPositionJSON[]):OrganizationalPosition[] { return data.map(OrganizationalPosition.fromJSON); }
  get fieldLabel():string { return this.field||'Tanpa bidang'; }
  get allowedMenuCount():number { return Object.entries(this.permissions).filter(([key,value])=>key.endsWith('.view')&&value).length; }
  get managedMenuCount():number { return Object.entries(this.permissions).filter(([key,value])=>key.endsWith('.manage')&&value).length; }
  toInput():OrganizationalPositionInput { return {field_id:this.fieldId||0,name:this.name.trim(),description:this.description.trim(),permissions:{...this.permissions}}; }
}
