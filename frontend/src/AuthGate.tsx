import { FormEvent, useEffect, useState } from 'react';
import { Capacitor } from '@capacitor/core';
import { Home, LogIn, ShieldCheck } from 'lucide-react';
import App from './App';
import PublicHome from './PublicHome';
import ResidentDashboard from './ResidentDashboard';
import { apiFetch as fetch, clearSessionToken, getAPIBaseURL, saveSessionToken, setAPIBaseURL } from './api';
import './auth.css';

type AuthUser = {
  id: number;
  username: string;
  full_name: string;
  position_id: number;
  field_id: number;
  position: string;
  resident_id: number;
  is_resident: boolean;
  permissions: Record<string, boolean>;
};

type AuthMode = 'loading' | 'configure' | 'public' | 'bootstrap' | 'login' | 'authenticated';
type LoginAudience = 'warga' | 'pengurus';

export default function AuthGate() {
  const [mode, setMode] = useState<AuthMode>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);
  const [username, setUsername] = useState('');
  const [fullName, setFullName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [bootstrapRequired, setBootstrapRequired] = useState(false);
  const [loginAudience, setLoginAudience] = useState<LoginAudience>('warga');
  const [apiBaseURL, setApiBaseURLInput] = useState(getAPIBaseURL());
  const [connectionError, setConnectionError] = useState('');

  const loadUser = async () => {
    const response = await fetch('/api/auth/me');
    if (!response.ok) throw new Error('login diperlukan');
    return await response.json() as AuthUser;
  };

  useEffect(() => {
    const initialize = async () => {
      if (Capacitor.isNativePlatform() && !getAPIBaseURL()) {
        setMode('configure');
        return;
      }
      try {
        const statusResponse = await fetch('/api/auth/bootstrap-status');
        if (!statusResponse.ok) throw new Error('Tidak dapat memeriksa status akun');
        const status = await statusResponse.json() as { required: boolean };
        setBootstrapRequired(status.required);
        try {
          setUser(await loadUser());
          setMode('authenticated');
        } catch (initializeError) {
          setConnectionError(initializeError instanceof Error ? initializeError.message : 'Tidak dapat terhubung ke server.');
          setMode('public');
        }
      } catch {
        setMode('public');
      }
    };
    void initialize();
  }, []);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const bootstrap = mode === 'bootstrap';
      const response = await fetch(bootstrap ? '/api/auth/bootstrap' : '/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(bootstrap ? { username, full_name: fullName, password } : { username, password }),
      });
      const result = await response.json() as { error?: string; token?: string };
      if (!response.ok) throw new Error(result.error || 'Permintaan gagal');
      if (result.token) saveSessionToken(result.token);
      setUser(await loadUser());
      setMode('authenticated');
      setPassword('');
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : 'Terjadi kesalahan');
    } finally {
      setBusy(false);
    }
  };

  const logout = async () => {
    await fetch('/api/auth/logout', { method: 'POST' });
    clearSessionToken();
    setUser(null);
    setMode('public');
  };

  const saveAPIConfiguration = (event: FormEvent) => {
    event.preventDefault();
    try {
      setAPIBaseURL(apiBaseURL);
      clearSessionToken();
      window.location.reload();
    } catch (configurationError) {
      setConnectionError(configurationError instanceof Error ? configurationError.message : 'Alamat server tidak valid.');
    }
  };

  if (mode === 'configure') return <main className="auth-screen"><section className="auth-panel">
    <div className="auth-mark"><Home size={23}/></div>
    <p className="auth-eyebrow">WIDOROKU · KONEKSI APLIKASI</p>
    <h1>Hubungkan ke server RT</h1>
    <p className="auth-description">Masukkan alamat server Widoroku yang dapat dijangkau dari ponsel. Jangan gunakan localhost kecuali API berjalan langsung di perangkat ini.</p>
    {connectionError&&<div className="auth-error" role="alert">{connectionError}</div>}
    <form className="form" onSubmit={saveAPIConfiguration}>
      <label>Alamat server<input type="url" required placeholder="https://rt.example.id" value={apiBaseURL} onChange={event=>setApiBaseURLInput(event.target.value)}/></label>
      <p className="hint">Untuk pengembangan, gunakan alamat IP komputer di jaringan Wi-Fi yang sama, misalnya http://192.168.1.20:8080. Untuk penggunaan nyata gunakan HTTPS.</p>
      <button className="primary-btn">Simpan dan sambungkan</button>
    </form>
  </section></main>;
  if (mode === 'loading') return <main className="auth-screen"><p>Memeriksa sesi...</p></main>;
  if (mode === 'authenticated' && user?.is_resident) return <ResidentDashboard onLogout={logout} onConfigureAPI={()=>setMode('configure')} />;
  if (mode === 'authenticated' && user) return <App user={user} onLogout={logout} onConfigureAPI={()=>setMode('configure')} />;
  if (mode === 'public') return <><PublicHome
    onResidentLogin={() => { setLoginAudience('warga'); setMode('login'); }}
    onAdminLogin={() => { setLoginAudience('pengurus'); setMode(bootstrapRequired ? 'bootstrap' : 'login'); }}
  />{connectionError&&<p className="auth-connection-error" role="alert">{connectionError}</p>}<button className="auth-server-settings" onClick={()=>setMode('configure')}>Pengaturan server</button></>;

  const bootstrap = mode === 'bootstrap';
  return <main className="auth-screen"><section className="auth-panel">
    <div className="auth-mark">{bootstrap ? <Home size={23} /> : <ShieldCheck size={23} />}</div>
    <button type="button" className="auth-back" onClick={() => setMode('public')}>← Kembali ke beranda</button>
    <button type="button" className="auth-back" onClick={()=>setMode('configure')}>Ubah alamat server</button>
    <p className="auth-eyebrow">WIDOROKU · PORTAL {bootstrap ? 'PENGURUS' : loginAudience.toUpperCase()}</p>
    <h1>{bootstrap ? 'Buat akun administrator' : loginAudience === 'warga' ? 'Masuk sebagai warga' : 'Masuk sebagai pengurus'}</h1>
    <p className="auth-description">{bootstrap ? 'Akun ini hanya dibuat sekali sebagai pengelola awal.' : loginAudience === 'warga' ? 'Gunakan username dan password akun warga yang diberikan pengurus RT.' : 'Gunakan akun pengurus yang telah diberikan administrator.'}</p>
    {error && <div className="auth-error" role="alert">{error}</div>}
    <form className="form" onSubmit={submit}>
      <label>Username<input autoComplete="username" required value={username} onChange={event => setUsername(event.target.value)} /></label>
      {bootstrap && <label>Nama lengkap<input autoComplete="name" required value={fullName} onChange={event => setFullName(event.target.value)} /></label>}
      <label>Password<input autoComplete={bootstrap ? 'new-password' : 'current-password'} type="password" minLength={bootstrap ? 12 : undefined} required value={password} onChange={event => setPassword(event.target.value)} /></label>
      {bootstrap && <p className="hint">Password minimal 12 karakter. Tidak ada password default.</p>}
      <button className="primary-btn" disabled={busy}>{bootstrap ? <ShieldCheck size={16} /> : <LogIn size={16} />}{busy ? 'Memproses...' : bootstrap ? 'Buat administrator' : 'Masuk'}</button>
    </form>
  </section></main>;
}
