// Файл: web/vite.config.ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Сборка встраивается в Go-бинарник (web/embed.go); пути относительные.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: { outDir: 'dist', emptyOutDir: true },
})
