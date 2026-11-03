// Lint tests with the same rules as web/, typed against tests/tsconfig.json.
import webConfig from '../web/eslint.config.js'

export default [
  ...webConfig,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      parserOptions: {
        projectService: false,
        project: ['./tsconfig.json'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
]
