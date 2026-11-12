# Running maskwire in front of semantic-cache-gateway

maskwire produces deterministic, tenant-keyed tokens, so the same prompt always reaches the cache as the same
masked text. That is what lets a semantic cache keep hitting while personal data stays out of it. This page
records what the chain needs from [semantic-cache-gateway](https://github.com/Guilherme-Ciano/semantic-cache-gateway)
(SCG).

## Bugs patched at demo build time

The demo compose file builds SCG from `../semantic-cache-gateway` through `deploy/scg/Dockerfile`, which applies
three one-line fixes during the image build. The SCG repository itself is not modified. Each one should become an
upstream PR:

| Symptom | Cause | Fix applied in the demo image |
|---|---|---|
| Every successful response is `200` with an empty body | `timeoutWriter.Write` returns `0, nil` once `WriteHeader` has run, so the JSON body written after `WriteHeader(200)` is dropped (`internal/adapters/httpserver/middleware.go`) | Pass the write through to the wrapped writer |
| Startup panic with Redis Stack 7.4: `RESP3 responses for this command are disabled` | go-redis v9.7 negotiates RESP3 and refuses `FT.INFO` unless `UnstableResp3` is set | Set `Protocol: 2` on the go-redis client |
| Startup fails with `ft.info "rag_cache_idx": Unknown index name` on a fresh Redis | `isNoIndexErr` matches `"Unknown Index name"` case-sensitively; Redis Stack 7.4 says `"Unknown index name"` | Compare in lower case |

## Namespace isolation SCG needs

SCG builds its cache key from the embedding of the user messages only. It has no tenant or subject dimension.
With masking in front of it, two customers who write the same request differ only in their tokens:

```
Customer A: "...reply to the customer at [EMAIL_FBDYD9] ... CPF [CPF_A0SA1Q] ..."
Customer B: "...reply to the customer at [EMAIL_DRV3SE] ... CPF [CPF_TYVCR2] ..."
```

Those embed almost identically, so customer B is served the answer generated for customer A. The demo shows
this in scenario 3 (`docs/demo-output.txt`): the cache answers with A's tokens, and maskwire's foreign-token
guard replaces them with `[email]` and `[cpf]` instead of restoring A's values. The guard prevents the data
breach, but B still gets an answer written for someone else.

The change SCG needs:

1. **Tenant namespace in the key.** Accept an `X-Cache-Namespace` header (maskwire would send the tenant id) and
   store it as a tag field in the index; filter every KNN query on it. No entry is ever shared across tenants.
2. **Subject pre-key.** Extract the set of `[TYPE_SUFFIX]` tokens from the masked prompt, hash the sorted set, and
   require an exact match on that hash before accepting a semantic hit. Prompts about different subjects then
   never share an answer, while paraphrases about the same subject still hit, because maskwire's tokens are
   deterministic per tenant.
3. **Optionally**, skip caching entirely when a prompt carries tokens of types the operator marks as sensitive.

Until SCG has (1) and (2), route traffic for subject-specific prompts through maskwire's direct route, or accept
that the guard turns cross-subject hits into generic answers.
