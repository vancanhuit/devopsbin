<script lang="ts">
  import { untrack } from 'svelte'
  import { call, type CallResult, type EndpointParam } from './api'

  interface Props {
    method?: string
    path: string
    title: string
    description: string
    // Status codes the endpoint may return that should still be treated as a
    // successful call (e.g. readyz returns 503 when not ready).
    expectedStatuses?: number[]
    // Inputs to collect before calling a parameterized endpoint (e.g. the
    // {code} segment of /status/{code}). Empty for endpoints with no input.
    params?: EndpointParam[]
    // HTTP methods the endpoint supports when more than one is available (e.g.
    // /echo). When set, the card shows a method picker; the body input appears
    // for methods that carry a request body.
    methods?: string[]
  }

  let {
    method = 'GET',
    path,
    title,
    description,
    expectedStatuses = [200],
    params = [],
    methods,
  }: Props = $props()

  let loading = $state(false)
  let result = $state<CallResult | null>(null)

  // selectedMethod tracks the chosen HTTP method for multi-method endpoints,
  // seeded once with the first option. Single-method cards keep `method`.
  let selectedMethod = $state<string>(untrack(() => methods?.[0] ?? method))

  // body holds the request body for methods that carry one. It is sent only
  // when the selected method accepts a body (see bodyAllowed).
  let body = $state('')

  // bodyAllowed is true when the selected method carries a request body, which
  // gates the body textarea and what is forwarded to the call.
  const bodyAllowed = $derived(['POST', 'PUT', 'PATCH', 'DELETE'].includes(selectedMethod))

  // displayMethod is the badge label: the selected method for multi-method
  // cards, otherwise the static method.
  const displayMethod = $derived(methods ? selectedMethod : method)

  // values holds the current input for each parameter, seeded once with its
  // default so the endpoint is callable without edits. params is static config
  // per card, so untrack makes the initial-only capture explicit.
  let values = $state<Record<string, string>>(
    untrack(() => Object.fromEntries(params.map((p) => [p.name, p.defaultValue])))
  )

  // displayPath substitutes the current input values into the path template so
  // the user sees the concrete URL they are about to call (e.g. /status/404).
  const displayPath = $derived(
    params.reduce((acc, p) => acc.replace(`{${p.name}}`, values[p.name] || `{${p.name}}`), path)
  )

  async function trigger() {
    loading = true
    try {
      result = await call(path, values, {
        method: selectedMethod,
        body: bodyAllowed ? body : undefined,
      })
    } finally {
      loading = false
    }
  }

  // A call "succeeded" (reached the server with an understood status) when the
  // status is one the endpoint is documented to return.
  const reached = $derived(result !== null && expectedStatuses.includes(result.status))

  function statusClasses(r: CallResult): string {
    if (r.error || r.status === 0) return 'bg-rose-500/15 text-rose-300 ring-rose-500/30'
    if (r.ok) return 'bg-emerald-500/15 text-emerald-300 ring-emerald-500/30'
    return 'bg-amber-500/15 text-amber-300 ring-amber-500/30'
  }

  function pretty(body: unknown): string {
    if (typeof body === 'string') return body
    return JSON.stringify(body, null, 2)
  }
</script>

<article
  class="flex flex-col gap-4 rounded-xl border border-slate-800 bg-slate-900/60 p-5 shadow-lg"
>
  <header class="flex items-start justify-between gap-4">
    <div>
      <div class="flex items-center gap-2">
        <span class="rounded bg-slate-800 px-2 py-0.5 font-mono text-xs font-semibold text-sky-300">
          {displayMethod}
        </span>
        <h2 class="text-base font-semibold text-slate-100">{title}</h2>
      </div>
      <p class="mt-1 font-mono text-xs text-slate-500">{displayPath}</p>
      <p class="mt-2 text-sm text-slate-400">{description}</p>
    </div>
    <button
      type="button"
      onclick={trigger}
      disabled={loading}
      class="shrink-0 rounded-lg bg-sky-600 px-3.5 py-2 text-sm font-medium text-white transition hover:bg-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {loading ? 'Running…' : 'Send'}
    </button>
  </header>

  {#if methods && methods.length > 1}
    <div class="flex flex-wrap items-end gap-3">
      <label class="flex flex-col gap-1 text-xs">
        <span class="font-medium text-slate-400">Method</span>
        <select
          bind:value={selectedMethod}
          disabled={loading}
          class="w-28 rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 font-mono text-sm text-slate-200 focus:border-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:opacity-50"
        >
          {#each methods as m (m)}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </label>
      {#if bodyAllowed}
        <label class="flex flex-1 flex-col gap-1 text-xs">
          <span class="font-medium text-slate-400">Body</span>
          <textarea
            bind:value={body}
            disabled={loading}
            rows="2"
            placeholder="Request body to echo back"
            class="min-w-40 rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 font-mono text-sm text-slate-200 focus:border-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:opacity-50"
          ></textarea>
        </label>
      {/if}
    </div>
  {/if}

  {#if params.length > 0}
    <div class="flex flex-wrap gap-3">
      {#each params as param (param.name)}
        <label class="flex flex-col gap-1 text-xs">
          <span class="font-medium text-slate-400">{param.label}</span>
          <input
            type={param.type === 'number' ? 'number' : 'text'}
            bind:value={values[param.name]}
            min={param.min}
            max={param.max}
            placeholder={param.placeholder}
            disabled={loading}
            class="w-28 rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 font-mono text-sm text-slate-200 focus:border-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:opacity-50"
          />
        </label>
      {/each}
    </div>
  {/if}

  {#if result}
    <div class="flex flex-col gap-2" role="status" aria-live="polite">
      <div class="flex flex-wrap items-center gap-2 text-xs">
        <span
          class="rounded-full px-2.5 py-0.5 font-mono font-semibold ring-1 ring-inset {statusClasses(
            result
          )}"
        >
          {result.error ? 'ERR' : result.status}
        </span>
        {#if reached}
          <span class="text-slate-500">documented response</span>
        {/if}
        <span class="ml-auto font-mono text-slate-500">{result.durationMs} ms</span>
      </div>
      <pre
        class="max-h-64 overflow-auto rounded-lg bg-slate-950/80 p-3 font-mono text-xs leading-relaxed text-slate-300 ring-1 ring-slate-800">{result.error ??
          pretty(result.body)}</pre>
    </div>
  {/if}
</article>
