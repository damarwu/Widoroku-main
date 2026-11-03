import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { PiggyBank, Plus, ChevronLeft, ChevronRight, Pencil, Trash2, Eye } from 'lucide-react';
import { apiFetch } from '../api';
import { SavingsRepository, type SavingsMeeting, type SavingsResident } from './SavingsRepository';
import { SavingsStage } from './SavingsStage';
import { SavingsLedgerEntry } from './SavingsLedgerEntry';
import { SavingsTransaction } from './SavingsTransaction';
import { SavingsSummary } from './SavingsSummary';
import Modal from '../shared/Modal';
import '../shared/modal.css';
import './savings.css';

const money = (amount: number) => new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 }).format(amount);

export default function SavingsPage({ canManage, onToast }: { canManage: boolean; onToast: (message: string) => void }) {
  const repository = useMemo(() => new SavingsRepository(), []);
  const [stages, setStages] = useState<SavingsStage[]>([]);
  const [meetings, setMeetings] = useState<SavingsMeeting[]>([]);
  const [residents, setResidents] = useState<SavingsResident[]>([]);
  const [ledger, setLedger] = useState<SavingsLedgerEntry[]>([]);
  const [transactions, setTransactions] = useState<SavingsTransaction[]>([]);
  const [transactionTotal, setTransactionTotal] = useState(0);
  const [summary,setSummary] = useState<SavingsSummary|null>(null);
  const [stageFilter, setStageFilter] = useState(0);
  const [classFilter, setClassFilter] = useState(0);
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const [resident, setResident] = useState('');
  const [period, setPeriod] = useState('');
  const [amount, setAmount] = useState('50000');
  const [busy, setBusy] = useState(false);
  const [depositModal,setDepositModal] = useState(false);
  const [stageDialog, setStageDialog] = useState<{ mode: 'add'|'edit'|'delete'; stage: SavingsStage|null }|null>(null);
  const [stageMeetingCount, setStageMeetingCount] = useState('2');
  const [transactionDialog, setTransactionDialog] = useState<{ mode: 'view'|'edit'|'delete'; transaction: SavingsTransaction }|null>(null);
  const [transactionAmount, setTransactionAmount] = useState('');
  const [transactionNote, setTransactionNote] = useState('');
  const pageSize = 10;
  const refresh = useCallback(async () => {
    try {
      const [settings, data, history, totals] = await Promise.all([repository.loadStages(), repository.loadLedger(stageFilter, classFilter, page, pageSize), repository.loadTransactions(stageFilter, page, pageSize), repository.loadSummary()]);
      setStages(settings.stages); setMeetings(settings.meetings); setResidents(settings.residents); setLedger(data.items); setTotal(data.total); setTransactions(history.items); setTransactionTotal(history.total); setSummary(totals);
    } catch (error) { onToast(error instanceof Error ? error.message : 'Gagal memuat tabungan'); }
  }, [repository, stageFilter, classFilter, page, onToast]);
  useEffect(() => { void refresh(); }, [refresh]);
  const eligibleMeetings = meetings.filter((meeting) => meeting.status === 'TERLAKSANA' && meeting.stage_number > 0);
  const selectedMeeting = meetings.find((meeting) => String(meeting.id) === period);
  const selectedResident = residents.find((item) => String(item.id) === resident);
  const maxPage = Math.max(1, Math.ceil(total / pageSize));
  const openStage = (mode:'add'|'edit'|'delete', stage:SavingsStage|null=null) => { setStageMeetingCount(String(stage?.meetingCount??2)); setStageDialog({mode,stage}); };
  const saveStage = async () => { if(!stageDialog)return; setBusy(true); try { if(stageDialog.mode==='add') await repository.createStage(Number(stageMeetingCount)); else if(stageDialog.mode==='edit'&&stageDialog.stage) await repository.updateStage(new SavingsStage(stageDialog.stage.stageNumber,Number(stageMeetingCount))); else if(stageDialog.mode==='delete'&&stageDialog.stage) await repository.deleteStage(stageDialog.stage.stageNumber); onToast(stageDialog.mode==='add'?'Tahap ditambahkan':stageDialog.mode==='edit'?'Tahap diperbarui':'Tahap dihapus'); setStageDialog(null); await refresh(); } catch (error) { onToast(error instanceof Error ? error.message : 'Perubahan tahap gagal'); } finally { setBusy(false); } };
  const openTransaction = (mode:'view'|'edit'|'delete', transaction:SavingsTransaction) => { setTransactionAmount(String(transaction.amount)); setTransactionNote(transaction.note); setTransactionDialog({mode,transaction}); };
  const saveTransaction = async () => { if(!transactionDialog)return; setBusy(true); try { if(transactionDialog.mode==='edit') await repository.updateTransaction(transactionDialog.transaction.id,Number(transactionAmount),transactionNote); else if(transactionDialog.mode==='delete') await repository.deleteTransaction(transactionDialog.transaction.id); onToast(transactionDialog.mode==='edit'?'Setoran diperbarui':'Setoran dihapus'); setTransactionDialog(null); await refresh(); } catch(error) { onToast(error instanceof Error?error.message:'Setoran gagal diperbarui'); } finally { setBusy(false); } };
  const deposit = async (event: FormEvent) => { event.preventDefault(); setBusy(true); try { await repository.deposit(Number(resident), Number(period), Number(amount)); onToast('Setoran tabungan tercatat dan masuk ke buku kas'); setAmount('50000'); setDepositModal(false); setPage(1); await refresh(); } catch (error) { onToast(error instanceof Error ? error.message : 'Gagal mencatat setoran'); } finally { setBusy(false); } };

  return <div className="savings-page">
    {summary&&<>
      <section className="savings-summary-grid" aria-label="Ringkasan Tabungan Jalan">
        <article className="savings-summary-card"><span>Total tabungan masuk</span><b>{money(summary.totalCollected)}</b><small>{summary.residentCount} warga terdaftar</small></article>
        <article className="savings-summary-card"><span>Target per tahap</span><b>{money(summary.targetPerStage)}</b><small>Akumulasi target semua kelas</small></article>
        <article className="savings-summary-card"><span>Target seluruh tahap</span><b>{money(summary.allStagesTarget)}</b><small>{summary.stageCount} tahap terkonfigurasi</small></article>
        <article className="savings-summary-card"><span>Realisasi pengeluaran</span><b>{money(summary.totalSpent)}</b><small>Penggunaan dana tercatat di Buku Kas</small></article>
        <article className="savings-summary-card emphasis"><span>Saldo akhir Tabungan Jalan</span><b>{money(summary.endingBalance)}</b><small>Total masuk dikurangi realisasi</small></article>
      </section>
      <section className="panel savings-ledger"><div className="panel-heading"><div><h3>Rekap pelunasan per tahap</h3><p>Jumlah warga yang belum mencapai target setoran setiap tahap.</p></div></div>
        <div className="table-wrap"><table><thead><tr><th>Tahap</th><th>Periode</th><th>Target tahap</th><th>Total masuk</th><th>Sisa target</th><th>Belum lunas</th></tr></thead><tbody>{summary.stages.map(item=><tr key={item.stageNumber}><td><b>Tahap {item.stageNumber}</b></td><td>{item.meetingCount} kumpulan</td><td>{money(item.target)}</td><td>{money(item.collected)}</td><td>{money(item.remaining)}</td><td><b>{item.unpaidResidents} warga</b></td></tr>)}</tbody></table>{!summary.stages.length&&<div className="empty"><p>Belum ada tahap tabungan.</p></div>}</div>
      </section>
      <section className="panel savings-ledger"><div className="panel-heading"><div><h3>Realisasi penggunaan dana</h3><p>Pengeluaran perbaikan jalan dan infrastruktur yang dicatat di Buku Kas.</p></div></div>
        <div className="table-wrap"><table><thead><tr><th>Tanggal</th><th>Penggunaan / Keterangan</th><th>Nominal</th></tr></thead><tbody>{summary.realizations.map(item=><tr key={item.id}><td>{item.date}</td><td>{item.note||'—'}</td><td><b>{money(item.amount)}</b></td></tr>)}</tbody></table>{!summary.realizations.length&&<div className="empty"><p>Belum ada realisasi pengeluaran.</p></div>}</div>
      </section>
    </>}
    {canManage && <section className="panel savings-settings"><div className="panel-heading"><div><h3>Periode tabungan per tahap</h3><p>Tentukan berapa kumpulan untuk menyelesaikan tiap tahap.</p></div><button className="primary-btn" type="button" onClick={()=>openStage('add')}><Plus size={15}/> Tambah tahap</button></div>
      <div className="table-wrap"><table><thead><tr><th>Tahap</th><th>Periode</th><th>Tagihan dibuat</th><th>Aksi</th></tr></thead><tbody>{stages.map(stage=><tr key={stage.stageNumber}><td><b>{stage.label}</b></td><td>{stage.meetingCount} kumpulan</td><td>{stage.billedResidents} warga</td><td><div className="savings-row-actions"><button className="secondary-btn" type="button" onClick={()=>openStage('edit',stage)} aria-label={`Ubah ${stage.label}`}><Pencil size={14}/></button><button className="secondary-btn danger" type="button" onClick={()=>openStage('delete',stage)} aria-label={`Hapus ${stage.label}`}><Trash2 size={14}/></button></div></td></tr>)}</tbody></table></div>
      <p className="hint">Tahap yang sudah memiliki setoran dikunci agar riwayat dan pembagian kumpulan tetap akurat.</p>
    </section>}
    {canManage && <section className="panel savings-deposit"><div className="panel-heading"><div><h3>Setoran tabungan</h3><p>Dana infrastruktur, tidak dapat ditarik warga.</p></div><button type="button" className="primary-btn" onClick={()=>setDepositModal(true)}><Plus size={15}/> Catat setoran</button></div></section>}
    <section className="panel savings-ledger"><div className="panel-heading"><div><h3>Buku tabungan per tahap</h3><p>Setoran dan sisa target per warga.</p></div><PiggyBank size={18}/></div>
      <div className="savings-class-tabs" role="tablist" aria-label="Kelas tabungan">{[{id:0,label:'Semua kelas'},{id:1,label:'Kelas 1'},{id:2,label:'Kelas 2'},{id:3,label:'Kelas 3'},{id:4,label:'Kelas 4 · Tidak wajib'}].map(item=><button type="button" role="tab" aria-selected={classFilter===item.id} className={classFilter===item.id?'active':''} onClick={()=>{setClassFilter(item.id);setPage(1)}} key={item.id}>{item.label}</button>)}</div>
      <div className="savings-filter"><label>Tampilkan tahap<select value={stageFilter} onChange={event => { setStageFilter(Number(event.target.value)); setPage(1); }}><option value={0}>Semua riwayat</option>{stages.map(stage => <option key={stage.stageNumber} value={stage.stageNumber}>{stage.label}</option>)}</select></label><span>{total} warga</span></div>
      <div className="table-wrap"><table><thead><tr><th>Tahap</th><th>Rumah / Warga</th><th>Kelas</th><th>Target tahap</th><th>Terkumpul</th><th>Sisa target</th></tr></thead><tbody>{ledger.map(row => <tr key={`${row.residentId}-${row.stage}`}><td>{row.stage ? `Tahap ${row.stage}` : 'Semua tahap'}</td><td><b>{row.houseCode}</b><small className="savings-name">{row.name}</small></td><td>{row.savingsClass===4?'Kelas 4 · Sukarela':`Kelas ${row.savingsClass}`}</td><td>{row.savingsClass===4?'Tidak wajib':money(row.tariff)}</td><td><b>{money(row.deposited)}</b></td><td>{row.savingsClass===4?'—':money(row.remaining)}</td></tr>)}</tbody></table>{!ledger.length && <div className="empty"><p>Belum ada setoran pada pilihan ini.</p></div>}</div>
      <div className="savings-pagination"><span>Halaman {page} dari {maxPage}</span><div><button className="secondary-btn" disabled={page<=1} onClick={() => setPage(value => value-1)}><ChevronLeft size={16}/> Sebelumnya</button><button className="secondary-btn" disabled={page>=maxPage} onClick={() => setPage(value => value+1)}>Berikutnya <ChevronRight size={16}/></button></div></div>
    </section>
    <section className="panel savings-ledger"><div className="panel-heading"><div><h3>Riwayat setoran</h3><p>Catatan penerimaan yang membentuk saldo tabungan. {transactionTotal} transaksi.</p></div></div>
      <div className="table-wrap"><table><thead><tr><th>Tanggal kumpulan</th><th>Tahap</th><th>Rumah / Warga</th><th>Nominal</th><th>Catatan</th><th>Aksi</th></tr></thead><tbody>{transactions.map(item => <tr key={item.id}><td>{item.meetingDate || '—'}</td><td>{item.stage ? `Tahap ${item.stage}` : '—'}</td><td><b>{item.houseCode}</b><small className="savings-name">{item.name}</small></td><td><b>{money(item.amount)}</b></td><td>{item.note || '—'}</td><td><div className="savings-row-actions"><button className="secondary-btn" onClick={()=>openTransaction('view',item)} aria-label="Lihat setoran"><Eye size={14}/></button>{canManage&&<><button className="secondary-btn" onClick={()=>openTransaction('edit',item)} aria-label="Ubah setoran"><Pencil size={14}/></button><button className="secondary-btn danger" onClick={()=>openTransaction('delete',item)} aria-label="Hapus setoran"><Trash2 size={14}/></button></>}</div></td></tr>)}</tbody></table>{!transactions.length && <div className="empty"><p>Belum ada riwayat setoran.</p></div>}</div>
      <div className="savings-pagination"><span>Halaman {page} dari {Math.max(1, Math.ceil(transactionTotal / pageSize))}</span><div><button className="secondary-btn" disabled={page<=1} onClick={() => setPage(value => value-1)}><ChevronLeft size={16}/> Sebelumnya</button><button className="secondary-btn" disabled={page>=Math.max(1, Math.ceil(transactionTotal/pageSize))} onClick={() => setPage(value => value+1)}>Berikutnya <ChevronRight size={16}/></button></div></div>
    </section>
    {stageDialog&&<Modal title={stageDialog.mode==='add'?'Tambah tahap tabungan':stageDialog.mode==='edit'?'Ubah tahap tabungan':'Hapus tahap tabungan'} subtitle="Atur berapa kumpulan untuk periode tahap ini." onClose={()=>setStageDialog(null)} size="small">
      {stageDialog.mode==='delete'?<p>Hapus {stageDialog.stage?.label}? Tahap hanya dapat dihapus bila belum memiliki transaksi dan merupakan tahap terakhir.</p>:<label className="savings-modal-field">{stageDialog.mode==='edit'?stageDialog.stage?.label:'Jumlah kumpulan'}<input type="number" min="1" max="100" value={stageMeetingCount} onChange={event=>setStageMeetingCount(event.target.value)}/><small>kumpulan untuk menyelesaikan tahap</small></label>}
      <div className="savings-modal-actions"><button type="button" className="ghost-btn" onClick={()=>setStageDialog(null)}>Batal</button>{stageDialog.mode==='delete'?<button type="button" className="primary-btn danger-btn" disabled={busy} onClick={()=>void saveStage()}>Hapus tahap</button>:<button type="button" className="primary-btn" disabled={busy||Number(stageMeetingCount)<1} onClick={()=>void saveStage()}>{stageDialog.mode==='add'?'Tambah tahap':'Simpan perubahan'}</button>}</div>
    </Modal>}
    {depositModal&&<Modal title="Catat setoran tabungan" subtitle="Setoran masuk sebagai penerimaan Tabungan Jalan di buku kas." onClose={()=>setDepositModal(false)} size="medium">
      <form id="savings-deposit-form" className="form-grid" onSubmit={event=>void deposit(event)}><label className="span-2">Warga / KK<select required value={resident} onChange={event=>setResident(event.target.value)}><option value="">Pilih warga</option>{residents.map(item=><option key={item.id} value={item.id}>{item.house_code} · {item.head_name} · Kelas {item.savings_class}{item.savings_class===4?' · Tidak wajib':''}</option>)}</select></label><label className="span-2">Kumpulan<select required value={period} onChange={event=>setPeriod(event.target.value)}><option value="">Pilih kumpulan terlaksana</option>{eligibleMeetings.map(item=><option key={item.id} value={item.id}>Tahap {item.stage_number} · {item.meeting_date}</option>)}</select></label><label className="span-2">Nominal (Rp)<input type="number" min="1" required value={amount} onChange={event=>setAmount(event.target.value)}/></label><p className="hint span-2">{selectedResident?.savings_class===4?'Kelas 4 tidak wajib ikut. Setoran yang dicatat bersifat sukarela. ':''}{selectedMeeting?`Dibukukan pada Tahap ${selectedMeeting.stage_number}.`: 'Pilih kumpulan yang sudah terlaksana.'} Saldo tidak dapat ditarik oleh warga.</p><div className="savings-modal-actions span-2"><button type="button" className="ghost-btn" onClick={()=>setDepositModal(false)}>Batal</button><button type="submit" className="primary-btn" disabled={busy||!resident||!period||Number(amount)<1}>Simpan setoran</button></div></form>
    </Modal>}
    {transactionDialog&&<Modal title={transactionDialog.mode==='view'?'Rincian setoran':transactionDialog.mode==='edit'?'Ubah setoran':'Hapus setoran'} subtitle={`${transactionDialog.transaction.houseCode} · ${transactionDialog.transaction.name} · Tahap ${transactionDialog.transaction.stage}`} onClose={()=>setTransactionDialog(null)} size="medium">
      {transactionDialog.mode==='view'?<div className="savings-detail"><p><span>Tanggal kumpulan</span><b>{transactionDialog.transaction.meetingDate||'—'}</b></p><p><span>Nominal</span><b>{money(transactionDialog.transaction.amount)}</b></p><p><span>Catatan</span><b>{transactionDialog.transaction.note||'—'}</b></p></div>:transactionDialog.mode==='edit'?<div className="form-grid"><label>Nominal setoran (Rp)<input type="number" min="1" value={transactionAmount} onChange={event=>setTransactionAmount(event.target.value)}/></label><label className="span-2">Catatan<input maxLength={500} value={transactionNote} onChange={event=>setTransactionNote(event.target.value)}/></label></div>:<p>Hapus setoran {money(transactionDialog.transaction.amount)} dari {transactionDialog.transaction.name}? Jika dana sudah dikeluarkan melalui buku kas dan saldo menjadi kurang, penghapusan akan ditolak.</p>}
      <div className="savings-modal-actions"><button className="ghost-btn" type="button" onClick={()=>setTransactionDialog(null)}>Tutup</button>{transactionDialog.mode==='edit'&&<button className="primary-btn" disabled={busy||Number(transactionAmount)<1} onClick={()=>void saveTransaction()}>Simpan perubahan</button>}{transactionDialog.mode==='delete'&&<button className="primary-btn danger-btn" disabled={busy} onClick={()=>void saveTransaction()}>Hapus setoran</button>}</div>
    </Modal>}
  </div>;
}
