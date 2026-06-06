<script lang="ts">
  import EndpointCard from './lib/EndpointCard.svelte'

  interface Endpoint {
    method: string
    path: string
    title: string
    description: string
    expectedStatuses: number[]
  }

  const endpoints: Endpoint[] = [
    {
      method: 'GET',
      path: '/livez',
      title: 'Liveness',
      description: 'Process-only liveness check. Always 200 while the process runs.',
      expectedStatuses: [200],
    },
    {
      method: 'GET',
      path: '/readyz',
      title: 'Readiness',
      description: 'Reports whether the service is ready to receive traffic (503 when not).',
      expectedStatuses: [200, 503],
    },
    {
      method: 'GET',
      path: '/startupz',
      title: 'Startup',
      description: 'Reports whether startup has completed (503 while still starting).',
      expectedStatuses: [200, 503],
    },
    {
      method: 'GET',
      path: '/version',
      title: 'Version',
      description: 'Build and version metadata for the running binary.',
      expectedStatuses: [200],
    },
  ]
</script>

<div class="min-h-screen bg-slate-950 text-slate-100">
  <div class="mx-auto max-w-4xl px-6 py-12">
    <header class="mb-10">
      <div class="flex items-center gap-3">
        <span
          class="flex h-10 w-10 items-center justify-center rounded-lg bg-sky-600 text-lg font-bold"
        >
          ⚙
        </span>
        <div>
          <h1 class="text-2xl font-bold tracking-tight">DevOpsBin Console</h1>
          <p class="text-sm text-slate-400">
            Trigger the backend runtime endpoints and inspect their responses.
          </p>
        </div>
      </div>
    </header>

    <main class="grid gap-5 sm:grid-cols-2">
      {#each endpoints as ep (ep.path)}
        <EndpointCard
          method={ep.method}
          path={ep.path}
          title={ep.title}
          description={ep.description}
          expectedStatuses={ep.expectedStatuses}
        />
      {/each}
    </main>

    <footer class="mt-12 border-t border-slate-800 pt-6 text-xs text-slate-500">
      Requests are proxied to the API base
      <code class="font-mono text-slate-400">/api/v1</code>. Configure the backend origin via
      <code class="font-mono text-slate-400">VITE_API_PROXY_TARGET</code>.
    </footer>
  </div>
</div>
