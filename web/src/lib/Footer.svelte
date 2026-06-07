<script lang="ts">
  interface Tech {
    name: string
    href: string
    // Full literal Tailwind classes (text + hover) so the JIT compiler can
    // see them; do not build these class names dynamically.
    color: string
  }

  interface Props {
    version?: string | null
  }

  const { version = null }: Props = $props()

  const stack: readonly Tech[] = [
    { name: 'Go', href: 'https://go.dev', color: 'text-cyan-400 hover:text-cyan-300' },
    {
      name: 'PostgreSQL',
      href: 'https://www.postgresql.org',
      color: 'text-sky-400 hover:text-sky-300',
    },
    { name: 'Redis', href: 'https://redis.io', color: 'text-red-400 hover:text-red-300' },
    { name: 'Bun', href: 'https://bun.sh', color: 'text-amber-300 hover:text-amber-200' },
    { name: 'Svelte', href: 'https://svelte.dev', color: 'text-orange-400 hover:text-orange-300' },
    { name: 'Vite', href: 'https://vite.dev', color: 'text-violet-400 hover:text-violet-300' },
    {
      name: 'Tailwind CSS',
      href: 'https://tailwindcss.com',
      color: 'text-teal-400 hover:text-teal-300',
    },
  ]
</script>

<footer class="fixed inset-x-0 bottom-0 border-t border-slate-800 bg-slate-950/80 backdrop-blur">
  <div
    class="mx-auto flex max-w-4xl flex-col gap-2 px-6 py-3 text-xs text-slate-500 sm:flex-row sm:items-center sm:justify-between"
  >
    <ul class="flex flex-wrap items-center gap-x-3 gap-y-1">
      <li class="text-slate-400">Built with</li>
      {#each stack as tech (tech.name)}
        <li>
          <a
            href={tech.href}
            target="_blank"
            rel="noopener noreferrer"
            class="font-medium underline-offset-4 transition-colors hover:underline {tech.color}"
            >{tech.name}</a
          >
        </li>
      {/each}
    </ul>
    {#if version}
      <span class="rounded bg-slate-800/60 px-2 py-0.5 font-mono font-medium text-emerald-400"
        >{version}</span
      >
    {/if}
  </div>
</footer>
