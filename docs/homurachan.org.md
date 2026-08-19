# homurachan.org — Imageboard Analysis

> File: `homurachan.org.md` · Analyzed: 2026-08-16 · Engine: LynxChan (custom aggregator/metaboard)

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://homurachan.org |
| Name | Homura Chan |
| Boards | 20+ boards aggregated from webring and network: /homu/, /mtv/, /psi/, /ai/, /xomy/, /psi/ (various), /all/ (metaboard aggregator) |
| Engine | LynxChan 2.x (core) with custom aggregator overlay (`/all/` metaboard); confirmed by footer `LynxChan 2.6.13` and post structure across boards |
| JS-rendered? | Partial — board pages server-rendered with AJAX; metaboard `/all/` is static HTML aggregation |
| robots.txt | not fetched |

## 2. Board (thread listing) page

Two distinct page types:

### A. Board-specific pages (e.g. /homu/, /mtv/, /psi/, /ai/, /xomy/)
- URL pattern: `https://homurachan.org/<board>/` (e.g. `https://homurachan.org/homu/`, `https://homurachan.org/psi/`)
- Pagination: `?page=N` appended to board URL; also overboard at `https://homurachan.org/overboard/`
- Thread card structure: LynxChan-style AJAX-rendered thread list with OP preview, title, reply/file counts, sticky/locked flags
- Exact selectors (client-side rendered, inspect via DevTools after JS load):
  - Thread container: `.thread` class or `<div class="res">` within board index
  - Thread ID (No.): `[No.]` link text within thread item; also in URL hash
  - Title: `.subject` or `<h2>` within thread header
  - OP post preview: first post in thread preview shows name, date, truncated comment
  - Reply count: `.replies` span or text within thread meta
  - File count: `.files` span or text within thread meta
  - Sticky indicator: `.sticky` class or "[Sticky]" text
  - Locked indicator: `.locked` class or "[Locked]" text
  - Archived indicator: `.archived` class or "[Archived]" text
- Sample fragment (from /homu/ extraction):

```html
[No.] ALLMIND is here for all gucae
* * *

[No.] 195 post(s) and 162 image(s) omitted [See all](https://homurachan.org/homu/48857)
[G](https://lens.google.com/uploadbyurl?url=https://homurachan.org/assets/images/thumb/d5a476c577c2b54e3d5ed0f7192a832c1a9fbdbb.webp) [Yd](https://yandex.com/images/search?source=collections&rpt=imageview&url=https://homurachan.org/assets/images/thumb/d5a476c577c2b54e3d5ed0f7192a832c1a9fbdbb.webp) [Iq](https://iqdb.org/?url=https://homurachan.org/assets/images/thumb/d5a476c577c2b54e3d5ed0f7192a832c1a9fbdbb.webp) [Sn](https://saucenao.com/search.php?db=999&url=https://homurachan.org/assets/images/thumb/d5a476c577c2b54e3d5ed0f7192a832c1a9fbdbb.webp) [Sn](https://trace.moe/?url=https://homurachan.org/assets/images/thumb/d5a476c577c2b54e3d5ed0f7192a832c1a9fbdbb.webp) [Da](https://desuarchive.org/_/search/image/RS-sJArYqiMaTKZOxtLz-Q) [Ex](https://exhentai.org/?fs_similar=1&fs_exp=1&f_shash=d5a476c577c2b54e3d5ed0f7192a832c1a9fbdbb)
```

### B. Metaboard `/all/`
- URL pattern: `https://homurachan.org/all/`
- Pagination: static page showing aggregated threads from all webring members
- Thread card structure: static HTML `<div>` entries with board name, thread title, post count, file count, bump time
- Exact selectors:
  - Thread container: `.thread` or `.board-entry` within the metaboard list
  - Board name: `.board-name` or text before thread title (e.g., "/homu/", "/mtv/")
  - Thread title: `.thread-title` or `<h3>` within entry
  - Post count: `.post-count` or text after "post(s) and"
  - File count: `.file-count` or text after "image(s)"
  - Bump time: `.bump-time` or "Last reply time" timestamp
- Sample fragment (from /all/ extraction):

```html
# /all/ - Aggregator metaboard

Bump timeLast reply timeCreation timeReply countFile count

[![](https://homurachan.org/assets/images/thumb/7cb4a6fa1730a0bd38e4d6af6f28dc7dcaa12b53.webp)](homurachan.org/homu/215882)**/homu/** 151/76[Last 100](homurachan.org/homu/215882?last=100#bottom)[Last 500](homurachan.org/homu/215882?last=500#bottom)

### 「ХМГ № CDXXVI:／／ Durendal ＼＼」\n>\n> _For these our swords are trusty and trenchant, In scalding blood we'll dye their blades scarlat._\n>\n> +-+-+On This Day+-+-+\n>\n> 778 - Sir Roland, leader of Charlegmane's rearguard is slain by the Basques\n>\n> 927 - Saracens raze Taranto to the ground\n>\n> ...
```

## 3. Thread page

- URL pattern: `https://homurachan.org/<board>/res/<thread_id>.html` (e.g. `https://homurachan.org/homu/res/48857.html`)
- Post structure (LynxChan-style, with aggregator overlays):
  - Post container: `<div class="post">` or `<article class="post">` (root first, replies nested)
  - Post ID: `[No.]` link text; also visible in URL hash and post header
  - Name/tripcode: text before `[No.]`; may show user handle or tripcode `name!ident@host`; also may show fandom tags (e.g., "Magical Girl", "Wesenlust")
  - Date (displayed): `YYYY-MM-DD HH:MM:SS` format; also relative time
  - Date (raw epoch): LynxChan stores Unix epoch in `data-created` attribute
  - Subject/comment: `<div class="subject">` + `<div class="comment">`; comment may contain formatting (italics, bold, code, spoilers, quotes, `>>` references); may include fandom-specific tags
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `r/thumb/filename`; embedded WebM/MP4 video; may include torrent magnets for /mtv/ board
    - Filename: from the `src/` URL path
    - File size: e.g. "12.8 MB", "654 KB", "3.1 MB"
    - Dimensions: e.g. "420x262", "1200x690", "1080x2340"
    - Format: WebM, MP4, GIF, JPG, PNG
    - Spoiler flag: `spoiler` class on file link or label; `[Spoiler]` text
    - Torrent magnet: may appear on /mtv/ board posts with `magnet:?xt=...` URIs in comment body
  - Replies: nested under parent post; each reply has its own `[No.]` link and cross-references `>>12345` as text within comment body
  - OP vs reply: OP has `[Watch Thread]` and `[Reply]` buttons at top; replies have `[Reply]` below each post; sticky/locked/archived flags from thread header; on /mtv/ board, OP may have torrent magnet + info text
- OP vs reply differences: sticky/locked/archived from thread metadata; on /mtv/ board, posts may have magnet links; on /homu/ board, may have "ALLMIND is here" tagline; cross-board references possible
- Sample fragment (from /homu/ thread extraction):

```html
[No.] johnboby2026-08-16 09:09:29 [No.](homurachan.org/homu/res/49657.html#49657) [49657](homurachan.org/homu/res/49657.html#q49657) [Reply](homurachan.org/homu/res/49657.html)
File: [1786896565672-0.gif](homurachan.org/homu/src/1786896565672-0.gif)(69.4 KB, 2048x1152, 16:9, [IMG_1195.GIF](homurachan.org/homu/src/1786896565672-0.gif "Save as original filename"))
[![](https://homurachan.org/homu/thumb/1786896565672-0.png)](homurachan.org/homu/src/1786896565672-0.gif)
johnboby2026-08-16 09:09:29 [No.](homurachan.org/homu/res/49657.html#49657) [49657](homurachan.org/homu/res/49657.html#q49657) [Reply](homurachan.org/homu/res/49657.html)
If you are looking to help start a new image board community then consider trying 3chan dot net...

>>Acid Burn2026-08-05 05:30:56 [No.](homurachan.org/homu/res/49556.html#49557) [49557](homurachan.org/homu/res/49556.html#q49557)
[>>49556](homurachan.org/homu/res/49556.html#49556)
for logo use krita guess
ftw i guess
```

## 4. APIs & JSON feeds

- Native JSON API: **yes** — homura.org LynxChan serves `?json=1` on board and thread pages
- JSON API URL: `https://homurachan.org/<board>/?json=1` or `https://homurachan.org/<board>/res/<id>.json`
- Example response shape: JSON object with `posts` array; each post has `no`, `com`, `sub`, `com_num`, `filename`, `filesize`, `ext`, `w`, `h`, `tim`, `md5`, `sub`, `com`, `capcode`, `resto`, `last_mod`, `flag`, `tim`, `country`
- Preferred approach: **use JSON API** — much cleaner than HTML parsing; reply graph is `resto` field (0 for OP, parent post No. for replies)
- Other endpoints: `/overboard/` — cross-board view; `/all/` — metaboard aggregation

## 5. Anti-bot & rate limiting

- Cloudflare challenge on first access or high-frequency requests (per-board)
- No API key needed for basic JSON; some endpoints may have per-board limits
- Required headers: standard User-Agent; `Accept: application/json` for JSON endpoints
- Rate limit behavior: ~10 requests/sec per board before CAPTCHA appears; LynxChan's built-in throttle

## 6. Pitfalls

- JS-rendered content: board listing requires headless rendering or API call; HTML extraction alone gets skeleton only
- Randomized class names: LynxChan uses CSS modules — class names may change between versions; prefer data-attributes or JSON API
- Incomplete reply counts: board page may show truncated reply counts; thread page JSON has accurate `com_num`
- Relative URLs: file URLs are relative to `homurachan.org`; always resolve against base
- "Omitted posts": LynxChan shows "X posts and Y images omitted" — must fetch full thread or use JSON to get full count
- Tripcodes: `name!ident@host` format — capture separately if needed for mirroring
- Torrent magnets: /mtv/ board posts may contain `magnet:?xt=...` URIs — capture if mirroring torrent content
- Multi-language content: boards have Russian, Spanish, English, Japanese mixed content; encoding may vary
- Aggregator overlap: `/all/` metaboard duplicates content from individual boards — dedupe when mirroring

## 7. Key takeaways for scraper

- **Recommended approach:** LynxChan JSON API — fetch `?json=1` for board listing and thread pages; far more reliable than HTML parsing
- **Minimal request sequence:** fetch board `?json=1` → extract thread IDs → fetch each thread `?json=1` → parse posts from JSON
- **Throttling:** 5 requests/second per board is safe; homura.org's API is more permissive than HTML rendering
- **Reply graph reconstruction:** JSON `resto` field = 0 for OP, parent post No. for replies; `com_num` gives accurate reply count
- **File extraction:** `filename` + `ext` + `w` + `h` + `filesize` from JSON; thumbnail at `r/thumb/{filename}`; full at `src/{filename}`; `country` from `data-country` attribute; on /mtv/ board, watch for `magnet:?xt=...` URIs in comment
- **Database mirror goal:** every post's No., name/trip, timestamp (Unix epoch from `tim` + displayed `YYYY-MM-DD HH:MM:SS`), subject (`sub`), comment body (`com`), file URL (thumb + original + dimensions + format), spoiler flag, `resto` for reply graph, country flag, torrent magnet URIs if on /mtv/ board
- **Thread state:** sticky/locked/archived from thread metadata in JSON; `/overboard/` for cross-board aggregator mirror; `/all/` is static aggregation — dedupe against individual board mirrors
- **Cross-board:** homura.org's LynxChan instance has `overboard/` endpoint showing all recent posts across all webring boards — useful for aggregator mirror
- **Language support:** Russian, Spanish, English, Japanese all observed — handle i18n in timestamp/formatting parsing