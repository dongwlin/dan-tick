import js from '@eslint/js'
import { withVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import { globalIgnores } from 'eslint/config'
import prettier from 'eslint-config-prettier/flat'
import vue from 'eslint-plugin-vue'
import accessibility from 'eslint-plugin-vuejs-accessibility'
import globals from 'globals'

// The Vue helper handles SFC parsing and component type false positives.
// vue-tsc remains responsible for checking component contracts.
export default withVueTs(
  { rootDir: import.meta.dirname },
  globalIgnores(['**/node_modules/**', '**/dist/**', '**/bindings/**', '**/coverage/**']),
  {
    files: ['**/*.{js,mjs,cjs,ts,tsx,vue}'],
    extends: [js.configs.recommended],
    linterOptions: { reportUnusedDisableDirectives: 'error' },
    rules: {
      'no-duplicate-imports': ['error', { allowSeparateTypeImports: true }],
    },
  },
  {
    files: ['src/**/*.{ts,tsx,vue}', 'vite.config.ts'],
    extends: [vueTsConfigs.recommendedTypeChecked],
    languageOptions: {
      parserOptions: {
        // Use both existing TS projects, including the separate Vite config.
        projectService: false,
        project: ['./tsconfig.json', './tsconfig.node.json'],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      '@typescript-eslint/consistent-type-imports': 'error',
      '@typescript-eslint/no-non-null-assertion': 'error',
    },
  },
  {
    files: ['src/**/*.vue'],
    extends: [vue.configs['flat/recommended'], accessibility.configs['flat/recommended']],
  },
  {
    files: ['src/**/*.{ts,tsx,vue}'],
    languageOptions: { globals: globals.browser },
  },
  {
    files: ['vite.config.ts', '*.{js,mjs,cjs}'],
    languageOptions: { globals: globals.node },
  },
  // Formatting belongs to Prettier; keep this last to disable conflicting rules.
  prettier,
)
