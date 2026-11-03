// Test files live outside web/, so bare imports from
// tests/ are resolved as if imported by web/src to reuse web/node_modules.
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Plugin, ViteUserConfig } from 'vitest/config'

const testsDir = dirname(fileURLToPath(import.meta.url))
const repoRoot = resolve(testsDir, '..')
const webDir = resolve(repoRoot, 'web')
const webImporter = resolve(webDir, 'src/main.tsx')

function normalize(path: string): string {
  return path.replaceAll('\\', '/').toLowerCase()
}

function isBareImport(source: string): boolean {
  return !/^(?:[./\0]|[A-Za-z]:|node:|virtual:|@\/)/.test(source)
}

function resolveTestImportsFromWeb(): Plugin {
  const testsPrefix = `${normalize(testsDir)}/`
  return {
    name: 'vanillapay:tests-resolve-from-web',
    enforce: 'pre',
    resolveId(source, importer, options) {
      if (!importer || !isBareImport(source) || !normalize(importer).startsWith(testsPrefix)) {
        return null
      }
      return this.resolve(source, webImporter, { ...options, skipSelf: true })
    },
  }
}

export default {
  root: repoRoot,
  cacheDir: resolve(webDir, 'node_modules/.vite-tests'),
  plugins: [resolveTestImportsFromWeb()],
  resolve: {
    alias: {
      '@': resolve(webDir, 'src'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['tests/frontend/**/*.test.{ts,tsx}'],
    setupFiles: [resolve(testsDir, 'frontend/setup.ts')],
    restoreMocks: true,
  },
} satisfies ViteUserConfig
