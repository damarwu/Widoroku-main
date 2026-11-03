# Widoroku RT — Go API + Web/PWA

Starter project untuk administrasi RT: data warga, periode kumpulan 35 hari, tagihan, pembayaran cicilan/kolektif, jimpitan QR, tabungan jalan, ronda, dan laporan.

## Jalankan di GitHub Codespaces
1. Buat repository GitHub baru, upload seluruh isi folder ini, lalu pilih **Code → Codespaces → Create codespace**.
2. Di terminal Codespaces:
   ```bash
   docker compose up -d db
   cd backend && go mod tidy && cd ..
   docker compose up --build
   ```
3. Buka tab **Ports**, buka port `5173` untuk web dan `8080` untuk API. Web memakai proxy Vite ke API.
4. Cek API: `GET /api/health`.

## Database
PostgreSQL berjalan dalam service Docker `db`. Schema awal otomatis diterapkan dari `backend/migrations/001_init.sql` saat container pertama kali dibuat. Data lokal tersimpan pada volume `postgres_data`.

## Web / Android
- Web/PWA: frontend React + Vite; bisa dibuka dari browser HP dan dipasang sebagai PWA setelah konfigurasi manifest/service worker dilengkapi.
- Android: proyek Capacitor ada di `frontend/android`. Instal Node.js, Android Studio, dan Android SDK, lalu jalankan dari folder `frontend`:
  ```bash
  npm install
  npm run android:sync
  npm run android:open
  ```
  Di Android Studio, pilih **Build → Build Bundle(s) / APK(s) → Build APK(s)**. Petugas PAM dan warga memakai login serta halaman masing-masing yang sudah ada di web.
- Saat aplikasi Android pertama kali dibuka, isi URL API yang dapat dijangkau ponsel (mis. `https://rt.example.id`). Jangan gunakan `localhost` kecuali server berjalan pada ponsel itu sendiri. URL dapat diubah lewat **Koneksi server**.
- Untuk pengembangan lokal di jaringan Wi-Fi yang sama, gunakan IP komputer dan port API, mis. `http://192.168.1.20:8080`; HTTP hanya diizinkan pada build debug Android. Build release sebaiknya memakai HTTPS. Pastikan firewall/server mengizinkan koneksi dari ponsel.
- Backend mendukung token Bearer untuk sesi aplikasi native. Deploy ulang backend setelah perubahan; CORS mengizinkan metode API yang dibutuhkan aplikasi.
- Membuat APK memerlukan Java/Android SDK. Codespaces dapat menyiapkan proyek, tetapi build APK sebaiknya dilakukan dari Android Studio/Android SDK.

## Catatan awal
- Ini starter/MVP, bukan sistem produksi siap pakai. Sebelum digunakan untuk uang nyata, lengkapi login dan otorisasi, audit trail, backup, HTTPS, pengujian transaksi, dan kebijakan privasi.
- Kumpulan libur tidak menghasilkan tagihan baru. Tunggakan lama tetap tersimpan dan boleh dicicil atau dilunasi.
- Tagihan periode dibuat hanya saat status kumpulan `TERLAKSANA`.
- Jangan memasukkan NIK atau data sensitif asli ke demo.

## Fitur yang sudah ada di starter
- Dashboard glossy hijau, responsif desktop dan HP.
- CRUD tambah warga/rumah dan QR token.
- Buat periode kumpulan berstatus TERJADWAL / TERLAKSANA / LIBUR.
- Generate jadwal Ahad Pon otomatis tiap 35 hari dari tanggal jangkar yang sudah ditentukan admin, termasuk jam kumpulan.
- Pembuatan tagihan ditolak jika kumpulan bukan TERLAKSANA.
- Pembayaran/cicilan individual dialokasikan ke tagihan tertua.
- Pencatatan jimpitan memakai QR token, tanggal, nominal, dan hasil kunjungan.
- Serah terima jimpitan per malam dengan dua koordinator dan jumlah terhitung dari jimpitan berhasil diambil.
- Jadwal petugas ronda Senin–Minggu, dengan beberapa petugas dalam satu hari.
- Ekspor riwayat serah terima sebagai CSV yang bisa dibuka di Excel serta cetak/simpan sebagai PDF.
- Pencatatan hadir/tidak hadir petugas ronda dan riwayat perubahan jadwal.
- Akun warga dengan dashboard pribadi untuk melihat tagihan kas, tabungan jalan, jadwal ronda, pembayaran, jimpitan, dan jadwal kumpulan.
- Warga dapat mengirim angka meter PAM untuk diverifikasi petugas; pengurus mengatur kanal pembayaran (mis. petugas mengambil, transfer bank, QRIS) beserta instruksi, lalu warga memilih cara bayar pada tagihan. Pelunasan tetap dicatat petugas.
- Pengaturan PAM dapat menyimpan gambar QRIS dan nomor rekening; warga dapat mengirim bukti transfer/QRIS untuk ditinjau. Petugas PAM mengelola verifikasi meter, bukti pembayaran, penerimaan kolektif, riwayat transaksi, dan komisi per tagihan lunas sesuai persentase yang diatur.
- Buat akun tersebut di **Manajemen pengguna** dengan jabatan **Warga**, tautkan rumah/KK, lalu berikan username dan password kepada warga.
- Input transaksi tabungan jalan dan ringkasan saldo peserta.

## Batasan MVP yang harus diketahui
- Nominal iuran Rp25.000 masih contoh hard-coded di endpoint pembuatan tagihan; jadikan konfigurasi sebelum dipakai.
- Pembayaran kolektif multi-KK, kamera QR langsung, penarikan tabungan dengan validasi saldo, dan laporan kas lengkap belum selesai.
- Service worker menyimpan shell aplikasi sederhana; ini bukan jaminan seluruh aplikasi berfungsi offline. Transaksi tetap memerlukan koneksi.
- Tanggal jangkar Ahad Pon harus dipastikan admin berdasarkan pasaran yang benar; generator meneruskan pola 35 hari dari tanggal tersebut dan tidak menebak pasaran.
