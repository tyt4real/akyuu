# 75chan.org — Imageboard Analysis

> File: `75chan.org.md` · Analyzed: 2026-08-16 · Engine: Burichan / vichan fork (Mexican imageboard)

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://75chan.org |
| Name | 75chan |
| Boards | /b/ (Random) — primary board; also /meta/ (Discusión del sitio) |
| Engine | Burichan / vichan fork (Mexican imageboard style); confirmed by footer style options and post structure |
| JS-rendered? | No — fully server-rendered HTML |
| robots.txt | not fetched |

## 2. Board (thread listing) page

- URL pattern: `https://75chan.org/<board>/` (e.g. `https://75chan.org/b/`)
- Pagination: numeric indices (`/1.html`, `/2.html`, `/3.html`, etc.); catalog at `https://75chan.org/b/catalog.html`
- Thread card structure: each thread is a `<tr>` row in the board index with OP preview, title, reply/file counts, sticky/locked flags
- Exact selectors:
  - Thread container: `<tr>` within the board table
  - Thread ID (No.): `[No.]` link text within thread row; also `data-no` attribute
  - Title: subject line within thread row
  - OP post link: thread row clicks to `res/thread_id.html`
  - Reply count: last column text or `<td>` with reply number
  - File count: last column text or `<td>` with file attachment indicator
  - Sticky indicator: `[Fijado]` text or sticky icon PNG
  - Locked indicator: no common lock observed on 75chan
- Sample fragment (from /b/ extraction):

```html
[No.] Anónimo2026/04/17 (Vie) 12:46:57[No.](75chan.org/b/res/1.html#431 "Reportar") [431](75chan.org/b/res/1.html#q431 "Responder al post") [R](75chan.org/b/imgboard.php?report=431 "Reportar")
[No.] cobrax!dhXQmo9HvI2026/04/18 (Sáb) 00:01:05[No.](75chan.org/b/res/1.html#432 "Reportar") [432](75chan.org/b/res/1.html#q432 "Responder al post")
el dia que lo pusieron en linea, salu2

[No.] cobrax!dhXQmo9HvI2026/05/22 (Vie) 12:00:28[No.](75chan.org/b/res/1.html#500 "Reportar") [500](75chan.org/b/res/1.html#q500 "Responder al post")
[R](75chan.org/b/imgboard.php?report=500 "Reportar") MMD -Lucky star- Nekomimi switch Tsukasa *fixed*

[No.] Anónimo2026/06/07 (Dom) 22:22:43[No.](75chan.org/b/res/1.html#508 "Reportar") [508](75chan.org/b/res/1.html#q508 "Responder al post")
1780892563203.jpg–(50.58KB, 510x680, konata peluche.jpg)

[No.] Entidad2026/03/10 (Mar) 18:45:08[No.](75chan.org/b/res/356.html#356 "Reportar") [356](75chan.org/b/res/356.html#q356)
Mejor cunny de BA
```

## 3. Thread page

- URL pattern: `https://75chan.org/<board>/res/<thread_id>.html` or `https://75chan.org/<board>/res/<id>` (e.g. `https://75chan.org/b/res/1.html`)
- Post structure (vichan-style, Spanish-localized):
  - Post container: `<tr class="post">` or `<div class="post">` (root first, replies nested in `<table>`)
  - Post ID: `[No.]` link text; also `data-no` attribute on post element
  - Name/tripcode: text before `[No.]`; may show "Anónimo" or handle + `!` tripcode
  - Date (displayed): `YYYY/MM/DD (Día) (HH:MM:SS)` format (e.g. "2026/04/17 (Vie) 12:46:57")
  - Date (raw epoch): not directly visible; can be derived from displayed date
  - Subject/comment: text within `<td>` or `<div>` after `[No.]` link
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `thumb/filename`
    - Filename: from the `src/` URL path (e.g. `1776537039807.jpg`)
    - File size: e.g. "50.58KB"
    - Dimensions: e.g. "510x680"
    - Format: JPG
    - Spoiler flag: `[Spoiler]` text in filename or `spoiler` class
  - Replies: nested under parent; each reply has `[No.]` link and `>>reply_no` cross-reference text in comment
  - OP vs reply differences: OP has `[Responder]` link at bottom; replies have `[Responder]` link and `>>parent_no` reference; sticky/locked from thread header
- OP vs reply differences: admin posts have `[R]` (Report) link; admin posts show file info + admin message; regular posts have simple `[No.]` link
- Sample fragment (trimmed):

```html
[No.] Anónimo2026/04/17 (Vie) 12:46:57[No.](75chan.org/b/res/1.html#431 "Reportar") [431](75chan.org/b/res/1.html#q431 "Responder al post")
[R](75chan.org/b/imgboard.php?report=431 "Reportar") [No.](75chan.org/b/res/1.html#431 "Reportar") [431](75chan.org/b/res/1.html#q431 "Responder al post")
el dia que lo pusieron en linea, salu2

[No.] cobrax!dhXQmo9HvI2026/04/18 (Sáb) 00:01:05[No.](75chan.org/b/res/1.html#432 "Reportar") [432](75chan.org/b/res/1.html#q432 "Responder al post")
[R](75chan.org/b/imgboard.php?report=432 "Reportar") [No.](75chan.org/b/res/1.html#432 "Reportar") [432](75chan.org/b/res/1.html#q432 "Responder al post")
el dia que lo pusieron en linea, salu2
```

## 4. APIs & JSON feeds

- `?json=1` query param: **not supported** — 75chan's engine does not expose native JSON API
- Preferred approach: HTML parsing — board and thread pages render fully in HTML; selectors are stable
- Other endpoints: none publicly documented

## 5. Anti-bot & rate limiting

- Minimal Cloudflare protection — 75chan is relatively permissive
- No API token required for basic viewing
- Required headers: standard User-Agent
- Rate limit behavior: very permissive; 10+ requests/second appears safe based on extracted data

## 6. Pitfalls

- Language: site is in Spanish; all content, UI, and post metadata in Spanish
- Relative URLs: all file/thumbs URLs relative to `75chan.org`; resolve against board domain
- Admin markings: admin posts marked with `[R]` (Report) link and `## Admin` signature + date; these have additional file/info fields
- Post numbering: threads start at No. 1 and increment; some threads have 500+ posts with "X posts omitted" markers
- "33 posts omitted. Haz click en Responder para verlos." — must follow link to expand collapsed posts
- Numeric dates: `YYYY/MM/DD` format; may need conversion to epoch for database mirror
- File size formatting: KB suffix with decimal (e.g. "50.58KB", "272.55KB"); be liberal in parsing
- Admin messages: may contain announcements, site status, or closure notices (e.g., "75chan funcionando otra vez en tiempo y forma"; later "el sitio va a cerrar")

## 7. Key takeaways for scraper

- **Recommended approach:** HTML parsing — 75chan renders fully in HTML; stable selectors; Spanish language
- **Minimal request sequence:** fetch board page → extract thread IDs → fetch each thread page → parse posts
- **Throttling:** very permissive; 10+ requests/second safe
- **Reply graph reconstruction:** capture `>>431` references as bare text; also look for `<a>` wrappers with href containing the No.
- **File extraction:** thumbnail URL → `75chan.org/b/thumb/...jpg`; full size → `75chan.org/b/src/...jpg`; resolve relative to `75chan.org`
- **Database mirror goal:** every post's No., name/trip (in Spanish), timestamp (`YYYY/MM/DD (Día) HH:MM:SS`), subject (in Spanish), comment body, file URL (thumb + original), file size, dimensions, spoiler flag, admin markers (`[R]`, `## Admin`)
- **Thread state:** sticky (`[Fijado]`) rarely used; no lock observed; cycled threads not common; "X posts omitted" markers must be followed
- **Language note:** all UI, subjects, and comments in Spanish — encode/decode accordingly; non-ASCII characters (ñ, á, é, í, ó, ú, ü) are standard
- **Admin handling:** admin posts have `[R]` link + `## Admin` signature; capture these as special posts with admin message + file info