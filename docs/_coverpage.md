<div class="lr-hero">
  <span class="hero-badge">Open source · Zero dependencies</span>
  <h1 class="hero-title">One API for every LLM,<br /><span class="gradient-text">built for Go.</span></h1>
  <p class="hero-sub">24 providers behind a single interface — with typed tools, structured output, agents, stateful graphs, MCP and A2A. Only the standard library.</p>
  <div class="hero-actions">
    <a href="#/getting-started" class="btn btn-primary">Get Started</a>
    <a href="https://github.com/promptrails/langrails" class="btn btn-secondary" target="_blank">GitHub</a>
  </div>
  <div class="install-cmd">
    <span class="dollar">$</span>
    <span>go get github.com/promptrails/langrails</span>
    <button onclick="navigator.clipboard.writeText('go get github.com/promptrails/langrails')" title="Copy" aria-label="Copy install command">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
    </button>
  </div>
  <div class="features-grid">
    <a class="feature-card" href="#/providers">
      <h3>24 providers</h3>
      <p>OpenAI, Anthropic, Gemini, Bedrock, Groq, Mistral, Ollama and more — switch with one constant or a <code>provider:model</code> string.</p>
    </a>
    <a class="feature-card" href="#/tool-calling">
      <h3>Typed tools &amp; output</h3>
      <p>Tools from Go functions and <code>Generate[T]</code> structured output, with schemas derived from your structs.</p>
    </a>
    <a class="feature-card" href="#/agents">
      <h3>Agents</h3>
      <p>A middleware-driven tool loop with streaming, call limits, fallback, context editing and human-in-the-loop.</p>
    </a>
    <a class="feature-card" href="#/graph">
      <h3>Graphs</h3>
      <p>LangGraph-style workflows: fan-out, subgraphs, checkpoints, resume and durable interrupts.</p>
    </a>
    <a class="feature-card" href="#/mcp">
      <h3>MCP &amp; A2A</h3>
      <p>Use any MCP server over HTTP or stdio, and talk to other agents with the A2A protocol.</p>
    </a>
    <a class="feature-card" href="#/observability">
      <h3>Production-ready</h3>
      <p>Retry with <code>Retry-After</code>, rate limiting, response caching and tracing hooks as composable decorators.</p>
    </a>
  </div>
</div>
