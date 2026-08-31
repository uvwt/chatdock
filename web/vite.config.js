import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [tailwindcss(), react()],
  build: {
    // Base UI 与 Floating UI 是稳定的第三方交互基础层，单独成块可避免菜单能力继续挤压主应用包，
    // 同时让后续 Button/Dialog/Menu 复用同一份可缓存 vendor chunk。
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [{
            name: 'ui-vendor',
            test: /node_modules\/(?:@base-ui|@floating-ui)\//,
          }],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8720',
      '/mcp': 'http://127.0.0.1:8720',
    },
  },
});
