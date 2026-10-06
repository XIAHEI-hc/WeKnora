import { fileURLToPath, URL } from 'node:url'
import { dirname, resolve } from 'node:path'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const currentDirectory = dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  base: '/weknora-workbench/',
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist/weknora-workbench',
    emptyOutDir: true,
    rollupOptions: {
      input: resolve(currentDirectory, 'workbench.html'),
    },
  },
})
