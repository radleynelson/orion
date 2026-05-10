import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

declare const process: { env: Record<string, string | undefined> };

const browserPreview = process.env.ORION_BROWSER_PREVIEW === '1';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    __ORION_BROWSER_PREVIEW__: JSON.stringify(browserPreview),
  },
  resolve: {
    alias: browserPreview
      ? [
          {
            find: /^(\.\.\/)+wailsjs\/go\/main\/App$/,
            replacement: new URL('./src/dev/wails/App.ts', import.meta.url).pathname,
          },
          {
            find: /^(\.\.\/)+wailsjs\/runtime\/runtime$/,
            replacement: new URL('./src/dev/wails/runtime.ts', import.meta.url).pathname,
          },
        ]
      : [],
  },
})
