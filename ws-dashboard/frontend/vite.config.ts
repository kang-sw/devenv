import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('/node_modules/react-aria-components/')) {
            return 'react-aria';
          }
          if (id.includes('/node_modules/@xterm/')) {
            return 'xterm';
          }
          if (id.includes('/node_modules/dockview/')) {
            return 'dockview';
          }
          if (
            id.includes('/node_modules/react/') ||
            id.includes('/node_modules/react-dom/')
          ) {
            return 'react-core';
          }
          if (id.includes('/node_modules/')) {
            return 'vendor';
          }
        },
      },
    },
  },
  plugins: [react()],
});
