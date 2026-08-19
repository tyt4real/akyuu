# Desuarchive.org — 4chan archive (FoolFuuka)

> File: `desuarchive.md` · Analyzed: 2026-08-18 · Engine: FoolFuuka

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://desuarchive.org |
| Boards | 22 mirrored 4chan boards (auto-discovered via `/_/api/chan/archives/`) |
| Engine | FoolFuuka |
| API | `/_/api/chan/` (JSON), also served by 4plebs/nyafuu/wakarimasen archives |
| robots.txt | not fetched |

Desuarchive is a read-only archive of 4chan: boards contain every archived
thread, newest first. Unlike live 4chan there is no pagination of currently
active threads — history is enumerated page by page through the index endpoint.

## 2. JSON API (the `/_/api/chan/` feed)

All endpoints are GET, plain JSON, no auth, block `Python-urllib` user agents
(403) — always send a real UA.

| Endpoint | Shape |
|---|---|
| `/_/api/chan/boards/` | internal boards only (`site`, `boards`, `Articles`) |
| `/_/api/chan/archives/` | `site` + `archives: {<id>: {shortname, name, is_nsfw, ...}}` — the 22 mirrored boards |
| `/_/api/chan/index/?board=X&page=N` | `{<threadnum>: {omitted, images_omitted, op: {…}}}` — one page of history (~10 threads) |
| `/_/api/chan/thread/?board=X&num=Y` | `{"<Y>": {op: {…}, posts: {<postnum>: {…}}}}`; `{"error":"Thread not found."}` (HTTP 200) for missing |

### Post object (thread-style)

- `num`, `thread_num`, `subnum` (skip ghost posts where `subnum != "0"`)
- `timestamp` (unix seconds), `sticky`/`locked`/`deleted` as **strings**
- `name`/`name_processed`, `trip`/`trip_processed`, `capcode`, `poster_hash`
  (poster_id), `poster_country`, `email` (`"sage"` marks sage), `title`
- `comment` (raw BBCode with `>>num` quotes), `comment_processed` (sanitized HTML)
- `media`: `media_link`/`thumb_link` (absolute CDN URLs on
  `desu-usergeneratedcontent.xyz`), `media_hash` (**base64 md5** — mapped to hex
  for blob dedup), `media_size`/`media_w`/`media_h` as **strings**, `media_status`
  (`"normal"` only → file kept), `spoiler`
- replies have `resto: null`; use `thread_num` as the parent id

## 3. Adapter notes (`internal/adapter/desuarchive`)

- `ListBoards` reads `/archives/` (not `/boards/`).
- `FetchCatalogPage(ctx, board, page)` reads `/index/`; `FetchCatalog` = page 1.
- `FetchThread` maps the 200 `{"error": …}` body to `httpclient.ErrNotFound`.
- `ParsePost` skips files whose `media_status` is empty or not "normal" and maps
  `media_hash` (base64 md5) to the hex form the downloader dedups on.
- Thread posts always carry `archived: true`, so `runThread` archives them
  immediately after the first fetch.

## 4. Scheduler integration

- Site config sets `auto_boards: true` (discover + upsert the 22 boards at
  startup, appended to the site's board list) and `paginated_catalog: true`
  (catalog jobs carry a per-board page cursor instead of a live board snapshot).
- Cursor is stored on `boards.archive_page`; each poll crawls
  `crawl_pages_per_poll` pages (default 1) and advances the cursor on success.
- Reaching the end of history (an empty page) wraps the cursor back to page 1,
  so newly archived threads are picked up; the mirror dedup keeps re-crawls
  cheap (only index pages are re-fetched, not threads).
- A catalog error leaves the cursor untouched and reschedules the job with
  backoff.
- Paginated crawls never run the "active thread missing from catalog → archive"
  pass — a history page is not the whole board.

## 5. Mirror dedup (4chan ↔ desuarchive)

4chan post numbers are globally unique, so a thread pulled from live 4chan and
the identical thread on desuarchive share a native id. `runCatalog` calls
`store.FindThreadMirror` for every thread in every catalog (both live 4chan and
desuarchive); a thread whose native id already exists on any other board of a
`fourchan`/`desuarchive` site is skipped entirely — no upsert, no thread job, no
duplicate download. The rule is symmetric and applies regardless of crawl order.

## 6. Operations

- Start a crawl: add `config/sites/desuarchive.yaml`, run the archiver. First
  pass discovers the boards; the crawl then advances ~10 threads per board per
  poll cycle at the configured `rate_limit_per_sec`.
- To restart a board's crawl from the newest page, reset its cursor:
  `UPDATE boards SET archive_page = 1 WHERE code = 'X';`
- Default config is text + thumbnails (`download_thumb: true`,
  `download_full: false`); flip `download_full: true` to keep originals.