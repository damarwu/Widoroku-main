import { useEffect, useState, type FormEvent } from 'react';
import { ArrowDownLeft, ArrowUpRight, RefreshCw, Wallet } from 'lucide-react';
import { apiFetch } from './api';
import './cashbook.css';

type CashbookField = { key:string; label:string; income:number; pending:number; expense:number; balance:number; can_record:boolean };
type CashbookEntry = { id:number|null; date:string; field_key:string; field_label:string; kind:'MASUK'|'KELUAR'; amount:number; note:string; actor:string; created_at:string };
type CashbookData = { fields:CashbookField[]; entries:CashbookEntry[] };
const money=(value:number)=>new Intl.NumberFormat('id-ID',{style:'currency',currency:'IDR',maximumFractionDigits:0}).format(value);
const todayLocal=()=>{const date=new Date();return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`};

export default function CashbookPage({onToast}:{onToast:(message:string)=>void}){
  const [data,setData]=useState<CashbookData>({fields:[],entries:[]});
  const [fieldKey,setFieldKey]=useState('consumption');
  const [direction,setDirection]=useState<'MASUK'|'KELUAR'>('MASUK');
  const [date,setDate]=useState(todayLocal());
  const [amount,setAmount]=useState('');
  const [note,setNote]=useState('');
  const [loading,setLoading]=useState(true);
  const [saving,setSaving]=useState(false);
  const [error,setError]=useState('');
  const load=async()=>{
    setLoading(true);
    try{const response=await apiFetch('/api/cashbook');const body=await response.json();if(!response.ok)throw new Error(body.error||'Buku kas gagal dimuat.');const next=body as CashbookData;setData(next);setFieldKey((current)=>next.fields.some((field)=>field.key===current&&field.can_record)?current:next.fields.find((field)=>field.can_record)?.key||'');}
    catch(cause){setError(cause instanceof Error?cause.message:'Buku kas gagal dimuat.');}
    finally{setLoading(false)}
  };
  useEffect(()=>{void load()},[]);
  const submit=async(event:FormEvent)=>{
    event.preventDefault();setSaving(true);setError('');
    try{const response=await apiFetch('/api/cashbook/transactions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({field_key:fieldKey,direction,date,amount:Number(amount),note})});const body=await response.json();if(!response.ok)throw new Error(body.error||'Arus kas gagal dicatat.');setAmount('');setNote('');await load();onToast(direction==='MASUK'?'Kas masuk bidang dicatat.':'Pengeluaran bidang dicatat.');}
    catch(cause){setError(cause instanceof Error?cause.message:'Arus kas gagal dicatat.')}
    finally{setSaving(false)}
  };
  const selected=data.fields.find((field)=>field.key===fieldKey);
  return <div className="cashbook-page">
    <section className="panel">
      <div className="panel-heading"><div><h3>Arus kas per bidang</h3><p>Penerimaan dihitung setelah pengurus bidang mengonfirmasi alokasi. Saldo = penerimaan terkonfirmasi − pengeluaran tercatat.</p></div><button className="ghost-btn" type="button" onClick={()=>void load()} disabled={loading}><RefreshCw size={15}/> Segarkan</button></div>
      {error&&<div className="notice cashbook-error" role="alert">{error}</div>}
      {loading?<p>Memuat arus kas...</p>:<div className="cashbook-grid">{data.fields.map((field)=><article className="cashbook-card" key={field.key}><div className="cashbook-card-title"><span><Wallet size={16}/></span><b>{field.label}</b></div><dl><div><dt>Penerimaan</dt><dd>{money(field.income)}</dd></div><div><dt>Menunggu konfirmasi</dt><dd>{money(field.pending)}</dd></div><div><dt>Pengeluaran</dt><dd>{money(field.expense)}</dd></div></dl><div className="cashbook-balance"><span>Saldo</span><b>{money(field.balance)}</b></div></article>)}</div>}
    </section>

    {data.fields.some((field)=>field.can_record)&&<section className="panel">
      <div className="panel-heading"><div><h3>Catat arus kas bidang</h3><p>Pengurus dapat mencatat pemasukan dan pengeluaran untuk bidang yang terhubung ke jabatannya.</p></div><ArrowUpRight size={19}/></div>
      <form className="cashbook-form" onSubmit={submit}>
        <label>Bidang<select value={fieldKey} onChange={(event)=>setFieldKey(event.target.value)}>{data.fields.filter((field)=>field.can_record).map((field)=><option key={field.key} value={field.key}>{field.label}</option>)}</select></label>
        <label>Arus<select value={direction} onChange={(event)=>setDirection(event.target.value as 'MASUK'|'KELUAR')}><option value="MASUK">Kas masuk</option><option value="KELUAR">Kas keluar</option></select></label>
        <label>Tanggal<input type="date" value={date} onChange={(event)=>setDate(event.target.value)} required/></label>
        <label>Nominal (Rp)<input type="number" min="1" max={direction==='KELUAR'?(selected?.balance||undefined):undefined} value={amount} onChange={(event)=>setAmount(event.target.value)} required/></label>
        <label>Keterangan<input value={note} onChange={(event)=>setNote(event.target.value)} maxLength={500} placeholder="Contoh: pembelian lampu jalan" required/></label>
        <button className="primary-btn" disabled={saving||!data.fields.some((field)=>field.can_record)}>{saving?'Menyimpan...':'Simpan arus kas'}</button>
      </form>
    </section>}

    <section className="panel">
      <div className="panel-heading"><div><h3>Riwayat arus kas</h3><p>Penerimaan yang dikonfirmasi dan pengeluaran yang dicatat</p></div></div>
      <div className="table-wrap"><table><thead><tr><th>Tanggal</th><th>Bidang</th><th>Arus</th><th>Nominal</th><th>Keterangan</th><th>Dicatat oleh</th></tr></thead><tbody>
        {data.entries.map((entry,index)=><tr key={`${entry.kind}-${entry.id??entry.created_at}-${index}`}><td>{entry.date}</td><td>{entry.field_label}</td><td><span className={`cashbook-kind ${entry.kind==='MASUK'?'in':'out'}`}>{entry.kind==='MASUK'?<ArrowDownLeft size={14}/>:<ArrowUpRight size={14}/>}{entry.kind==='MASUK'?'Masuk':'Keluar'}</span></td><td><b>{money(entry.amount)}</b></td><td>{entry.note||'—'}</td><td>{entry.actor||'—'}</td></tr>)}
        {!data.entries.length&&<tr><td colSpan={6}>Belum ada arus kas terkonfirmasi atau pengeluaran tercatat.</td></tr>}
      </tbody></table></div>
    </section>
  </div>;
}
