import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'id.widoroku.rt',
  appName: 'Widoroku RT',
  webDir: 'dist',
  server: {
    androidScheme: 'https',
  },
};

export default config;
