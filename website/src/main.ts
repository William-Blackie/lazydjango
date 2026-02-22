import './style.css'

const installCommands = [
  'brew tap William-Blackie/lazydjango https://github.com/William-Blackie/lazydjango',
  'brew install --cask lazy-django',
  'lazy-django --version',
]

const app = document.querySelector<HTMLDivElement>('#app')

if (!app) {
  throw new Error('App container not found')
}

app.innerHTML = `
  <header class="glass mb-10 rounded-xl2 p-5 sm:p-7">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p class="font-mono text-xs uppercase tracking-[0.22em] text-mint">LazyDjango</p>
        <h1 class="mt-2 text-3xl font-bold leading-tight text-white sm:text-4xl">Django workflow speed, from one TUI.</h1>
      </div>
      <div class="flex items-center gap-3">
        <a class="rounded-lg border border-leaf/70 bg-leaf/20 px-4 py-2 text-sm font-semibold text-white transition hover:bg-leaf/30" href="https://github.com/William-Blackie/lazydjango" target="_blank" rel="noreferrer">GitHub</a>
        <a class="rounded-lg border border-amber/70 bg-amber/20 px-4 py-2 text-sm font-semibold text-white transition hover:bg-amber/30" href="#install">Install</a>
      </div>
    </div>
    <p class="mt-4 max-w-3xl text-sm leading-relaxed text-slate-200 sm:text-base">LazyDjango is a keyboard-first command center inspired by LazyVim and LazyGit. It helps teams run server tasks, container workflows, migrations, data operations, and diagnostics without bouncing between shell tabs and scripts.</p>
  </header>

  <main class="space-y-10">
    <section class="grid gap-6 lg:grid-cols-2">
      <article class="glass rounded-xl2 p-5">
        <h2 class="text-xl font-semibold text-white">What It Is</h2>
        <p class="mt-3 leading-relaxed text-slate-200">A terminal UI focused on Django day-to-day work: run commands, inspect output, manage containers, and handle data snapshots with consistent Vim-style navigation.</p>
        <ul class="mt-4 space-y-2 text-sm text-slate-200">
          <li><span class="kbd">j/k</span> move</li>
          <li><span class="kbd">g/G</span> top and bottom jump</li>
          <li><span class="kbd">Ctrl+d/u</span> page movement</li>
          <li><span class="kbd">:</span> command bar and <span class="kbd">/</span> search</li>
        </ul>
      </article>

      <article class="glass rounded-xl2 p-5">
        <h2 class="text-xl font-semibold text-white">Why It Works</h2>
        <ol class="step-list mt-3 list-decimal space-y-3 pl-5 text-sm leading-relaxed text-slate-200">
          <li><strong class="text-white">One control surface:</strong> project actions, DB work, snapshots, logs, and command history are all in one place.</li>
          <li><strong class="text-white">Project memory:</strong> task lists and history are persisted per project to reduce repetitive setup.</li>
          <li><strong class="text-white">Operational safety:</strong> dependency checks, clear output tabs, and direct task execution reduce context-switching mistakes.</li>
        </ol>
      </article>
    </section>

    <section class="space-y-5">
      <h2 class="text-2xl font-semibold text-white">LazyDjango In Action</h2>
      <div class="grid gap-6 lg:grid-cols-2">
        <figure class="glass overflow-hidden rounded-xl2 p-3">
          <img class="h-auto w-full rounded-lg border border-white/10" src="./images/lazydjango-overview.svg" alt="LazyDjango project overview with command output and model panels" loading="lazy" />
          <figcaption class="px-1 pt-3 text-sm text-slate-300">Project, database, data, and output panels tied together for the whole workflow.</figcaption>
        </figure>
        <figure class="glass overflow-hidden rounded-xl2 p-3">
          <img class="h-auto w-full rounded-lg border border-white/10" src="./images/lazydjango-output-tabs.svg" alt="LazyDjango output tabs showing command and logs streams" loading="lazy" />
          <figcaption class="px-1 pt-3 text-sm text-slate-300">Output tabs keep long-running logs separate from one-off commands.</figcaption>
        </figure>
      </div>
    </section>

    <section id="install" class="glass rounded-xl2 p-5 sm:p-6">
      <h2 class="text-2xl font-semibold text-white">Install</h2>
      <p class="mt-2 text-sm text-slate-200">macOS (Homebrew tap):</p>
      <div class="code-block mt-4 text-slate-100">${installCommands.join('\n')}</div>
      <p class="mt-4 text-sm text-slate-300">Need the latest release notes and binaries? <a class="text-mint underline decoration-mint/60 underline-offset-2" href="https://github.com/William-Blackie/lazydjango/releases" target="_blank" rel="noreferrer">View releases</a>.</p>
    </section>
  </main>

  <footer class="mt-10 border-t border-white/15 py-4 text-center text-xs text-slate-400">
    <p>LazyDjango • keyboard-first Django operations</p>
  </footer>
`
