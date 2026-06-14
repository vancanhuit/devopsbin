<script lang="ts">
  import { auth } from './auth.svelte'

  // mode toggles the inline form between logging in and registering. It is only
  // shown while logged out.
  let mode = $state<'login' | 'register'>('login')
  let username = $state('')
  let password = $state('')
  let busy = $state(false)
  let error = $state<string | null>(null)

  const roleBadge = $derived(auth.user?.role === 'admin' ? 'admin' : 'user')

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    if (busy) return
    busy = true
    error = null
    try {
      const ok =
        mode === 'login'
          ? await auth.login(username, password)
          : await auth.register(username, password)
      if (ok) {
        username = ''
        password = ''
      } else {
        error = mode === 'login' ? 'Invalid username or password.' : 'Could not register that user.'
      }
    } finally {
      busy = false
    }
  }

  async function signOut() {
    if (busy) return
    busy = true
    error = null
    try {
      await auth.logout()
    } finally {
      busy = false
    }
  }
</script>

<section
  class="flex flex-col gap-3 rounded-xl border border-slate-800 bg-slate-900/60 p-4 shadow-lg sm:flex-row sm:items-center sm:justify-between"
>
  {#if !auth.ready}
    <p class="text-sm text-slate-500">Checking session…</p>
  {:else if auth.authenticated}
    <div class="flex items-center gap-2 text-sm">
      <span class="text-slate-400">Signed in as</span>
      <span class="font-semibold text-slate-100">{auth.user?.username}</span>
      <span
        class="rounded-full bg-sky-500/15 px-2 py-0.5 text-xs font-medium text-sky-300 ring-1 ring-inset ring-sky-500/30"
      >
        {roleBadge}
      </span>
    </div>
    <button
      type="button"
      onclick={signOut}
      disabled={busy}
      class="shrink-0 rounded-lg border border-slate-700 px-3.5 py-2 text-sm font-medium text-slate-200 transition hover:bg-slate-800 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {busy ? 'Signing out…' : 'Sign out'}
    </button>
  {:else}
    <div class="flex items-center gap-2 text-sm text-slate-400">
      <span class="h-2 w-2 rounded-full bg-slate-600"></span>
      <span>Logged out</span>
    </div>
    <form class="flex flex-wrap items-end gap-2" onsubmit={submit}>
      <label class="flex flex-col gap-1 text-xs">
        <span class="font-medium text-slate-400">Username</span>
        <input
          type="text"
          bind:value={username}
          autocomplete="username"
          disabled={busy}
          class="w-36 rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 text-sm text-slate-200 focus:border-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:opacity-50"
        />
      </label>
      <label class="flex flex-col gap-1 text-xs">
        <span class="font-medium text-slate-400">Password</span>
        <input
          type="password"
          bind:value={password}
          autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
          disabled={busy}
          class="w-36 rounded-lg border border-slate-700 bg-slate-950/80 px-2.5 py-1.5 text-sm text-slate-200 focus:border-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:opacity-50"
        />
      </label>
      <button
        type="submit"
        disabled={busy}
        class="rounded-lg bg-sky-600 px-3.5 py-2 text-sm font-medium text-white transition hover:bg-sky-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-sky-400 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Working…' : mode === 'login' ? 'Sign in' : 'Register'}
      </button>
      <button
        type="button"
        onclick={() => {
          mode = mode === 'login' ? 'register' : 'login'
          error = null
        }}
        disabled={busy}
        class="text-xs text-sky-400 underline-offset-2 hover:underline disabled:opacity-50"
      >
        {mode === 'login' ? 'Need an account?' : 'Have an account?'}
      </button>
    </form>
  {/if}
</section>

{#if error}
  <p class="mt-2 text-sm text-rose-300">{error}</p>
{/if}
