import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    vue(),
    VitePWA({
      registerType: 'autoUpdate',
      injectRegister: 'auto',
      includeAssets: ['apple-touch-icon.png', 'airy-logo.png'],
      manifest: {
        id: '/',
        name: 'Airy 私人影像空间',
        short_name: 'Airy',
        description: '浏览和管理自己的照片与视频',
        lang: 'zh-CN',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#f7f5f2',
        theme_color: '#efebe7',
        icons: [
          {
            src: '/pwa-192x192.png',
            sizes: '192x192',
            type: 'image/png',
            purpose: 'any',
          },
          {
            src: '/pwa-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'any',
          },
          {
            src: '/pwa-maskable-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,woff2,png}'],
        cleanupOutdatedCaches: true,
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//],
        runtimeCaching: [
          {
            urlPattern: ({ url, sameOrigin }) => sameOrigin && (
              url.pathname === '/api/favorites/photos'
              || (url.pathname === '/api/gallery' && url.searchParams.get('favorites') === 'true')
            ),
            handler: 'NetworkFirst',
            options: {
              cacheName: 'airy-favorites-api-v1',
              cacheableResponse: { statuses: [200] },
              expiration: { maxEntries: 100, maxAgeSeconds: 7 * 24 * 60 * 60, purgeOnQuotaError: true },
            },
          },
          {
            urlPattern: ({ url, sameOrigin }) => sameOrigin && /^\/api\/media\/\d+\/thumbnail$/.test(url.pathname)
              && url.searchParams.get('favorite_cache') === '1',
            handler: 'CacheFirst',
            options: {
              cacheName: 'airy-favorites-thumbnails-v1',
              cacheableResponse: { statuses: [200] },
              expiration: { maxEntries: 1000, maxAgeSeconds: 30 * 24 * 60 * 60, purgeOnQuotaError: true },
            },
          },
          {
            urlPattern: ({ url, sameOrigin }) => sameOrigin && /^\/api\/media\/\d+\/original$/.test(url.pathname)
              && url.searchParams.get('favorite_cache') === '1',
            handler: 'CacheFirst',
            options: {
              cacheName: 'airy-favorites-images-v1',
              cacheableResponse: { statuses: [200] },
              expiration: { maxEntries: 128, maxAgeSeconds: 7 * 24 * 60 * 60, purgeOnQuotaError: true },
            },
          },
          {
            urlPattern: /\/api\//,
            handler: 'NetworkOnly',
          },
        ],
      },
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8096',
        changeOrigin: true,
      },
    },
  },
})

