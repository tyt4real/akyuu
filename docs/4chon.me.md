# 4chon.me — Imageboard Analysis

> File: `4chon.me.md` · Analyzed: 2026-08-16 · Engine: Tinyboard / vichan hybrid

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://4chon.me |
| Name | 4chon |
| Boards | /new/, /lounge/, /scrap/ (3 active boards) |
| Engine | Tinyboard-based (fork of vichan); confirmed by `/new/` and `/lounge/` page source |
| JS-rendered? | Partial — server-rendered HTML with some client-side enhancements |
| robots.txt | /robots.txt (not fetched in extraction) |

## 2. Board (thread listing) page

- URL pattern: `https://4chon.me/<board>/` (e.g. `https://4chon.me/new/`, `https://4chon.me/lounge/`)
- Pagination: `?page=N` or numeric index (`/2.html`, `/3.html`, etc.); catalog at `https://4chon.me/<board>/catalog.html`
- Thread card structure: each thread is a `<tr>` or `<div>` row with OP post preview, title, reply/file counts
- Exact selectors:
  - Thread container: `<tr class="thread">` on /new/ and /lounge/ pages
  - Thread ID (No.): `[No.]` link text within thread row; also `data-no` attribute
  - Title: `<span class="subj">` or `<a>` with subject text within thread header
  - OP post link: thread row clicks to `res/thread_id.html`; OP has file attachment in thread view
  - Reply count: `<span class="replycount">` or last column text like "33 Images / 58 Replies"
  - File count: `<span class="filecount">` or last column text
- Sample fragment (from /new/ extraction):

```html
[No.] Anonymous07/26/26 (Sun) 09:42:04 [No.](4chon.me/new/res/11588.html#11593 "Reply to this post") [11593](4chon.me/new/res/11588.html#q11593 "Reply to this post")
File: [1784761028001.jpeg](4chon.me/new/src/1784761028001.jpeg) (636.38 KB, 3000x2385, [20190810-RitXYs0AziaFkxWgY....jpeg](4chon.me/new/src/1784761028001.jpeg "20190810-RitXYs0AziaFkxWgY7n6.jpeg"))
[![](https://4chon.me/new/thumb/1784761028001.webp)](4chon.me/new/src/1784761028001.jpeg)
Anonymous07/26/26 (Sun) 09:42:04 [No.](4chon.me/new/res/11588.html#11593 "Reply to this post") [11593](4chon.me/new/res/11588.html#q11593 "Reply to this post")
Where is a description of this thing?
* * *
Anonymous07/22/26 (Wed) 18:57:11 [No.](4chon.me/new/res/11565.html#11565 "Reply to this post") [11565](4chon.me/new/res/11565.html#q11565 "Reply to this post")
new vichan software
if you have feature requests please leave them on github
8 posts and 1 image reply omitted. Click reply to view.
```

## 3. Thread page

- URL pattern: `https://4chon.me/<board>/res/<thread_id>.html` or `https://4chon.me/<board>/thread/<thread_id>`
- Post structure (vichan/Tinyboard hybrid):
  - Post container: `<div class="post">` or `<tr class="post">` (root first, replies nested in `<table>` or `<div>`)
  - Post ID: `[No.]` link text; also `data-no` attribute on post element
  - Name/tripcode: text before `[No.]`; may show handle or "Anonymous"
  - Date (displayed): `MM/DD/YY (HH:MM:SS)` format (e.g. "07/26/26 (Sun) 09:42:04")
  - Date (raw epoch): not directly stored in visible text; epoch can be derived from date string if needed
  - Subject/comment: `<span class="subj">` for subject; `<div class="com">` or direct text for comment body
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `thumb/filename` or `thumb/filename.webp`
    - Filename: from the `src/` URL path (e.g. `1784761028001.jpeg`)
    - File size: e.g. "636.38 KB"
    - Dimensions: e.g. "3000x2385"
    - Format: JPEG, PNG, WebM, GIF
    - Spoiler flag: `spoiler` class on file input or label; `[Spoiler]` text in filename
  - Replies: nested under parent; each reply has `[No.]` link and `>>12345` cross-reference text in comment
  - OP vs reply differences: OP has `[Reply]` link at bottom; replies have `[Reply]` link and `>>parent_no` reference
- OP vs reply differences: sticky/locked not commonly used on 4chon; cycled threads show at bottom of page
- Sample fragment (trimmed):

```html
Anonymous07/26/26 (Sun) 09:42:04 [No.](4chon.me/new/res/11588.html#11593 "Reply to this post") [11593](4chon.me/new/res/11588.html#q11593 "Reply to this post")
Where is a description of this thing?

* * *

File: [1784761028001.jpeg](4chon.me/new/src/1784761028001.jpeg) (636.38 KB, 3000x2385)
[![](https://4chon.me/new/thumb/1784761028001.webp)](4chon.me/new/src/1784761028001.jpeg)

Anonymous07/22/26 (Wed) 18:57:11 [No.](4chon.me/new/res/11565.html#11565 "Reply to this post") [11565](4chon.me/new/res/11565.html#q11565 "Reply to this post")
new vichan software

if you have feature requests please leave them on github

8 posts and 1 image reply omitted. Click reply to view.

[>>11574](4chon.me/new/res/11565.html#11574) They're not zoomers, bro, they're just stupid.

[!!gBf2t4..OM![Character - Anime - Frieren](4chon.me/static/custom-flags/character-anime-frieren.webp)07/26/26 (Sun) 05:25:42 [No.](4chon.me/new/res/11565.html#11589 "Reply to this post") [11589](4chon.me/new/res/11565.html#q11589 "Reply to this post")
[>>11582](4chon.me/new/res/11565.html#11582)
Sometimes dead is better.
```

## 4. APIs & JSON feeds

- `?json=1` query param: **not supported** on 4chon — Tinyboard variant does not expose native JSON API
- Catalog JSON: not available via standard endpoints
- Preferred approach: HTML parsing — 4chon's structure is consistent enough for CSS selector-based scraping
- POST endpoint: `https://4chon.me/mod.php` — moderation only; no public API for content retrieval

## 5. Anti-bot & rate limiting

- Cloudflare protection on board/index pages (challenge page on first access or high-frequency requests)
- CAPTCHA reload button `[⟳]` visible on thread listing pages
- Required headers: standard User-Agent; `Referer` header recommended
- Rate limit behavior: 4chon is relatively permissive; 5 requests/second is safe; may see CAPTCHA at higher frequency

## 6. Pitfalls

- Lazy loading: thumbnails loaded on-demand via `src/...webp` — must wait for full page render or request `src/` directly
- Randomized class names: Tinyboard uses stable IDs (`thread`, `post`, `subj`, `com`) but may add per-render classes; rely on core classes
- Incomplete reply counts: "8 posts and 1 image reply omitted. Click reply to view" — must follow omit link or fetch subsequent pages
- Relative URLs: all file/thumbs URLs relative to `4chon.me`; resolve against board domain
- "Omitted posts" indicator: thread pages may have "[X] posts and [Y] images omitted" — must click to expand or accept gap
- Custom flags: 4chon supports user-defined flags via `static/custom-flags/` — may appear as `<img>` with custom class/filename
- Encoding: HTML entities common; `>`, `<` appear in comment text — decode when parsing

## 7. Key takeaways for scraper

- **Recommended approach:** HTML parsing with omit-follow — 4chon's Tinyboard structure is stable enough
- **Minimal request sequence:** fetch board page → extract thread IDs → fetch each thread page → follow "X posts omitted" links if present → parse posts
- **Throttling:** 2-3 requests/second is safe; pause if Cloudflare challenge appears
- **Reply graph reconstruction:** capture `>>11574` references as bare text; also look for `<a>` wrappers with href containing the No.
- **File extraction:** thumbnail URL → `4chon.me/new/thumb/...webp`; full size → `4chon.me/new/src/...jpeg`; resolve relative to `4chon.me`
- **Database mirror goal:** every post's No., name, timestamp (`MM/DD/YY (HH:MM:SS)`), subject, comment body, file URL (thumb + original), file size, dimensions, spoiler flag, reply cross-references (`>>12345`)
- **Thread state:** cycled threads at bottom of page; no sticky/locked commonly used
- **Omit handling:** when "X posts and Y images omitted" appears, must follow the link or re-fetch thread with page parameter to get full content