import { ArrowDownLeft, ArrowRight, CalendarDays, Check, Droplets, HandHeart, Images, LogIn, PiggyBank, ReceiptText, ShieldCheck, Users, Wallet } from 'lucide-react';
import './public-home.css';

const activities=[
  {category:'Kebersamaan',title:'Gotong royong lingkungan',description:'Wadah informasi untuk pengumuman kerja bakti, kebersihan lingkungan, dan kegiatan kebersamaan warga.',icon:HandHeart},
  {category:'Keamanan',title:'Jadwal ronda malam',description:'Lihat pembagian jadwal petugas ronda setiap hari. Satu malam dapat dijaga oleh beberapa petugas.',icon:ShieldCheck},
  {category:'Pertemuan warga',title:'Kumpulan dan musyawarah',description:'Informasi jadwal pertemuan warga serta pengumuman penting dari pengurus RT.',icon:CalendarDays},
];

const gallery=[
  {src:'https://images.unsplash.com/photo-1511632765486-a01980e01a18?auto=format&fit=crop&w=900&q=80',alt:'Warga berkumpul bersama',caption:'Kebersamaan warga'},
  {src:'https://images.unsplash.com/photo-1529156069898-49953e39b3ac?auto=format&fit=crop&w=900&q=80',alt:'Sekelompok orang di kegiatan luar ruang',caption:'Kegiatan lingkungan'},
  {src:'https://images.unsplash.com/photo-1517457373958-b7bdd4587205?auto=format&fit=crop&w=900&q=80',alt:'Pertemuan dan kegiatan komunitas',caption:'Ruang untuk berbagi cerita'},
];

const services=[
  {title:'Data warga',description:'Informasi rumah dan kepala keluarga.',icon:Users},
  {title:'Jimpitan',description:'Pencatatan dan serah terima hasil jimpitan.',icon:ArrowDownLeft},
  {title:'Iuran dan pembayaran',description:'Ringkasan tagihan serta pembayaran kas.',icon:ReceiptText},
  {title:'Tabungan jalan',description:'Pencatatan saldo dan transaksi peserta.',icon:PiggyBank},
  {title:'PAM',description:'Pencatatan meter air dan pembayaran.',icon:Droplets},
  {title:'Jadwal ronda',description:'Jadwal petugas dan pencatatan kehadiran.',icon:ShieldCheck},
  {title:'Kumpulan Ahad Pon',description:'Jadwal pertemuan dan informasi undangan.',icon:CalendarDays},
  {title:'Laporan dan kas',description:'Dukungan administrasi dan transparansi RT.',icon:Wallet},
];

export default function PublicHome({onResidentLogin,onAdminLogin}:{onResidentLogin:()=>void;onAdminLogin:()=>void}){
  return <main className="public-home">
    <header className="public-header">
      <a className="public-brand" href="#beranda" aria-label="Widoroku - beranda"><span><Users size={20}/></span><b>Widoroku<small>PORTAL WARGA RT</small></b></a>
      <nav aria-label="Navigasi beranda"><a href="#layanan">Layanan</a><a href="#informasi">Informasi</a><a href="#galeri">Galeri</a></nav>
      <div className="public-login-actions"><button className="public-login public-login-resident" onClick={onResidentLogin}><LogIn size={16}/> Masuk warga</button><button className="public-login public-login-admin" onClick={onAdminLogin}><ShieldCheck size={16}/> Masuk pengurus</button></div>
    </header>

    <section className="public-hero" id="beranda">
      <div className="public-hero-copy">
        <span className="public-kicker"><span/> PORTAL INFORMASI WARGA</span>
        <h1>Guyub warganya.<br/><em>Mudah urusannya.</em></h1>
        <p>Ruang informasi dan administrasi warga untuk menjaga lingkungan tetap rukun, aman, dan transparan.</p>
        <div className="public-hero-actions"><a href="#informasi">Lihat informasi <ArrowRight size={16}/></a><button onClick={onResidentLogin}><LogIn size={16}/> Masuk dashboard warga</button></div>
        <div className="public-hero-note"><Check size={15}/> Informasi layanan RT dalam satu tempat</div>
      </div>
      <div className="public-hero-image" role="img" aria-label="Ilustrasi kegiatan kebersamaan komunitas"><span>Guyub · Rukun · Migunani</span></div>
    </section>

    <section className="public-services" id="layanan">
      <div className="public-section-heading"><div><span>LAYANAN WARGA</span><h2>Informasi RT, lebih mudah dijangkau</h2></div><p>Kenali layanan dan kegiatan administrasi yang didukung portal Widoroku.</p></div>
      <div className="public-service-grid">{services.map(({title,description,icon:Icon})=><article className="public-service-card" key={title}><span><Icon size={19}/></span><h3>{title}</h3><p>{description}</p></article>)}</div>
    </section>

    <section className="public-activities" id="informasi">
      <div className="public-section-heading"><div><span>KABAR LINGKUNGAN</span><h2>Informasi kegiatan warga</h2></div><p>Pengumuman dan kabar kegiatan dapat dibagikan di sini.</p></div>
      <div className="public-activity-grid">{activities.map(({category,title,description,icon:Icon})=><article className="public-activity-card" key={title}><div className="public-activity-icon"><Icon size={20}/></div><span>{category}</span><h3>{title}</h3><p>{description}</p><div className="public-activity-meta"><CalendarDays size={14}/> Informasi umum warga</div></article>)}</div>
      <p className="public-content-note">Pengumuman kegiatan aktual akan ditampilkan setelah dipublikasikan oleh pengurus.</p>
    </section>

    <section className="public-gallery" id="galeri">
      <div className="public-section-heading"><div><span>MOMEN WARGA</span><h2>Galeri kegiatan</h2></div><p>Ruang untuk menyimpan cerita dan dokumentasi kebersamaan.</p></div>
      <div className="public-gallery-grid">{gallery.map(photo=><figure key={photo.src}><img src={photo.src} alt={photo.alt} loading="lazy"/><figcaption><Images size={14}/>{photo.caption}<small>Foto ilustrasi</small></figcaption></figure>)}</div>
      <p className="public-content-note">Foto pada galeri ini adalah ilustrasi. Dokumentasi kegiatan warga dapat ditambahkan oleh pengurus.</p>
    </section>

    <section className="public-cta"><div className="public-cta-icon"><HandHeart size={24}/></div><div><span>BERSAMA MEMBANGUN LINGKUNGAN</span><h2>Warga terhubung, lingkungan semakin guyub.</h2><p>Portal administrasi digunakan oleh pengurus RT untuk membantu pencatatan dan pelayanan warga.</p></div><div className="public-cta-actions"><button onClick={onResidentLogin}>Masuk warga <ArrowRight size={16}/></button><button onClick={onAdminLogin}>Masuk pengurus <ArrowRight size={16}/></button></div></section>
    <footer className="public-footer"><a className="public-brand" href="#beranda"><span><Users size={18}/></span><b>Widoroku<small>PORTAL WARGA RT</small></b></a><p>Rukun · Aman · Transparan</p><small>© {new Date().getFullYear()} Widoroku RT</small></footer>
  </main>;
}
