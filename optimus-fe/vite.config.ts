import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src')
    }
  },
  server: {
    port: 5173,
    proxy: {
      '/api/v1': {
        target: 'http://localhost:8080',
        changeOrigin: false
      }
    }
  },
  build: {
    target: 'es2020',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia', 'pinia-plugin-persistedstate', 'axios'],
          antd:   ['ant-design-vue'],
          icons:  ['@ant-design/icons-vue'],
          utils:  ['dayjs', 'vue-i18n'],
          codemirror: ['vue-codemirror', 'codemirror', '@codemirror/lang-yaml', '@codemirror/theme-one-dark', '@codemirror/view', '@codemirror/state']
        }
      }
    }
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: []
  }
})
