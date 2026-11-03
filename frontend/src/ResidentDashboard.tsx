import { useEffect, useState, type FormEvent } from 'react';
import { ArrowDownLeft, CalendarDays, CircleAlert, Droplets, Gauge, LogOut, PiggyBank, ReceiptText, ShieldCheck, Users, Wallet } from 'lucide-react';
import { apiFetch as fetch } from './api';
import './resident-dashboard.css';

type DashboardData = {
  resident:{id:number;house_code:string;head_name:string;address:string;savings_class:number};
  bills:{id:number;description:string;meeting_date:string;amount:number;paid:number;remaining:number}[];
  bill_total:number;
  bill_due:number;
  pam_bills:{id:number;billing_month:string;meter_number:string;previous_reading:number;current_reading:number;consumption:number;total:number;paid:number;remaining:number;payment_method:string;payment_status:string;payment_review_note:string}[];
  pam_payment_history:{id:number;bill_id:number;billing_month:string;amount:number;method_code:string;note:string;received_at:string}[];
  pam_due:number;
  pam_customer?:{meter_number:string;last_reading:number;last_billing_month:string};
  pam_submission?:{billing_month:string;current_reading:number;status:string;review_note:string};
  pam_payment_methods:{code:string;name:string;instructions:string;active:boolean;account_number:string;image_data:string}[];
  savings_balance:number;
  savings_due:number;
  active_savings_stage:number;
  savings_stages:{stage:number;tariff:number;deposited:number;remaining:number}[];
  payments:{id:number;amount:number;note:string;date:string}[];
  upcoming_meetings:{date:string;time:string}[];
  ronda_schedule:{day_of_week:number;day:string;officers:string[]}[];
  jimpitan:{date:string;amount:number;status:string;note:string}[];
};

const money=(amount:number)=>new Intl.NumberFormat('id-ID',{style:'currency',currency:'IDR',maximumFractionDigits:0}).format(amount);
const currentMonth=()=>{const now=new Date();return `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,'0')}`};

export default function ResidentDashboard({onLogout,onConfigureAPI}:{onLogout:()=>void;onConfigureAPI:()=>void}){
  const [data,setData]=useState<DashboardData|null>(null);
  const [error,setError]=useState('');
  const [loading,setLoading]=useState(true);
  const [pamMonth,setPamMonth]=useState(currentMonth());
  const [pamReading,setPamReading]=useState('');
  const [pamSelections,setPamSelections]=useState<Record<number,string>>({});
  const [pamAmounts,setPamAmounts]=useState<Record<number,string>>({});
  const [pamProofs,setPamProofs]=useState<Record<number,string>>({});
  const [pamNotes,setPamNotes]=useState<Record<number,string>>({});
  const [pamMessage,setPamMessage]=useState('');

  const loadDashboard=async()=>{
    try{
      const response=await fetch('/api/resident/dashboard');
      const result=await response.json() as DashboardData|{error?:string};
      if(!response.ok)throw new Error((result as {error?:string}).error||'Gagal memuat dashboard warga');
      setData(result as DashboardData);
      setError('');
    }catch(loadError){setError(loadError instanceof Error?loadError.message:'Gagal memuat dashboard warga')}
    finally{setLoading(false)}
  };
  useEffect(()=>{void loadDashboard()},[]);

  const submitPamReading=async(event:FormEvent<HTMLFormElement>)=>{
    event.preventDefault();
    try{
      const response=await fetch('/api/resident/pam/readings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({billing_month:pamMonth,current_reading:Number(pamReading)})});
      const result=await response.json();
      if(!response.ok)throw new Error(result.error||'Gagal mengirim angka meter');
      setPamMessage(`Angka meter ${result.billing_month} terkirim dan menunggu verifikasi petugas.`);
      setPamReading('');
      await loadDashboard();
    }catch(actionError){setPamMessage(actionError instanceof Error?actionError.message:'Gagal mengirim angka meter')}
  };

  const choosePamPayment=async(billID:number,methodCode:string)=>{
    try{
      const response=await fetch('/api/resident/pam/payment-choice',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({bill_id:billID,method_code:methodCode})});
      const result=await response.json();
      if(!response.ok)throw new Error(result.error||'Gagal menyimpan pilihan pembayaran');
      setPamMessage(result.message||'Pilihan pembayaran PAM tersimpan.');
      setPamSelections(current=>({...current,[billID]:methodCode}));
      await loadDashboard();
    }catch(actionError){setPamMessage(actionError instanceof Error?actionError.message:'Gagal menyimpan pilihan pembayaran')}
  };

  const submitPamProof=async(billID:number,methodCode:string)=>{
    try{
      const response=await fetch('/api/resident/pam/payment-confirmations',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({bill_id:billID,amount:Number(pamAmounts[billID]),method_code:methodCode,proof_data:pamProofs[billID],note:pamNotes[billID]})});
      const result=await response.json();
      if(!response.ok)throw new Error(result.error||'Gagal mengirim bukti pembayaran');
      setPamProofs(current=>({...current,[billID]:''}));
      setPamNotes(current=>({...current,[billID]:''}));
      setPamMessage(result.message||'Bukti pembayaran terkirim untuk diverifikasi.');
      await loadDashboard();
    }catch(actionError){setPamMessage(actionError instanceof Error?actionError.message:'Gagal mengirim bukti pembayaran')}
  };
  const uploadPaymentProof=async(billID:number,file?:File)=>{
    if(!file)return;
    if(!['image/png','image/jpeg','image/webp'].includes(file.type)||file.size>1024*1024){
      setPamMessage('Bukti harus gambar PNG, JPEG, atau WebP maksimal 1 MB.');
      return;
    }
    try{
      const proof=await fileToDataURL(file);
      setPamProofs(current=>({...current,[billID]:proof}));
      setPamMessage('');
    }catch(uploadError){setPamMessage(uploadError instanceof Error?uploadError.message:'Bukti tidak dapat dibaca')}
  };

  if(loading)return <main className="resident-screen"><p>Memuat dashboard warga...</p></main>;
  if(error||!data)return <main className="resident-screen"><section className="resident-error"><CircleAlert size={24}/><h1>Dashboard belum dapat dimuat</h1><p>{error||'Data warga tidak tersedia.'}</p><button onClick={onLogout}><LogOut size={15}/> Keluar</button></section></main>;

  return <main className="resident-screen">
    <header className="resident-header"><a className="resident-brand" href="#"><span><Users size={19}/></span><b>Widoroku<small>PORTAL WARGA RT</small></b></a><div className="resident-account"><span>{data.resident.house_code} · {data.resident.head_name}</span><button onClick={onConfigureAPI}>Server</button><button onClick={onLogout}><LogOut size={15}/> Keluar</button></div></header>
    <div className="resident-content">
      <section className="resident-welcome"><div><span>PORTAL WARGA</span><h1>Sugeng rawuh, {data.resident.head_name}</h1><p>Pantau tagihan, tabungan, dan informasi kegiatan lingkungan dalam satu halaman.</p><small>Widoro, Potronayan, Nogosari, Boyolali</small></div><div className="resident-welcome-icon"><HomeIcon/></div></section>
      <div className="resident-stat-grid">
        <article><span className="resident-stat-icon amber"><ReceiptText size={19}/></span><small>Sisa tagihan iuran kas</small><b>{money(data.bill_due)}</b><span>{data.bills.filter(bill=>bill.remaining>0).length} tagihan belum lunas</span></article>
        <article><span className="resident-stat-icon cyan"><Droplets size={19}/></span><small>Sisa tagihan PAM</small><b>{money(data.pam_due)}</b><span>{data.pam_bills.filter(bill=>bill.remaining>0).length} tagihan belum lunas</span></article>
        <article><span className="resident-stat-icon green"><PiggyBank size={19}/></span><small>{data.resident.savings_class===4?'Status tabungan jalan':'Tagihan tabungan jalan'}</small><b>{data.resident.savings_class===4?'Tidak wajib':money(data.savings_due)}</b><span>{data.resident.savings_class===4?'Kelas 4 · boleh ikut secara sukarela':data.active_savings_stage?`Tahap ${data.active_savings_stage}`:'Belum ada tahap aktif'}</span></article>
        <article><span className="resident-stat-icon blue"><Wallet size={19}/></span><small>Saldo tabungan jalan</small><b>{money(data.savings_balance)}</b><span>Kelas {data.resident.savings_class}</span></article>
        <article><span className="resident-stat-icon violet"><ArrowDownLeft size={19}/></span><small>Total iuran kas</small><b>{money(data.bill_total)}</b><span>Akumulasi tagihan</span></article>
      </div>

      <div className="resident-columns">
        <section className="resident-card"><header><div><h2>Rincian tagihan iuran</h2><p>Data tagihan untuk KK Anda</p></div><ReceiptText size={19}/></header>
          {data.bills.length?<div className="resident-list">{data.bills.map(bill=><article key={bill.id}><div><b>{bill.description}</b><span>Jadwal {bill.meeting_date}</span></div><div className="resident-bill-values"><b>{money(bill.remaining)}</b><small>Dari {money(bill.amount)} · dibayar {money(bill.paid)}</small></div></article>)}</div>:<p className="resident-empty">Belum ada tagihan iuran untuk KK ini.</p>}
        </section>
        <section className="resident-card"><header><div><h2>{data.resident.savings_class===4?'Tabungan jalan':'Tagihan tabungan jalan'}</h2><p>{data.resident.savings_class===4?'Kelas 4 tidak wajib ikut iuran tabungan jalan. Setoran sukarela tetap dapat dicatat.':'Setoran dihitung sesuai periode tahap yang ditetapkan pengurus.'}</p></div><PiggyBank size={19}/></header>
          {data.resident.savings_class===4?<p className="resident-empty">Keluarga Anda tidak memiliki target setoran wajib untuk tabungan jalan.</p>:data.active_savings_stage?<div className="resident-savings-current"><span>Tahap aktif · {data.active_savings_stage}</span><b>{money(data.savings_due)}</b><small>Sisa setoran dari tarif {money(data.savings_stages[0]?.tariff||0)}</small></div>:<p className="resident-empty">Belum ada tahap tabungan jalan aktif. Tagihan muncul setelah periode kumpulan terlaksana.</p>}
          {data.resident.savings_class!==4&&data.savings_stages.length>1&&<div className="resident-list">{data.savings_stages.slice(1).map(stage=><article key={stage.stage}><div><b>Riwayat tahap {stage.stage}</b><span>Setoran {money(stage.deposited)} dari {money(stage.tariff)}</span></div><b>{stage.remaining?`Sisa ${money(stage.remaining)}`:'Lunas'}</b></article>)}</div>}
        </section>
      </div>

      <section className="resident-card resident-pam-card"><header><div><h2>Tagihan PAM</h2><p>Rincian pemakaian air, pembayaran, dan saldo tagihan meter KK Anda</p></div><Droplets size={19}/></header>
        {pamMessage&&<p className="resident-pam-message" role="status">{pamMessage}</p>}
        {data.pam_customer&&<div className="resident-meter-entry">
          <h3><Gauge size={17}/> Catat meter mandiri</h3>
          <p>Nomor meter {data.pam_customer.meter_number} · pembacaan terakhir disetujui {data.pam_customer.last_reading}{data.pam_customer.last_billing_month&&` (${data.pam_customer.last_billing_month.slice(0,7)})`}.</p>
          {data.pam_submission&&<p className={`resident-meter-status ${data.pam_submission.status==='PENDING'?'pending':'rejected'}`}>{data.pam_submission.status==='PENDING'?'Pengajuan meter menunggu verifikasi':`Pengajuan ${data.pam_submission.billing_month.slice(0,7)} ditolak`}{data.pam_submission.review_note&&`: ${data.pam_submission.review_note}`}</p>}
          <form onSubmit={submitPamReading}><label>Bulan<input type="month" required max={currentMonth()} value={pamMonth} onChange={event=>setPamMonth(event.target.value)}/></label><label>Angka meter sekarang<input type="number" min={data.pam_customer.last_reading} step="1" required value={pamReading} onChange={event=>setPamReading(event.target.value)}/></label><button type="submit" className="resident-pam-button">Kirim untuk diverifikasi</button></form>
        </div>}
        {data.pam_bills.length?<div className="table-wrap"><table><thead><tr><th>Bulan</th><th>Meter</th><th>Pemakaian</th><th>Tagihan</th><th>Dibayar</th><th>Sisa</th><th>Pembayaran</th></tr></thead><tbody>{data.pam_bills.map(bill=>{
          const chosenCode=pamSelections[bill.id]??bill.payment_method;
          const chosenMethod=data.pam_payment_methods.find(method=>method.code===chosenCode);
          const availableMethods=data.pam_payment_methods.filter(method=>method.active);
          return <tr key={bill.id}><td>{bill.billing_month.slice(0,7)}</td><td>{bill.meter_number}<small>{bill.previous_reading} → {bill.current_reading}</small></td><td>{bill.consumption} m³</td><td>{money(bill.total)}</td><td>{money(bill.paid)}</td><td><b className={bill.remaining>0?'resident-pam-due':'resident-pam-paid'}>{money(bill.remaining)}</b></td><td>{bill.remaining>0?<div className="resident-payment-choice"><select disabled={bill.payment_status==='PENDING'} aria-label={`Metode pembayaran PAM ${bill.billing_month}`} value={chosenCode} onChange={event=>setPamSelections(current=>({...current,[bill.id]:event.target.value}))}><option value="">Pilih metode</option>{availableMethods.map(method=><option key={method.code} value={method.code}>{method.name}</option>)}</select><button type="button" className="resident-pam-button" disabled={!chosenCode||chosenCode===bill.payment_method||bill.payment_status==='PENDING'} onClick={()=>void choosePamPayment(bill.id,chosenCode)}>Simpan pilihan</button>{chosenMethod&&<div className="resident-payment-instructions"><b>{chosenMethod.name}</b>{chosenMethod.account_number&&<span>Nomor rekening: {chosenMethod.account_number}</span>}{chosenMethod.image_data&&<img src={chosenMethod.image_data} alt={chosenMethod.name}/>}<span>{chosenMethod.instructions}</span></div>}{bill.payment_status==='PENDING'?<small className="resident-meter-status pending">Bukti pembayaran menunggu verifikasi petugas</small>:bill.payment_status==='REJECTED'?<small className="resident-meter-status rejected">Bukti ditolak: {bill.payment_review_note}</small>:chosenMethod?.code==='collection'?<small>Pembayaran akan diambil petugas. Hubungi petugas untuk konfirmasi pelunasan.</small>:bill.payment_method&&<div className="resident-payment-proof"><label>Nominal transfer/bayar<input type="number" min="1" max={bill.remaining} value={pamAmounts[bill.id]??String(bill.remaining)} onChange={event=>setPamAmounts(current=>({...current,[bill.id]:event.target.value}))}/></label><label>Bukti pembayaran (PNG/JPEG/WebP, maks. 1 MB)<input type="file" accept="image/png,image/jpeg,image/webp" onChange={event=>void uploadPaymentProof(bill.id,event.target.files?.[0])}/></label>{pamProofs[bill.id]&&<img src={pamProofs[bill.id]} alt="Pratinjau bukti pembayaran"/>}<label>Catatan / nomor referensi<input maxLength={500} value={pamNotes[bill.id]||''} onChange={event=>setPamNotes(current=>({...current,[bill.id]:event.target.value}))}/></label><button type="button" className="resident-pam-button" disabled={bill.payment_method!==chosenCode||!pamProofs[bill.id]||!Number(pamAmounts[bill.id]??bill.remaining)||Number(pamAmounts[bill.id]??bill.remaining)>bill.remaining} onClick={()=>void submitPamProof(bill.id,chosenCode)}>Kirim bukti pembayaran</button></div>}</div>:<span className="resident-pam-paid">Lunas</span>}</td></tr>
        })}</tbody></table></div>:<p className="resident-empty">Belum ada tagihan PAM untuk KK ini.</p>}
        <div className="resident-pam-history"><h3>Riwayat pembayaran PAM</h3><p>Penerimaan yang sudah dicatat petugas untuk KK Anda.</p>{data.pam_payment_history.length?<div className="table-wrap"><table><thead><tr><th>Tanggal diterima</th><th>Bulan tagihan</th><th>Metode</th><th>Nominal</th><th>Catatan</th></tr></thead><tbody>{data.pam_payment_history.map(payment=>{const method=data.pam_payment_methods.find(item=>item.code===payment.method_code);return <tr key={payment.id}><td>{payment.received_at.slice(0,16).replace('T',' ')}</td><td>{payment.billing_month.slice(0,7)}</td><td>{method?.name||payment.method_code}</td><td><b>{money(payment.amount)}</b></td><td>{payment.note||'—'}</td></tr>})}</tbody></table></div>:<p className="resident-empty">Belum ada pembayaran PAM yang tercatat.</p>}</div>
        <p className="resident-pam-footnote">Pilihan pembayaran membantu petugas menyiapkan penerimaan. Tagihan dianggap lunas setelah pembayaran diterima dan dicatat petugas.</p>
      </section>

      <div className="resident-columns">
        <section className="resident-card"><header><div><h2>Jadwal ronda</h2><p>Petugas ronda setiap hari</p></div><ShieldCheck size={19}/></header>
          <div className="resident-week">{data.ronda_schedule.map(day=><article key={day.day_of_week}><b>{day.day}</b><span>{day.officers.length?day.officers.join(', '):'Belum ada petugas dijadwalkan'}</span></article>)}</div>
        </section>
        <section className="resident-card"><header><div><h2>Agenda dan aktivitas</h2><p>Informasi kegiatan lingkungan</p></div><CalendarDays size={19}/></header>
          {data.upcoming_meetings.length?<div className="resident-list">{data.upcoming_meetings.map(meeting=><article key={meeting.date}><div><b>Kumpulan Ahad Pon</b><span>{meeting.date}</span></div><b>{meeting.time.slice(0,5)} WIB</b></article>)}</div>:<p className="resident-empty">Belum ada jadwal kumpulan berikutnya.</p>}
          <div className="resident-activity-note"><Droplets size={16}/><span>Informasi pelayanan warga dan pengumuman RT akan ditampilkan di sini.</span></div>
        </section>
      </div>

      <div className="resident-columns">
        <section className="resident-card"><header><div><h2>Riwayat pembayaran</h2><p>10 pembayaran terakhir untuk KK Anda</p></div><Wallet size={19}/></header>
          {data.payments.length?<div className="resident-list">{data.payments.map(payment=><article key={payment.id}><div><b>{money(payment.amount)}</b><span>{payment.date} · {payment.note||'Pembayaran iuran kas'}</span></div></article>)}</div>:<p className="resident-empty">Belum ada pembayaran tercatat.</p>}
        </section>
        <section className="resident-card"><header><div><h2>Catatan jimpitan</h2><p>5 pencatatan terakhir di rumah Anda</p></div><ArrowDownLeft size={19}/></header>
          {data.jimpitan.length?<div className="resident-list">{data.jimpitan.map((entry,index)=><article key={`${entry.date}-${index}`}><div><b>{entry.date} · {entry.status==='TERAMBIL'?money(entry.amount):entry.status}</b><span>{entry.note||'Catatan kunjungan jimpitan'}</span></div></article>)}</div>:<p className="resident-empty">Belum ada catatan jimpitan untuk KK ini.</p>}
        </section>
      </div>
      <footer className="resident-footer">Widoroku RT · Rukun · Aman · Transparan</footer>
    </div>
  </main>;
}

function HomeIcon(){return <Users size={35}/>}

function fileToDataURL(file:File):Promise<string>{
  return new Promise((resolve,reject)=>{
    const reader=new FileReader();
    reader.onerror=()=>reject(new Error('Gambar tidak dapat dibaca'));
    reader.onload=()=>typeof reader.result==='string'?resolve(reader.result):reject(new Error('Gambar tidak valid'));
    reader.readAsDataURL(file);
  });
}
