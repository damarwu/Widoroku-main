const API_BASE_KEY = 'widoroku_api_base_url';
const SESSION_TOKEN_KEY = 'widoroku_session_token';

const normalizeBaseURL = (value: string) => value.trim().replace(/\/+$/, '');

export function getAPIBaseURL() {
  const saved = localStorage.getItem(API_BASE_KEY);
  if (saved) return normalizeBaseURL(saved);
  return normalizeBaseURL(import.meta.env.VITE_API_BASE_URL || '');
}

export function setAPIBaseURL(value: string) {
  const baseURL = normalizeBaseURL(value);
  let parsed: URL;
  try {
    parsed = new URL(baseURL);
  } catch {
    throw new Error('Masukkan alamat server yang valid.');
  }
  if ((parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
    parsed.username || parsed.password || parsed.pathname !== '/' || parsed.search || parsed.hash) {
    throw new Error('Alamat server harus berupa URL http:// atau https:// tanpa path.');
  }
  localStorage.setItem(API_BASE_KEY, parsed.origin);
}

export function clearSessionToken() {
  localStorage.removeItem(SESSION_TOKEN_KEY);
}

export async function apiFetch(input: RequestInfo | URL, init?: RequestInit) {
  const requestURL = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
  const baseURL = getAPIBaseURL();
  const targetURL = baseURL && requestURL.startsWith('/api/')
    ? `${baseURL}${requestURL}`
    : requestURL;
  const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
  const token = localStorage.getItem(SESSION_TOKEN_KEY);
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const sameOrigin = new URL(targetURL, window.location.href).origin === window.location.origin;
  return fetch(targetURL, {
    ...init,
    headers,
    credentials: sameOrigin ? 'same-origin' : 'omit',
  });
}

export function saveSessionToken(token: string) {
  if (!token) throw new Error('Server tidak memberikan token sesi.');
  localStorage.setItem(SESSION_TOKEN_KEY, token);
}
