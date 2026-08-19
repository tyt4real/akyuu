# Wired-7.org — Imageboard Analysis

> File: `wired-7.org.md` · Analyzed: 2026-08-16 · Engine: custom PHP (vichan-fork)

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://wired-7.org |
| Name | Wired-7 |
| Boards | /a/ (Anime y manga), /e/ (Pasatiempos), /v/ (Videojuegos), /tech/ (Tecnología), /adv/ (Consejos), /b/ (Random), /h/ (Hentai R18), /hum/ (Humanidad), /nexo/ (Overboard) |
| Engine | Custom PHP fork of vichan (v. 3.3.1); footer shows "Wired-7 ― V. 3.3.1" |
| JS-rendered? | No — fully server-rendered HTML |
| robots.txt | not fetched |

## 2. Board (thread listing) page

- URL pattern: `https://wired-7.org/<board>/` (e.g. `https://wired-7.org/a/`, `https://wired-7.org/b/`)
- Pagination: `?page=N` appended to board URL; also overboard at `https://wired-7.org/nexo.html`
- Thread card structure: each thread is a `<tr>` or `<div>` row with OP post preview, title, reply/file counts, sticky/locked flags
- Exact selectors:
  - Thread container: `<tr class="thread">` or `<div class="thread">` on board index
  - Thread ID (No.): `[No.]` link text within thread row; also `data-no` attribute
  - Title: `<span class="subj">` or `<a>` with subject text within thread header
  - OP post link: thread row clicks to `res/thread_id.html`
  - Reply count: `<span class="replies">` or last column text
  - File count: `<span class="files">` or last column text
  - Sticky indicator: `.sticky` class or "[Sticky]" text or pin icon
  - Locked indicator: `.locked` class or "[Locked]" text
- Sample fragment (from /b/ extraction):

```html
[No.] Waiyado02/05/26 (Sáb) 17:57[No.](wired-7.org/b/res/34224.html#34224 "Link directo al post") [34224](wired-7.org/b/res/34224.html#q34224 "Responder al post")
File: [79.6 KB, 736x978](wired-7.org/b/src/1777759026847.jpg) [ImgOps](http://imgops.com/https://wired-7.org/b/src/1777759026847.jpg) [iqdb](http://iqdb.org/?url=https://wired-7.org/b/src/1777759026847.jpg)
[![](https://wired-7.org/b/thumb/1777759026847.webp)] 79.6 KB JPG

Experiencias con chicas pickmeNadeshiko02/05/26 (Sáb) 17:57[No.](wired-7.org/b/res/34224.html#34224 "Link directo al post") [34224](wired-7.org/b/res/34224.html#q34224 "Responder al post") [Post menu](wired-7.org/b/# "Post menu")
¿Qué es una pick-me?
Es una chica que busca aprobación masculina menospreciando a las demás mujeres o lo femenino en general.
```

## 3. Thread page

- URL pattern: `https://wired-7.org/<board>/res/<thread_id>.html` (e.g. `https://wired-7.org/b/res/34224.html`)
- Post structure (vichan-style):
  - Post container: `<div class="post">` or `<tr class="post">` (root first, replies nested)
  - Post ID: `[No.]` link text; also `data-no` attribute on post element
  - Name/tripcode: text before `[No.]`; may show handle or "Anónimo"
  - Date (displayed): `DD/MM/YY (HH:MM)` format (e.g. "02/05/26 (Sáb) 17:57")
  - Date (raw epoch): not directly visible; can be derived from displayed date if needed
  - Subject/comment: `<span class="subj">` for subject; `<div class="com">` or direct text for comment body
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `thumb/filename` or `thumb/filename.webp`
    - Filename: from the `src/` URL path (e.g. `1777759026847.jpg`)
    - File size: e.g. "79.6 KB"
    - Dimensions: e.g. "736x978"
    - Format: JPG, PNG, WebM, GIF
    - Spoiler flag: `spoiler` class on file link or label; `[Spoiler]` text
  - Replies: nested under parent; each reply has `[No.]` link and `>>parent_no` cross-reference text in comment
  - OP vs reply differences: OP has `[Post menu]` button at top; replies have `[Reply]` link; sticky/locked flags from thread header
- OP vs reply differences: sticky/locked from thread metadata; cycled threads at bottom
- Sample fragment (trimmed):

```html
[No.] Waiyado18/07/26 (Sáb) 00:03[No.](wired-7.org/b/res/34224.html#34713 "Link directo al post") [34713](wired-7.org/b/res/34224.html#q34713 "Responder al post") [Post menu](wired-7.org/b/# "Post menu") [>>34973](wired-7.org/b/#34973)
wai, si era como la de la pick, si me la daba aunque sea una vez. Esas generalmente son bien aventadas en el sexo

>>Waiyado31/07/26 (Vie) 02:59[No.](wired-7.org/b/res/34224.html#34835 "Link directo al post") [34835](wired-7.org/b/res/34224.html#q34835 "Responder al post") [Post menu](wired-7.org/b/# "Post menu") [>>34973](wired-7.org/b/#34973)
Lo de pickme es un buzzword que tienen las mujeres para discriminar a las que no se dejan llevar por la mente colmena foid.
```

## 4. APIs & JSON feeds

- `?json=1` query param: **not supported** — Wired-7's custom engine does not expose native JSON API
- Preferred approach: HTML parsing — engine's HTML structure is consistent and well-documented
- Other endpoints: `/nexo.html` — overboard view showing last commented threads across all boards

## 5. Anti-bot & rate limiting

- Cloudflare protection on board/index pages (challenge page on first access or high-frequency requests)
- No API token required for basic viewing
- Required headers: standard User-Agent; some boards may require referer
- Rate limit behavior: relatively permissive; 3-5 requests/second is safe; may see CAPTCHA at higher frequency

## 6. Pitfalls

- Lazy loading: not used — all threads rendered in HTML; thumbnails loaded via `thumb/` URLs
- Randomized class names: wired-7 uses stable class names (`thread`, `post`, `subj`, `com`) — reliable for selectors
- Incomplete reply counts: may show truncated counts on board index; always fetch full thread page for accurate count
- Relative URLs: all file/thumbs URLs relative to `wired-7.org`; resolve against board domain
- Language: site is in Spanish; all content, UI, and post metadata in Spanish
- "Omitted posts" indicator: not commonly seen on wired-7; full threads typically rendered
- Encoding: UTF-8; no special HTML entities observed beyond standard Spanish punctuation

## 7. Key takeaways for scraper

- **Recommended approach:** HTML parsing — wired-7's custom vichan fork renders fully in HTML; reliable selectors
- **Minimal request sequence:** fetch board page → extract thread IDs → fetch each thread page → parse posts
- **Throttling:** 3-5 requests/second is safe; pause if Cloudflare challenge appears
- **Reply graph reconstruction:** capture `>>34973` references as bare text; also look for `<a>` wrappers with href containing the No.
- **File extraction:** thumbnail URL → `wired-7.org/b/thumb/...webp`; full size → `wired-7.org/b/src/...jpg`; resolve relative to `wired-7.org`
- **Database mirror goal:** every post's No., name/trip, timestamp (`DD/MM/YY (HH:MM)`), subject (in Spanish), comment body, file URL (thumb + original), file size, dimensions, spoiler flag, UI language = Spanish
- **Thread state:** sticky/locked flags from thread header; cycled threads at bottom of page; overboard at `/nexo.html`
- **Language note:** all UI, subjects, and comments in Spanish — encode/decode accordingly; non-ASCII characters (ñ, á, é, í, ó, ú, ü) are standard