# LinkedIn post draft

EU teams want to use LLMs on customer tickets. The problem: every CPF, NIF and IBAN in those tickets ends up in a
provider's logs.

The usual fix is a sidecar that redacts before the call. Redaction can't be undone, so the answer comes back full
of placeholders. Tools that do restore values tend to buffer the whole streamed answer first, which kills the
streaming UX.

So I built maskwire, an open-source Go proxy for the OpenAI and Anthropic APIs.

Three decisions shaped it:

1️⃣ Restore inside the stream. A token like [EMAIL_FBDYD9] often arrives split across SSE events: "[EMA", then
"IL_FB", then "DYD9]". A small hand-written state machine holds back only bytes that could still become a token,
48 bytes at most, and restores the rest immediately. The tests cut streams at every split point and compare the
result with restoring the whole buffer.

2️⃣ Deterministic tokens. Tokens come from an HMAC with a per-tenant key, so the same prompt always reaches the
provider as the same masked text, and caches keep hitting.

3️⃣ Real checksums instead of loose regexes: CPF, the new alphanumeric CNPJ, PT NIF, ES DNI/NIE, DE Steuer-ID,
FR NIR, IT codice fiscale, IBAN and Luhn.

The part I didn't expect: putting a semantic cache behind the proxy creates a new leak. Two customers who ask
the same question differ only in their tokens, so the cache can answer customer B with customer A's response.
maskwire only restores tokens issued for the current request; anything else comes back as a neutral "[email]".
In the demo you can watch it happen.

Measured on one machine (AMD Ryzen 5 4600G, loopback, mock upstream, details in the repo):
• detection: ~400 MB/s per core on clean text, zero allocations
• added latency at 2,000 req/s with a 4 KB prompt: +0.22 ms p50, +0.52 ms p99, zero errors
• streaming: +0.29 ms to the first byte

What it is not: a GDPR or AI Act compliance product. Pseudonymised data is still personal data, and v0.1 does not
detect names. The README spells that out.

Go, Apache-2.0, single static binary, docker compose demo with no API keys:
https://github.com/Guilherme-Ciano/maskwire

#golang #privacy #gdpr #llm #opensource
