<script lang="ts">
  import { onMount } from 'svelte'
  import EndpointCard from './lib/EndpointCard.svelte'
  import Footer from './lib/Footer.svelte'
  import Session from './lib/Session.svelte'
  import { endpoints, getVersion, type Endpoint } from './lib/api'
  import { auth } from './lib/auth.svelte'

  let version: string | null = $state(null)

  // groups orders endpoints by their tag, preserving first-seen tag order, so
  // the console renders one labeled section per tag.
  const groups = (() => {
    const ordered: { tag: string; items: Endpoint[] }[] = []
    for (const ep of endpoints) {
      const bucket = ordered.find((g) => g.tag === ep.tag)
      if (bucket) bucket.items.push(ep)
      else ordered.push({ tag: ep.tag, items: [ep] })
    }
    return ordered
  })()

  onMount(async () => {
    void auth.hydrate()
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

    <div class="mb-10">
      <Session />
    </div>

    <main class="flex flex-col gap-10">
      {#each groups as group (group.tag)}
        <section>
          <h2 class="mb-4 text-sm font-semibold tracking-wide text-slate-400 uppercase">
            {group.tag}
          </h2>
          <div class="grid gap-5 sm:grid-cols-2">
            {#each group.items as ep (ep.path)}
              <EndpointCard
                method={ep.method}
                path={ep.path}
                title={ep.title}
                description={ep.description}
                expectedStatuses={ep.expectedStatuses}
                params={ep.params}
                bodyFields={ep.bodyFields}
                methods={ep.methods}
                supportsQuery={ep.supportsQuery}
                requiresAuth={ep.requiresAuth}
                requiresRole={ep.requiresRole}
                resultHeaders={ep.resultHeaders}
              />
            {/each}
          </div>
        </section>
      {/each}
    </main>
  </div>

  <Footer {version} />
</div>
