# 4chan.org — Imageboard Analysis

> File: `4chan.org.md` · Analyzed: 2026-08-16 · Engine: vichan (4channel)

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://www.4chan.org |
| Name | 4chan |
| Boards | 70+ boards (full list at /boards/ — returns 404; boards are at boards.4chan.org) |
| Engine | vichan / custom 4chan fork |
| JS-rendered? | static HTML served by Go |
| robots.txt | not fetched |

## 2. Board (thread listing) page

- URL pattern: `https://boards.4chan.org/<board>/` (e.g. `https://boards.4chan.org/b/`)
- Pagination: `?page=N` appended to board URL; catalog at `https://boards.4chan.org/<board>/catalog`
- Thread card structure: each thread is a table row in the board index; thread ID is the post No. in the OP
- Exact selectors:
  - Thread container: `<tr class="thread">` (or `<div class="thread">` on some boards)
  - Thread ID (No.): `[No.]` link text or `data-no` attribute
  - Title: `<span class="subj">` within the thread row
  - OP post link: thread row clicks to `res/thread_id.html`
  - Reply count: `<span class="replies">` or last column text
  - File count: `<span class="files">` or last column text
- Sample fragment (trimmed):

```html
[No.] Anonymous08/16/26(Sun)11:08:53[No.](boards.4chan.org/b/thread/952683023#p952683023 "Link to this post") [952683023](boards.4chan.org/b/thread/952683023#q952683023 "Reply to this post")
File: [1772300396251763.jpg](i.4cdn.org/b/1786892933109351.jpg) (630 KB, 1792x2400)
[![630 KB](i.4cdn.org/b/1786892933109351s.jpg)] 630 KB JPG
cel aiAnonymous08/16/26(Sun)11:08:53[No.](boards.4chan.org/b/thread/952683023#p952683023 "Link to this post") [952683023](boards.4chan.org/b/thread/952683023#q952683023 "Reply to this post")[▶](boards.4chan.org/b/# "Post menu")
58 Replies / 33 Images [View Thread](boards.4chan.org/b/thread/952683023/cel-ai)
```

## 3. Thread page

- URL pattern: `https://boards.4chan.org/<board>/res/<thread_id>.html` or `https://boards.4chan.org/<board>/thread/<thread_id>`
- Post structure:
  - Post container: `<span class="post">` or `<div class="post">` (root post first)
  - Post ID: `[No.]` link text; also `data-no` attribute on the post element
  - Name/tripcode: text before `[No.]`, often "Anonymous" or tripcode `!!...`
  - Date (displayed): `MM/DD/YY(HH:MM:SS)` format
  - Date (raw epoch): 4chan stores posts with timestamp attributes; visible text is relative
  - Subject/comment: `<span class="com">` or directly in post body
  - File/image block: `<a href="...">` linking to `i.4cdn.org/<board>/<filename>`; thumbnail `s.4cdn.org/...s.jpg`
    - Filename: from the link URL path
    - File size: e.g. "630 KB"
    - Dimensions: e.g. "1792x2400"
    - Spoiler flag: `spoiler` class on file input/label
  - Replies: nested or inline; reply link format `>>12345` as bare text or within `<a>` tags
- OP vs reply differences: OP has `[Post menu]` button; replies have `[Reply]` link; sticky/locked flags shown on thread view
- Sample fragment (trimmed):

```html
[No.] Anonymous08/16/26(Sun)11:08:53[No.](boards.4chan.org/b/thread/952683023#p952683023 "Link to this post") [952683023](boards.4chan.org/b/thread/952683023#q952683023 "Reply to this post")
File: [1772300396251763.jpg](i.4cdn.org/b/1786892933109351.jpg) (630 KB, 1792x2400)
[![630 KB](i.4cdn.org/b/1786892933109351s.jpg)] 630 KB JPG

Anonymous08/16/26(Sun)11:08:53[No.](boards.4chan.org/b/thread/952683023#p952683023 "Link to this post") [952683023](boards.4chan.org/b/thread/952683023#q952683023 "Reply to this post")[▶](boards.4chan.org/b/# "Post menu")
>>952685899>>952685973>>952685855 Just a little more shine

Anonymous08/16/26(Sun)12:58:25[No.](boards.4chan.org/b/thread/952683023#p952685994 "Link to this post") [952685994](boards.4chan.org/b/thread/952683023#q952685994 "Reply to this post")
File: [IMG_1845.jpg](i.4cdn.org/b/1786899513779671.jpg) (68 KB, 796x1024)
[![68 KB](i.4cdn.org/b/1786899513779671s.jpg)] 68 KB JPG

>>952685912
```

## 4. APIs & JSON feeds

- `?json=1` query param: **not supported** on classic 4chan — HTML only
- Catalog JSON: not available via standard endpoints
- Preferred approach: HTML parsing (the board/index pages have consistent structure)
- POST endpoint: `https://api.4chan.org/` — not publicly documented; use at own risk

## 5. Anti-bot & rate limiting

- Cloudflare protection on board/index pages (challenge page on first access or high-frequency requests)
- No API token required for basic viewing
- Required headers: standard User-Agent; some boards may require referer
- Rate limit behavior: 4chan is relatively permissive but aggressive scraping may trigger CAPTCHA

## 6. Pitfalls

- Lazy loading: not used — all threads rendered in HTML
- Randomized class names: 4chan uses stable class names (`thread`, `post`, `file`, etc.) — reliable for selectors
- Incomplete reply counts: on thread pages with many replies, "58 Replies / 33 Images" may include omitted replies; always fetch full thread to get accurate count
- Relative URLs: all file/thumbs URLs are relative to `i.4cdn.org` or `s.4cdn.org`; always resolve against board domain
- "Omitted posts" indicator: "55 replies and 31 images omitted. Click here to view" — must follow link to expand
- Encoding: HTML entities common (e.g., `>`, `<`) — decode when parsing

## 7. Key takeaways for scraper

- **Recommended approach:** HTML parsing — 4chan's structure is stable and well-documented
- **Minimal request sequence:** fetch board page → extract thread IDs → fetch each thread page (`res/ID.html`) → parse posts
- **Throttling:** 2-3 requests/second is safe; pause if Cloudflare challenge appears
- **Reply graph reconstruction:** capture `>>12345` references as bare text; also look for `<a>` wrappers
- **File extraction:** thumbnail URL → `s.4cdn.org/...s.jpg`; full size → `i.4cdn.org/...jpg`; resolve relative to `i.4cdn.org`
- **Database mirror goal:** every post's No., name, trip, timestamp (epoch conversion from `MM/DD/YY HH:MM:SS`), subject, comment body, file URL (thumb + original), file size, dimensions, spoiler flag
- **Thread state:** sticky/locked flags appear on thread index; archived threads have `[archive]` tag