import React from 'react'; import { createRoot } from 'react-dom/client'; import { Capacitor } from '@capacitor/core'; import AuthGate from './AuthGate'; import './style.css';
createRoot(document.getElementById('root')!).render(<React.StrictMode><AuthGate /></React.StrictMode>);

if ('serviceWorker' in navigator && import.meta.env.PROD && !Capacitor.isNativePlatform()) window.addEventListener('load', () => navigator.serviceWorker.register('/sw.js').catch(console.error));
