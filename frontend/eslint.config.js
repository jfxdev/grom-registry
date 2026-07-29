import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist/**', 'src/shared/api/generated/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  {
    files: ['**/*.{ts,tsx,vue}'],
    languageOptions: {
      globals: {
        console: 'readonly',
        document: 'readonly',
        fetch: 'readonly',
        navigator: 'readonly',
        window: 'readonly',
      },
      parserOptions: {
        parser: tseslint.parser,
      },
    },
    rules: {
      'vue/multi-word-component-names': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/first-attribute-linebreak': 'off',
      'vue/html-closing-bracket-newline': 'off',
      'vue/html-self-closing': ['error', {
        html: { void: 'always', normal: 'always', component: 'always' },
        svg: 'always',
        math: 'always',
      }],
      '@typescript-eslint/no-explicit-any': 'error',
    },
  },
)
