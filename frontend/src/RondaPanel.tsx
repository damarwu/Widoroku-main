import { FormEvent, useEffect, useState } from 'react';
import { ArrowDownLeft, CalendarDays, Download, Printer, Save, ShieldCheck } from 'lucide-react';
import { apiFetch as fetch } from './api';
import './ronda.css';

type Props = { section:'handover'|'schedule'|'attendance'|'audit'; canManage:boolean };
type NightlyTotal = { date:string; amount:number };
type Handover = { id:number; session_date:string; total_amount:number; coordinator_one:string; coordinator_two:string; note:string; handed_over_at:string };
type ScheduleDay = { day_of_week:number; day:string; officers:string[] };
type Attendance = { officer_name:string; status:string; note:string };
type AuditEntry = { id:number; changed_at:string; changed_by:string; before_schedule:ScheduleDay[]; after_schedule:ScheduleDay[] };
type ApiResult = { error?:string; message?:string };

const weekdays=['Senin','Selasa','Rabu','Kamis','Jumat','Sabtu','Minggu'];
const money=(amount:number)=>new Intl.NumberFormat('id-ID',{style:'currency',currency:'IDR',maximumFractionDigits:0}).format(amount);
const localDate=()=>{const date=new Date();return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')}`};
const csvCell=(value:string|number)=>`"${String(value).replaceAll('"','""')}"`;
const downloadCSV=(rows:string[][],filename:string)=>{
  const content='\uFEFF'+rows.map(row=>row.map(csvCell).join(',')).join('\r\n');
  const url=URL.createObjectURL(new Blob([content],{type:'text/csv;charset=utf-8'}));
  const link=document.createElement('a');link.href=url;link.download=filename;link.click();URL.revokeObjectURL(url);
};
const scheduleChanges=(entry:AuditEntry)=>weekdays.map((day,index)=>{
  const before=entry.before_schedule.find(item=>item.day_of_week===index+1)?.officers||[];
  const after=entry.after_schedule.find(item=>item.day_of_week===index+1)?.officers||[];
  return {day,before,after};
}).filter(change=>JSON.stringify(change.before)!==JSON.stringify(change.after));

export default function RondaPanel({section,canManage}:Props){
  const [nightlyTotals,setNightlyTotals]=useState<NightlyTotal[]>([]);
  const [handovers,setHandovers]=useState<Handover[]>([]);
  const [schedule,setSchedule]=useState<string[][]>(weekdays.map(()=>[]));
  const [attendance,setAttendance]=useState<Attendance[]>([]);
  const [attendanceDate,setAttendanceDate]=useState(localDate);
  const [audit,setAudit]=useState<AuditEntry[]>([]);
  const [loading,setLoading]=useState(true);
  const [saving,setSaving]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const [sessionDate,setSessionDate]=useState('');
  const [coordinatorOne,setCoordinatorOne]=useState('');
  const [coordinatorTwo,setCoordinatorTwo]=useState('');
  const [note,setNote]=useState('');

  const load=async()=>{
    setLoading(true);
    setError('');
    try{
      if(section==='handover'){
        const [jimpitanResponse,handoverResponse]=await Promise.all([fetch('/api/jimpitan/nightly-totals'),fetch('/api/jimpitan/handovers')]);
        const jimpitanData=await jimpitanResponse.json() as NightlyTotal[]|ApiResult;
        const handoverData=await handoverResponse.json() as Handover[]|ApiResult;
        if(!jimpitanResponse.ok)throw new Error((jimpitanData as ApiResult).error||'Gagal memuat data jimpitan');
        if(!handoverResponse.ok)throw new Error((handoverData as ApiResult).error||'Gagal memuat serah terima');
        setNightlyTotals(jimpitanData as NightlyTotal[]);
        setHandovers(handoverData as Handover[]);
        const latest= (handoverData as Handover[])[0];
        if(latest){setCoordinatorOne(current=>current||latest.coordinator_one);setCoordinatorTwo(current=>current||latest.coordinator_two)}
      }else if(section==='schedule'){
        const response=await fetch('/api/ronda/schedule');
        const data=await response.json() as ScheduleDay[]|ApiResult;
        if(!response.ok)throw new Error((data as ApiResult).error||'Gagal memuat jadwal ronda');
        const days=data as ScheduleDay[];
        setSchedule(weekdays.map((_,index)=>days.find(day=>day.day_of_week===index+1)?.officers||[]));
      }else if(section==='attendance'){
        await loadAttendance(attendanceDate);
      }else{
        const response=await fetch('/api/ronda/audit');
        const data=await response.json() as AuditEntry[]|ApiResult;
        if(!response.ok)throw new Error((data as ApiResult).error||'Gagal memuat riwayat perubahan jadwal');
        setAudit(data as AuditEntry[]);
      }
    }catch(loadError){setError(loadError instanceof Error?loadError.message:'Gagal memuat data')}
    finally{setLoading(false)}
  };

  useEffect(()=>{void load()},[section]);

  const loadAttendance=async(date:string)=>{
    setAttendance([]);
    const response=await fetch(`/api/ronda/attendance?date=${encodeURIComponent(date)}`);
    const data=await response.json() as Attendance[]|ApiResult;
    if(!response.ok)throw new Error((data as ApiResult).error||'Gagal memuat kehadiran ronda');
    setAttendance(data as Attendance[]);
  };

  const handedOverDates=new Set(handovers.map(item=>item.session_date));
  const availableNights=nightlyTotals.filter(night=>night.amount>0&&!handedOverDates.has(night.date));

  const sendHandover=async(event:FormEvent)=>{
    event.preventDefault();
    setSaving(true);setError('');setNotice('');
    try{
      const response=await fetch('/api/jimpitan/handovers',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_date:sessionDate,coordinator_one:coordinatorOne,coordinator_two:coordinatorTwo,note})});
      const data=await response.json() as ApiResult;
      if(!response.ok)throw new Error(data.error||'Serah terima gagal disimpan');
      setNotice(data.message||'Serah terima berhasil disimpan');setSessionDate('');setNote('');
      await load();
    }catch(saveError){setError(saveError instanceof Error?saveError.message:'Serah terima gagal disimpan')}
    finally{setSaving(false)}
  };

  const saveSchedule=async(event:FormEvent)=>{
    event.preventDefault();
    setSaving(true);setError('');setNotice('');
    try{
      const response=await fetch('/api/ronda/schedule',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(schedule.map((officers,index)=>({day_of_week:index+1,officers:officers.join('\n').split('\n')})))});
      const data=await response.json() as ApiResult;
      if(!response.ok)throw new Error(data.error||'Jadwal ronda gagal disimpan');
      setNotice(data.message||'Jadwal ronda berhasil disimpan');
      await load();
    }catch(saveError){setError(saveError instanceof Error?saveError.message:'Jadwal ronda gagal disimpan')}
    finally{setSaving(false)}
  };

  const saveAttendance=async(event:FormEvent)=>{
    event.preventDefault();
    setSaving(true);setError('');setNotice('');
    try{
      const response=await fetch('/api/ronda/attendance',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({session_date:attendanceDate,officers:attendance})});
      const data=await response.json() as ApiResult;
      if(!response.ok)throw new Error(data.error||'Kehadiran ronda gagal disimpan');
      setNotice(data.message||'Kehadiran ronda berhasil disimpan');
      await loadAttendance(attendanceDate);
    }catch(saveError){setError(saveError instanceof Error?saveError.message:'Kehadiran ronda gagal disimpan')}
    finally{setSaving(false)}
  };

  const exportHandovers=()=>{
    const rows=[['Tanggal malam','Jumlah diserahkan','Koordinator 1','Koordinator 2','Catatan'],...handovers.map(item=>[item.session_date,String(item.total_amount),item.coordinator_one,item.coordinator_two,item.note])];
    downloadCSV(rows,'laporan-serah-terima-jimpitan.csv');
  };

  if(section==='handover')return <div className="work-grid ronda-grid">
    <section className="panel form-panel">
      <div className="panel-heading"><div><h3>Serah terima uang jimpitan</h3><p>Total dihitung dari catatan berhasil diambil pada malam yang dipilih.</p></div><ArrowDownLeft/></div>
      {error&&<div className="notice" role="alert">{error}</div>}
      {notice&&<div className="ronda-success" role="status">{notice}</div>}
      {canManage?<form className="form" onSubmit={sendHandover}>
        <label>Malam pengambilan jimpitan<select required value={sessionDate} onChange={event=>setSessionDate(event.target.value)}><option value="">Pilih tanggal</option>{availableNights.map(night=><option key={night.date} value={night.date}>{night.date} · {money(night.amount)}</option>)}</select></label>
        <div className="ronda-coordinators">
          <label>Koordinator jimpitan 1<input required maxLength={120} value={coordinatorOne} onChange={event=>setCoordinatorOne(event.target.value)} placeholder="Nama koordinator pertama"/></label>
          <label>Koordinator jimpitan 2<input required maxLength={120} value={coordinatorTwo} onChange={event=>setCoordinatorTwo(event.target.value)} placeholder="Nama koordinator kedua"/></label>
        </div>
        {sessionDate&&<div className="ronda-total"><span>Jumlah yang diserahkan</span><b>{money(nightlyTotals.find(night=>night.date===sessionDate)?.amount||0)}</b></div>}
        <label>Catatan (opsional)<input maxLength={1000} value={note} onChange={event=>setNote(event.target.value)} placeholder="Contoh: uang diserahkan setelah ronda"/></label>
        <button className="primary-btn" disabled={saving||loading||!availableNights.length}><Save size={16}/>{saving?'Menyimpan...':'Catat serah terima'}</button>
        {!loading&&!availableNights.length&&<p className="hint">Belum ada malam dengan jimpitan terkumpul yang belum diserahterimakan.</p>}
      </form>:<p className="hint">Anda memiliki akses lihat saja untuk data serah terima.</p>}
    </section>
    <section className="panel ronda-print-area">
      <div className="panel-heading"><div><h3>Laporan serah terima jimpitan</h3><p>{handovers.length} malam tercatat</p></div></div>
      <div className="ronda-export-actions ronda-no-print"><button type="button" className="text-link" disabled={loading||!handovers.length} onClick={exportHandovers}><Download size={15}/> Unduh Excel (CSV)</button><button type="button" className="text-link" disabled={loading||!handovers.length} onClick={()=>window.print()}><Printer size={15}/> Cetak / Simpan PDF</button></div>
      {loading?<p className="hint">Memuat data...</p>:<div className="table-wrap"><table><thead><tr><th>Tanggal</th><th>Jumlah</th><th>Koordinator 1</th><th>Koordinator 2</th><th>Catatan</th></tr></thead><tbody>{handovers.map(item=><tr key={item.id}><td>{item.session_date}</td><td><b>{money(item.total_amount)}</b></td><td>{item.coordinator_one}</td><td>{item.coordinator_two}</td><td>{item.note||'—'}</td></tr>)}</tbody></table>{!handovers.length&&<p className="hint">Belum ada catatan serah terima.</p>}</div>}
    </section>
  </div>;

  if(section==='attendance')return <div className="work-grid ronda-grid">
    <section className="panel">
      <div className="panel-heading"><div><h3>Kehadiran petugas ronda</h3><p>Daftar petugas mengikuti jadwal untuk hari yang dipilih.</p></div><ShieldCheck/></div>
      {error&&<div className="notice" role="alert">{error}</div>}
      {notice&&<div className="ronda-success" role="status">{notice}</div>}
      <div className="ronda-attendance-date"><label>Tanggal ronda<input type="date" value={attendanceDate} onChange={async event=>{const date=event.target.value;setAttendanceDate(date);setLoading(true);setError('');try{await loadAttendance(date)}catch(loadError){setError(loadError instanceof Error?loadError.message:'Gagal memuat kehadiran')}finally{setLoading(false)}}}/></label></div>
      {loading?<p className="hint">Memuat daftar petugas...</p>:attendance.length?<form className="form" onSubmit={saveAttendance}>
        <div className="table-wrap"><table><thead><tr><th>Petugas</th><th>Status hadir</th><th>Catatan</th></tr></thead><tbody>{attendance.map((item,index)=><tr key={item.officer_name}><td><b>{item.officer_name}</b></td><td><select required disabled={!canManage} aria-label={`Kehadiran ${item.officer_name}`} value={item.status} onChange={event=>setAttendance(current=>current.map((row,rowIndex)=>rowIndex===index?{...row,status:event.target.value}:row))}><option value="">Belum dicatat</option><option value="HADIR">Hadir</option><option value="ALPA">Tidak hadir</option></select></td><td><input disabled={!canManage} aria-label={`Catatan ${item.officer_name}`} maxLength={500} value={item.note} onChange={event=>setAttendance(current=>current.map((row,rowIndex)=>rowIndex===index?{...row,note:event.target.value}:row))}/></td></tr>)}</tbody></table></div>
        {canManage&&<button className="primary-btn" disabled={saving}><Save size={16}/>{saving?'Menyimpan...':'Simpan kehadiran'}</button>}
      </form>:<p className="hint">Belum ada petugas ronda terjadwal untuk tanggal ini.</p>}
    </section>
  </div>;

  if(section==='audit')return <div className="work-grid ronda-grid">
    <section className="panel">
      <div className="panel-heading"><div><h3>Riwayat perubahan jadwal ronda</h3><p>Perubahan disimpan otomatis setiap jadwal mengalami perubahan.</p></div><CalendarDays/></div>
      {error&&<div className="notice" role="alert">{error}</div>}
      {loading?<p className="hint">Memuat riwayat...</p>:audit.map(entry=><article className="ronda-audit-entry" key={entry.id}>
        <header><b>{new Date(entry.changed_at).toLocaleString('id-ID')}</b><span>Diubah oleh {entry.changed_by}</span></header>
        <div className="ronda-audit-changes">{scheduleChanges(entry).map(change=><div key={change.day}><b>{change.day}</b><span>{change.before.join(', ')||'Belum ada petugas'} <i>→</i> {change.after.join(', ')||'Belum ada petugas'}</span></div>)}</div>
      </article>)}
      {!loading&&!audit.length&&<p className="hint">Belum ada perubahan jadwal yang tercatat.</p>}
    </section>
  </div>;

  return <div className="work-grid ronda-grid">
    <section className="panel">
      <div className="panel-heading"><div><h3>Jadwal ronda malam</h3><p>Atur petugas untuk setiap hari Senin sampai Minggu.</p></div><ShieldCheck/></div>
      {error&&<div className="notice" role="alert">{error}</div>}
      {notice&&<div className="ronda-success" role="status">{notice}</div>}
      {loading?<p className="hint">Memuat jadwal...</p>:<form className="ronda-schedule-form" onSubmit={saveSchedule}>
        {weekdays.map((day,index)=><label className="ronda-day" key={day}><span><CalendarDays size={16}/>{day}</span><textarea aria-label={`Petugas ronda ${day}`} rows={3} disabled={!canManage} value={schedule[index].join('\n')} onChange={event=>setSchedule(current=>current.map((officers,dayIndex)=>dayIndex===index?event.target.value.split('\n'):officers))} placeholder={'Satu nama per baris\nBisa lebih dari satu petugas'}/><small>Pisahkan nama petugas dengan baris baru.</small></label>)}
        {canManage&&<button className="primary-btn" disabled={saving}><Save size={16}/>{saving?'Menyimpan...':'Simpan jadwal ronda'}</button>}
      </form>}
    </section>
  </div>;
}
