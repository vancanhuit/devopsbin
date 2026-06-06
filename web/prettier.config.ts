import type { Config } from 'prettier'

export default {
  plugins: ['prettier-plugin-svelte'],
  overrides: [{ files: '*.svelte', options: { parser: 'svelte' } }],
  printWidth: 100,
  semi: false,
  singleQuote: true,
  trailingComma: 'es5',
} satisfies Config
