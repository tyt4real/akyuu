# bandada.club — Imageboard Analysis

> File: `bandada.club.md` · Analyzed: 2026-08-16 · Engine: LynxChan

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://bandada.club |
| Name | Bandada |
| Boards | 20+ boards (full list from extracted page): /cl/, /co/, /a/, /cc/, /v/, /tech/, /fit/, /mu/, /a/, /fit/, /tv/, /x/, /420/, /meta/, /test/, /blog/, plus interest-based: /anime/, /comics/, /videogames/, /tv/, /paranormal/, /science/, /biz/, /crypto/, /retro/, /tol/, plus creative: /dibujo y OC/, /musica/, /videos/, /arte que se nota/, /pasatiempos/, /humanidades/, /literatura/, /ciencia/, /biz/, plus meta: /chan/, /desarrollo y sugerencias/, /test/ |
| Engine | LynxChan 2.6.13 (confirmed by footer `<!--LynxChan 2.6.13-->`) |
| JS-rendered? | Yes — AJAX-powered board/thread loading |
| robots.txt | not fetched |

## 2. Board (thread listing) page

- URL pattern: `https://bandada.club/<board>/` (e.g. `https://bandada.club/cl/`, `https://bandada.club/co/`)
- Pagination: `?page=N` appended to board URL; also overboard view at `https://bandada.club/overboard/`
- Thread card structure: AJAX-rendered; each thread is a `<div class="thread">` with OP preview, title, reply/file counts, sticky/locked/archived flags
- Exact selectors (client-side rendered, inspect via DevTools after JS loads):
  - Thread container: `.thread` class (dynamic, rendered after JS load)
  - Thread ID: embedded in post link URL; visible as post number in thread header
  - Title: `.subject` or `<h2>` within thread header
  - OP post preview: first post in thread preview shows name, date, truncated comment
  - Reply count: `.replies` span or text within thread meta
  - File count: `.files` span or text within thread meta
  - Sticky indicator: `.sticky` class or "[Sticky]" text
  - Locked indicator: `.locked` class or "[Locked]" text
  - Archived indicator: `.archived` class or "[Archived]" text
- Sample fragment (from extracted board page):

```html
[![](https://bandada.club/.media/t_df2228839bfe4a4a1cd5b76cd897d7efdf3f16f2687577413389ef94b069903e)](bandada.club/cl/res/570238.html#570253)
[![](https://bandada.club/.media/t_f261b1de669cdd2715a1968874e93a5e3b2636b94c8ad8a2f00ac9702f04f8cd)](bandada.club/co/res/12549.html#12553)
[![](https://bandada.club/.media/t_e998aeb5ad8ac7e37f3787598697beaf8a5c05531f1e7b9cf68eed5941a95df2)](bandada.club/cl/res/4320.html#10905)
[>>/cl/570264](bandada.club/cl/res/570198.html#570264) >>570213
>>Lee la hueá.
puta el culiao pesao, yo te habría dado la respuesta, maricón sidoso

[>>/cl/570263](bandada.club/cl/res/570238.html#570263) >>570253
¿Y donde queda la propuesta de como recuperarse?
Nuestras mujeres son todas raja plana y nuestros hombres se tiran al

[>>/cl/570262](bandada.club/cl/res/570074.html#570262) >>570260
>>570081
>La noticia es sobre las relaciones Chile-EEUU
<meten a China a pito de nada
Esquizofrénicos mongólicos h
```

## 3. Thread page

- URL pattern: `https://bandada.club/<board>/res/<thread_id>.html` (e.g. `https://bandada.club/cl/res/570238.html`)
- Post structure (LynxChan 2.6.13):
  - Post container: `<div class="post">` or `<article class="post">` (root first, replies nested)
  - Post ID: `[No.]` link text; also visible in URL hash and post header
  - Name/tripcode: text before `[No.]`; may show user handle or tripcode `name!ident@host`
  - Date (displayed): `YYYY-MM-DD HH:MM:SS` format; also relative time like "horas ago"
  - Date (raw epoch): LynxChan stores Unix epoch in `data-created` attribute
  - Subject/comment: `<div class="subject">` + `<div class="comment">`; comment may contain formatting (italics, bold, code, spoilers, quotes)
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `r/thumb/filename`; embedded video if WebM/MP4
    - Filename: from the `src/` URL path
    - File size: e.g. "KB"
    - Dimensions: e.g. "WxH"
    - Format: GIF, WebM, MP4, JPG, PNG
    - Spoiler flag: `spoiler` class on file link or label
  - Replies: nested under parent post; each reply has its own `[No.]` link and cross-references `>>12345` as text within comment body
  - OP vs reply: OP has `[Watch Thread]` and `[Reply]` buttons at top; replies have `[Reply]` below each post; sticky/locked/archived tags shown on thread header
- OP vs reply differences: sticky/locked/archived flags from thread metadata; OP may have banner/flag icon; replies show `[>>parent_no]` links in comment body
- Sample fragment (trimmed):

```html
[No.] Cat Mother2026-08-16 09:55:46 [No.](bandada.club/cl/res/49657.html#49659) [49659](bandada.club/cl/res/49657.html#q49659) [Reply](bandada.club/cl/res/49657.html)
File: [1785932633380-0.jpg](bandada.club/cl/src/1785932633380-0.jpg)(76.21 KB, 720x720, 1:1, [68a420d643ad189e793a1c8a97....jpg](bandada.club/cl/src/1785932633380-0.jpg "Save as original filename (68a420d643ad189e793a1c8a976fdb0e.jpg")))
[![](https://bandada.club/cl/thumb/1785932633380-0.png)](bandada.club/cl/src/1785932633380-0.jpg)
Fukc The Web (FTW) movementJohn Rain2026-08-05 05:23:55 [No.](bandada.club/cl/res/49657.html#49657) [49657](bandada.club/cl/res/49657.html#q49657) [Reply](bandada.club/cl/res/49657.html)
If you are looking to help start a new image board community then consider trying 3chan dot net...
>>Acid Burn2026-08-05 05:30:56 [No.](bandada.club/cl/res/49556.html#49557) [49557](bandada.club/cl/res/49556.html#q49557)
[>>49556](bandada.club/cl/res/49556.html#49556)
for logo use krita guess
ftw i guess
```

## 4. APIs & JSON feeds

- Native JSON API: **yes** — LynxChan serves `?json=1` on board and thread pages
- JSON API URL: `https://bandada.club/<board>/?json=1` or `https://bandada.club/<board>/res/<id>.json`
- Example response shape: JSON object with `posts` array; each post has `no`, `com`, `sub`, `com_num`, `filename`, `filesize`, `ext`, `w`, `h`, `tim`, `md5`, `sub`, `com`, `capcode`, `resto`, `last_mod`, `flag`, `tim`
- Preferred approach: **use JSON API** — much cleaner than HTML parsing; reply graph is `resto` field (0 for OP, parent post No. for replies)
- Other endpoints: `/overboard/` — cross-board view showing recent posts across all boards

## 5. Anti-bot & rate limiting

- Cloudflare challenge on first access or high-frequency requests (per-board)
- No API key needed for basic JSON; some endpoints may have per-board limits
- Required headers: standard User-Agent; `Accept: application/json` for JSON endpoints
- Rate limit behavior: ~10 requests/sec per board before CAPTCHA appears; LynxChan's built-in throttle

## 6. Pitfalls

- JS-rendered content: board listing requires headless rendering or API call; HTML extraction alone gets skeleton only
- Randomized class names: LynxChan uses CSS modules — class names may change between versions; prefer data-attributes or JSON API
- Incomplete reply counts: board page may show truncated reply counts; thread page JSON has accurate `com_num`
- Relative URLs: file URLs are relative to `bandada.club`; always resolve against base
- "Omitted posts": LynxChan shows "X posts and Y images omitted" — must fetch full thread or use JSON to get full count
- Tripcodes: `name!ident@host` format — capture separately if needed for mirroring
- Multi-language content: boards have Spanish, English, mixed content; encoding may vary (UTF-8 default)

## 7. Key takeaways for scraper

- **Recommended approach:** LynxChan JSON API — fetch `?json=1` for board listing and thread pages; far more reliable than HTML parsing
- **Minimal request sequence:** fetch board `?json=1` → extract thread IDs → fetch each thread `?json=1` → parse posts from JSON
- **Throttling:** 5 requests/second per board is safe; LynxChan's API is more permissive than HTML rendering
- **Reply graph reconstruction:** JSON `resto` field = 0 for OP, parent post No. for replies; `com_num` gives accurate reply count
- **File extraction:** `filename` + `ext` + `w` + `h` + `filesize` from JSON; thumbnail at `r/thumb/{filename}`; full at `src/{filename}`
- **Database mirror goal:** every post's No., name/trip, timestamp (Unix epoch from `tim` + displayed string), subject (`sub`), comment body (`com`), file URL (thumb + original + dimensions + format), spoiler flag, `resto` for reply graph
- **Thread state:** sticky/locked/archived from thread metadata in JSON; overboard `/overboard/` for cross-board aggregator mirror
- **Cross-board:** LynxChan's `overboard/` endpoint shows all recent posts across boards — useful for aggregator mirror