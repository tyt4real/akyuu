# akyuu - the better imageboard archiver
<img width="520" height="699" alt="akyuu" src="https://github.com/user-attachments/assets/d5679f0e-d440-4d3d-b629-06b88d8dd6cd" />

(((better patchouli?!)))

A self-hosted, multi-site imageboard archiver. It polls 4chan-style boards
(4chan itself, vichan forks, LynxChan instances) plus FoolFuuka 4chan archives
into a normalized Postgres schema, keeps one copy of every unique file content
across all sites, and can optionally download full images and thumbnails behind
a mandatory CSAM hash check. It can also index post bodies and answer
**semantic search** queries over the archive (see below).

No content is ever stored from a site — only structure (board, thread, post,
quote graph, file metadata). Downloading bytes is optional and off by default.

Built cuz the previous one was bad

## Quick start

```sh
# 1. Postgres
createdb akyuu            # or the equivalent for your install

# 2. Create your config from the template, then edit it
cp config/config.yaml.example config/config.yaml
$EDITOR config/config.yaml

# 3. Run
go run ./cmd/archiver -config config/config.yaml
```

Sites are read from `config/sites/*.yaml`; every file becomes a site row, every
board a board row. Boards removed from config are left in the DB, never deleted.

## Configuration

### Global (`config/config.yaml`)

`config/config.yaml` is your private, git-ignored file (it holds the DB DSN).
The committed template is `config/config.yaml.example` — copy it and edit:

- `database.dsn` — required Postgres DSN.
- `storage.dir` — root for the content-addressable tree.
- `safety` — CSAM hash sources (below).
- `scheduler` — global defaults for poll interval, rate limiting, download
  policy, circuit breaker, job retries, thread staleness/archiving.
- `sites_dir`, `log_level`.

### Per-site (`config/sites/*.yaml`)

```yaml
name: lainchan
base_url: https://lainchan.org
platform: lynxchan            # lynxchan | vichan | fourchan
api_available: true           # whether a JSON feed exists (used for thread fetching)
poll_interval: 60s            # optional, overrides the global default
rate_limit_per_sec: 2         # optional
proxy: "socks5://127.0.0.1:9050"   # optional: route this site's scraping +
                                   # downloads through a SOCKS5 proxy (e.g. Tor)
boards:
  - code: r
    title: Random
    nsfw: false
    download_full: true       # optional per-board override
    download_thumb: true
```

Download flags resolve global default → site → board (first non-nil wins).
Setting `scheduler.text_only: true` globally disables **both** full-res and
thumbnail downloads (a text-only archive) regardless of any per-site/board
overrides; the scheduler then never enqueues download or backfill jobs.

### Archive sources

Sites with `platform: desuarchive` pull 4chan's archived history instead of a
live board:

- `auto_boards: true` — discover and sync all mirrored boards at startup from
  the archive's board listing (no static `boards:` list).
- `paginated_catalog: true` — the board catalog is the full thread history,
  crawled page by page. Each board tracks a cursor (`boards.archive_page`) and
  advances it by `crawl_pages_per_poll` pages (default 1) per poll cycle.
  Reaching the end of history wraps back to page 1 so newly archived threads
  are still picked up.
- Mirror dedup is automatic: 4chan post numbers are globally unique, so a
  thread already pulled from live 4chan (or from the archive on another board)
  is never pulled again — the catalog entry is skipped. See
  `docs/desuarchive.md`.

### Semantic search

Post bodies can be embedded (in-process ONNX, pgvector) and searched by
meaning. Opt-in via the `embeddings` config section; an embed worker inside
`cmd/archiver` indexes posts (including the one-time backfill of existing
archives), and a separate `cmd/api` binary serves `POST /search` and
`GET /health`. Setup, API shape and caveats: `docs/semantic-search.md`.

### Supported platforms

- **LynxChan** (`platform: lynxchan`) — native JSON API. Catalog via
  `GET /<board>/?json=1`, thread via `GET /<board>/res/<id>.json`. Handles the
  three catalog shapes in the wild (array, map keyed by thread id, flat post
  list). Files from `files[]` or constructed `src/<tim><ext>` /
  `thumb/<tim>.png` paths.
- **vichan** (`platform: vichan`) — server-rendered HTML (`data-no`,
  `data-time`, `.com` comment block, `src/` + `thumb/` file links). Used by
  4chon.me, wired-7.org and friends.
- **4chan** (`platform: fourchan`) — 4chan's HTML catalog and thread pages
  (`data-no`, `.postMessage`, `.fileText`/`.fileThumb`, `poster_hash`).
  Files are resolved from the `//i.4cdn.org` CDN.
- **desuarchive** (`platform: desuarchive`) — FoolFuuka's `/_/api/chan/` JSON
  feed (`/_/api/chan/archives/` for board discovery, `/index/` for paginated
  history, `/thread/` for full threads). Media carries the base64 md5 used for
  blob dedup before any bytes are downloaded; `media_status` filters out
  deleted/banned files. See `docs/desuarchive.md`.

Quote extraction (`>>123`, `>>>/board/123`) and HTML sanitization are shared in
`internal/parser`; each adapter wires its own markup to it.

## Downloading & storage

With `download_full` (or `download_thumb`) enabled, content is fetched by the
scheduler's download job and stored content-addressably:

```
storage/full/<hash[0:2]>/<hash[2:4]>/<hash><ext>
storage/thumb/<hash[0:2]>/<hash[2:4]>/<hash>.jpg
```

Same content from two sites lands at the same path — the `blobs` table indexes
it once and tracks `ref_count`. Platform-provided hashes (md5/sha1) are bound
to blobs at write time so dedup happens even before any bytes are downloaded.
Thumbnails are generated locally for raster images (max 250px, JPEG) and a
placeholder is stored for non-raster content.

## CSAM safety

Every file scheduled for full download passes a hash-matching check first.
Configure sources in `safety`:

- `hash_list_path` — a text file of hex SHA-256 hashes, one per line (`#`
  comments allowed). Run a tool like `photodna`/`pdq` over the list as needed.
- `api_url` + `api_token` — an optional PhotoDNA-style remote endpoint.

The check runs on **every** file and cannot be disabled per-site or per-board.
A match discards the bytes before storage and logs it — the file is not
retried. With no source configured the gate still runs; it simply matches
nothing (and logs a warning at startup). Running this archiver without a
populated hash list is not a substitute for real moderation.

## Adding a site

1. Copy an existing `config/sites/*.yaml`, set `name`, `base_url`, `platform`
   and the board list.
2. Confirm the platform's parser handles the site by inspecting its HTML/JSON
   against the samples in `docs/` (each has a page naming the platform).
3. Restart — `main` syncs new sites/boards into the DB.

## Adding a platform

1. Implement `internal/adapter.Adapter` in a new package
   (`FetchCatalog`, `FetchThread`, `ParsePost`, `PlatformName`) using the
   shared `model` types and `httpclient`. Optional capabilities are extra
   interfaces the scheduler type-asserts: `BoardLister` (`ListBoards`) enables
   `auto_boards` discovery, `PaginatedCataloger` (`FetchCatalogPage`) enables
   paginated history crawls.
2. Register it in `adapter.New`'s platform switch.
3. Add a site config with the new `platform` value.
4. Write an adapter test from a real sample (see the existing ones; they are
   served via `httptest` so no network is needed).

## Development

```sh
go build ./... && go vet ./...
gofmt -l .

# Unit tests (no DB needed)
go test ./internal/parser/ ./internal/config/ ./internal/downloader/ ./internal/adapter/...

# Full suite including integration tests (against a local Postgres).
# -p 1 keeps packages from racing each other on the shared test database:
# the store/scheduler/downloader tests TRUNCATE the tables they use.
AKYUU_TEST_DSN='postgres://user:pass@localhost:5432/akyuu?sslmode=disable' \
  go test -p 1 -count=1 ./...

# With coverage:
AKYUU_TEST_DSN='postgres://user:pass@localhost:5432/akyuu?sslmode=disable' \
  go test -p 1 -count=1 -cover ./...
```

The store, scheduler, and downloader integration tests `TRUNCATE` their
tables, so point them at a scratch database. A throwaway instance is easy
with Docker:

```sh
docker run -d --name akyuu-pg -e POSTGRES_USER=akyuu -e POSTGRES_PASSWORD=akyuu \
  -e POSTGRES_DB=akyuu -p 5432:5432 postgres:16-alpine
```
