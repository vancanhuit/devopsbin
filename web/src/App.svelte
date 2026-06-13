<script lang="ts">
  import { onMount } from 'svelte'
  import EndpointCard from './lib/EndpointCard.svelte'
  import Footer from './lib/Footer.svelte'
  import { endpoints, getVersion } from './lib/api'

  let version: string | null = $state(null)

  onMount(async () => {
    try {
      const info = await getVersion()
      version = info.version
    } catch {
      // Version fetch is best-effort; don't surface failures to the user.
    }
  })
</script>

<div class="min-h-screen bg-slate-950 text-slate-100">
  <div class="mx-auto max-w-4xl px-6 pt-12 pb-24">
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
          params={ep.params}
          methods={ep.methods}
        />
      {/each}
    </main>
  </div>

  <Footer {version} />
</div>
