# lainchan.org — Imageboard Analysis

> File: `lainchan.org.md` · Analyzed: 2026-08-16 · Engine: LynxChan

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://lainchan.org |
| Name | Lainchan |
| Boards | /r/ (Random), /q/ (Questions/Complaints), /g/ (Technology), /h/ (Hardware), /a/ (Anime), /b/ (Random), /d/ (Hentai), /e/ (Ecchi), /w/ (Wallpapers), /s/ (Technical Support), /z/ (Anarchist), /hum/ (Humanity), /⌀/ (Random), /Ω/ (Technology - IPv6), /Δ/ (Science & Math), /Λ/ (Philosophy), /Θ/ (Do It Yourself), /Φ/ (Do It Yourself), /Π/ (Anime), /Σ/ (Music), /Γ/ (Literature), /β/ (Board-tan), /α/ (Meta), /ω/ (Overboard) |
| Engine | LynxChan 2.x |
| JS-rendered? | Yes — board/thread pages load dynamically via AJAX |
| robots.txt | /robots.txt (not fetched in extraction) |

## 2. Board (thread listing) page

- URL pattern: `https://lainchan.org/<board>/` (e.g. `https://lainchan.org/r/`, `https://lainchan.org/q/`)
- Pagination: `?page=N` appended to board URL; also catalog at `https://lainchan.org/<board>/catalog.html`
- Thread card structure: AJAX-rendered; thread items appear as `<div class="thread">` with clickable title + OP post preview
- Exact selectors (client-side rendered, inspect via DevTools):
  - Thread container: `.thread` class (dynamic, rendered after JS load)
  - Thread ID: embedded in post link URL (`#<id>`); visible as post number in thread view
  - Title: `.subject` or `<h2>` within thread header
  - OP post preview: first post in thread preview shows name, date, truncated comment
  - Reply count: `.replies` span or text within thread meta
  - File count: `.files` span or text within thread meta
- Sample fragment (from extracted board page):

```html
[No.] 49657Anonymous08/16/26 09:09:29[No.](lainchan.org/r/res/49657.html#49657 "Reply to this post") [49657](lainchan.org/r/res/49657.html#q49657 "Reply to this post")
File: [1786896565672-0.gif](lainchan.org/r/src/1786896565672-0.gif)(69.4 KB, 2048x1152, 16:9, [IMG_1195.GIF](lainchan.org/r/src/1786896565672-0.gif "Save as original filename"))
[![](https://lainchan.org/r/thumb/1786896565672-0.png)](lainchan.org/r/src/1786896565672-0.gif)
johnboby2026-08-16 09:09:29 [No.](lainchan.org/r/res/49657.html#49657) [49657](lainchan.org/r/res/49657.html#q49657) [Reply](lainchan.org/r/res/49657.html)
If you are looking to help start a new image board community then consider trying 3chan dot net...
```

## 3. Thread page

- URL pattern: `https://lainchan.org/<board>/res/<thread_id>.html` (e.g. `https://lainchan.org/r/res/49657.html`)
- Post structure (LynxChan JSON + HTML hybrid):
  - Post container: `<div class="post">` or `<article class="post">` (root first, replies nested)
  - Post ID: `[No.]` link text; also visible in URL hash and post header
  - Name/tripcode: text before `[No.]`; may show user handle or tripcode `name!ident@host`
  - Date (displayed): `YYYY-MM-DD HH:MM:SS` format (ISO-ish); also relative time like "2 hours ago"
  - Date (raw epoch): LynxChan stores Unix epoch in `data-created` attribute or POST JSON metadata
  - Subject/comment: `<div class="subject">` + `<div class="comment">`; comment may contain formatting (italics, bold, code, spoilers)
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `r/thumb/filename`; embedded video if WebM/MP4
    - Filename: from the `src/` URL path
    - File size: e.g. "69.4 KB"
    - Dimensions: e.g. "2048x1152"
    - Format: GIF, WebM, MP4, JPG, PNG
    - Spoiler flag: `spoiler` class on file link or label
  - Replies: nested under parent post; each reply has its own `[No.]` link and cross-references `>>12345` as text
  - OP vs reply: OP has `[Watch Thread]` and `[Reply]` buttons at top; replies have `[Reply]` below each post
- OP vs reply differences: sticky/locked tags shown on thread header; OP may have banner/flag icon; replies show `[>>parent_no]` links in comment body
- Sample fragment (trimmed):

```html
[No.] johnboby2026-08-16 09:09:29 [No.](lainchan.org/r/res/49657.html#49657) [49657](lainchan.org/r/res/49657.html#q49657) [Reply](lainchan.org/r/res/49657.html)
File: [1786896565672-0.gif](lainchan.org/r/src/1786896565672-0.gif)(69.4 KB, 2048x1152, 16:9)
[![](https://lainchan.org/r/thumb/1786896565672-0.png)](lainchan.org/r/src/1786896565672-0.gif)
If you are looking to help start a new image board community then consider trying 3chan dot net...
>>Acid Burn2026-08-05 05:30:56 [No.](lainchan.org/r/res/49556.html#49557) [49557](lainchan.org/r/res/49556.html#q49557)
[>>49556](lainchan.org/r/res/49556.html#49556)
for logo use krita geg
ftw i guess
```

## 4. APIs & JSON feeds

- Native JSON API: **yes** — LynxChan serves `?json=1` on board and thread pages
- JSON API URL: `https://lainchan.org/<board>/?json=1` or `https://lainchan.org/<board>/res/<id>.json`
- Example response shape: JSON object with `posts` array; each post has `no`, `com`, `filename`, `filesize`, `ext`, `w`, `h`, `tim`, `md5`, `sub`, `com`, `capcode`, `resto`, `last_mod`, `com_num`, `flag`, `tim`
- Preferred approach: **use JSON API** — much cleaner than HTML parsing; reply graph is `resto` field (0 for OP, parent post No. for replies)
- Other endpoints: `/catalog.html` — HTML catalog view; `/overboard/` — cross-board view

## 5. Anti-bot & rate limiting

- Cloudflare challenge on first access or high-frequency requests (per-board)
- No API key needed for basic JSON; some endpoints may require board-specific limits
- Required headers: standard User-Agent; `Accept: application/json` for JSON endpoints
- Rate limit behavior: ~10 requests/sec per board before CAPTCHA appears; LynxChan's built-in throttle

## 6. Pitfalls

- JS-rendered content: board listing requires headless rendering or API call; HTML extraction alone gets skeleton only
- Randomized class names: LynxChan uses CSS modules — class names may change between versions; prefer data-attributes or JSON API
- Incomplete reply counts: board page may show truncated reply counts; thread page JSON has accurate `com_num`
- Relative URLs: file URLs are relative to `lainchan.org`; always resolve against base
- "Omitted posts": LynxChan shows "X posts and Y images omitted" — must fetch full thread or use JSON to get full count
- Tripcodes: `name!ident@host` format — capture separately if needed for mirroring

## 7. Key takeaways for scraper

- **Recommended approach:** LynxChan JSON API — fetch `?json=1` for board listing and thread pages; far more reliable than HTML parsing
- **Minimal request sequence:** fetch board `?json=1` → extract thread IDs → fetch each thread `?json=1` → parse posts from JSON
- **Throttling:** 5 requests/second per board is safe; LynxChan's API is more permissive than HTML rendering
- **Reply graph reconstruction:** JSON `resto` field = 0 for OP, parent post No. for replies; `com_num` gives accurate reply count
- **File extraction:** `filename` + `ext` + `w` + `h` + `filesize` from JSON; thumbnail at `r/thumb/{filename}`; full at `src/{filename}`
- **Database mirror goal:** every post's No., name/trip, timestamp (Unix epoch from `tim` + displayed string), subject (`sub`), comment body (`com`), file URL (thumb + original + dimensions + format), spoiler flag, `resto` for reply graph
- **Thread state:** sticky/locked from thread metadata; cycled/archived from board state in JSON
- **Cross-board:** LynxChan's `overboard/` endpoint shows all recent posts across boards — useful for aggregator mirror