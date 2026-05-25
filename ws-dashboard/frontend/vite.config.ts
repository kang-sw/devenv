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
          if (id.includes('/node_modules/@codemirror/lang-markdown/')) {
            return 'codemirror-lang-markdown';
          }
          if (id.includes('/node_modules/@codemirror/lang-javascript/')) {
            return 'codemirror-lang-javascript';
          }
          if (id.includes('/node_modules/@codemirror/lang-json/')) {
            return 'codemirror-lang-json';
          }
          if (id.includes('/node_modules/@codemirror/lang-css/')) {
            return 'codemirror-lang-css';
          }
          if (id.includes('/node_modules/@codemirror/lang-html/')) {
            return 'codemirror-lang-html';
          }
          if (id.includes('/node_modules/@codemirror/lang-yaml/')) {
            return 'codemirror-lang-yaml';
          }
          if (id.includes('/node_modules/@codemirror/lang-python/')) {
            return 'codemirror-lang-python';
          }
          if (id.includes('/node_modules/@codemirror/lang-rust/')) {
            return 'codemirror-lang-rust';
          }
          if (id.includes('/node_modules/@codemirror/lang-xml/')) {
            return 'codemirror-lang-xml';
          }
          if (id.includes('/node_modules/@codemirror/lang-sql/')) {
            return 'codemirror-lang-sql';
          }
          if (id.includes('/node_modules/@codemirror/lang-go/')) {
            return 'codemirror-lang-go';
          }
          if (id.includes('/node_modules/@codemirror/lang-java/')) {
            return 'codemirror-lang-java';
          }
          if (id.includes('/node_modules/@codemirror/lang-cpp/')) {
            return 'codemirror-lang-cpp';
          }
          if (id.includes('/node_modules/@codemirror/lang-php/')) {
            return 'codemirror-lang-php';
          }
          if (id.includes('/node_modules/@codemirror/legacy-modes/')) {
            return 'codemirror-legacy-modes';
          }
          if (id.includes('/node_modules/@codemirror/')) {
            return 'codemirror-core';
          }
          if (id.includes('/node_modules/@lezer/')) {
            return 'codemirror-parser';
          }
          if (
            id.includes('/node_modules/style-mod/') ||
            id.includes('/node_modules/w3c-keyname/') ||
            id.includes('/node_modules/crelt/')
          ) {
            return 'codemirror-support';
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
