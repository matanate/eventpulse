import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      globals: true,
      setupFiles: ['./src/test/setup.ts'],
      env: {
        VITE_API_BASE_URL: 'http://test.local',
        VITE_DEMO_PROJECT_ID: 'test-project-id',
        VITE_DEMO_API_KEY: 'epk_test',
      },
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html'],
        include: ['src/**/*.{ts,tsx}'],
        exclude: ['src/test/**', 'src/main.tsx'],
      },
    },
  }),
)
