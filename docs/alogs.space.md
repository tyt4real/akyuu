# alogs.space — Imageboard Analysis

> File: `alogs.space.md` · Analyzed: 2026-08-16 · Engine: LynxChan

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://alogs.space |
| Name | Alogs Space |
| Boards | /cow/ - Lolcows, /robowaifu/ - DIY Robot Wives, /ita/ - Italian Section, /kong/ - Kong, /sw/ - Star Wars |
| Engine | LynxChan 2.9.2 (confirmed by footer `LynxChan 2.9.2` and post structure) |
| JS-rendered? | Partial — board pages server-rendered with AJAX enhancements |
| robots.txt | not fetched |

## 2. Board (thread listing) page

- URL pattern: `https://alogs.space/<board>/` (e.g. `https://alogs.space/cow/`, `https://alogs.space/robowaifu/`)
- Pagination: `?page=N` appended to board URL; also overboard at `https://alogs.space/overboard/`
- Thread card structure: LynxChan-style thread list with OP preview, title, reply/file counts
- Exact selectors (from extracted cow/ board page):
  - Thread container: `.thread` class or `<div class="res">` within board index
  - Thread ID (No.): `[No.]` link text within thread item; also in URL hash
  - Title: `.subject` or `<h2>` within thread header
  - OP post preview: first post in thread preview shows name, date, truncated comment
  - Reply count: `.replies` span or text within thread meta
  - File count: `.files` span or text within thread meta
- Sample fragment (from /cow/ extraction):

```html
[>>/robowaifu/45052](alogs.space/robowaifu/res/43642.html#45052) >>45049
But /shelter/ looks completely dead.
Anyway, news from https://my.frantech.ca/:
>Down for Maintenance (Err 3)
>We ex

[>>/robowaifu/45051](alogs.space/robowaifu/res/43642.html#45051) >>45050
Feel free to post about it here if you'd care to Anon.

[>>/robowaifu/45050](alogs.space/robowaifu/res/43642.html#45050) >>45044
>trashchan is down again

[>>/robowaifu/45049](alogs.space/robowaifu/res/43642.html#45049) >>45047
Also, just any FYI there's a WebRing -wide rallying point:
https://junkuchan.org/shelter/index.html

/shelter/ got i

[>>/cow/289960](alogs.space/cow/res/115132.html#289960) >>289958
Daniel Lopez pretending to be an unfunny Podawful butt boy btw

[>>/cow/289958](alogs.space/cow/res/115132.html#289958) >>289957
You got me, Danny. I should not have appealed to posts that were 2 years late. But it's not neceesary to share Blockla

[>>/cow/289957](alogs.space/cow/res/115132.html#289957) >>289956
Someone shit up a thread and got a temporary thread ban? That's doesn't sit right with me either, especially two years

[>>/robowaifu/45047](alogs.space/robowaifu/res/43642.html#45047) >>45046
They don't necessarily have to use the same security system, maybe trashchan was just put on some blacklist used my mul

[>>/robowaifu/45046](alogs.space/robowaifu/res/43642.html#45046) >>45044
BTW, IIRC an Anon on Trash said that he had the exact same issue as ( >>44972 ), and he was on Xfinity. Apparently they

[>>/robowaifu/45044](alogs.space/robowaifu/res/43642.html#45044) >>45044
```

## 3. Thread page

- URL pattern: `https://alogs.space/<board>/res/<thread_id>.html` (e.g. `https://alogs.space/robowaifu/res/43642.html`)
- Post structure (LynxChan 2.9.2):
  - Post container: `<div class="post">` or `<article class="post">` (root first, replies nested)
  - Post ID: `[No.]` link text; also visible in URL hash and post header
  - Name/tripcode: text before `[No.]`; may show user handle or tripcode `name!ident@host`
  - Date (displayed): `YYYY-MM-DD HH:MM:SS` format; also relative time
  - Date (raw epoch): LynxChan stores Unix epoch in `data-created` attribute
  - Subject/comment: `<div class="subject">` + `<div class="comment">`; comment may contain formatting (italics, bold, code, spoilers, quotes, `>>` references)
  - File/image block: `<a href="...">` linking to `src/filename`; thumbnail `r/thumb/filename`; embedded WebM/MP4 video
    - Filename: from the `src/` URL path
    - File size: e.g. "KB"
    - Dimensions: e.g. "WxH"
    - Format: GIF, WebM, MP4, JPG, PNG
    - Spoiler flag: `spoiler` class on file link or label
  - Replies: nested under parent post; each reply has its own `[No.]` link and cross-references `>>12345` as text within comment body
  - OP vs reply: OP has `[Watch Thread]` and `[Reply]` buttons at top; replies have `[Reply]` below each post
- OP vs reply differences: sticky/locked not commonly discussed; tripcode `name!ident@host` format captured; cross-board references to /shelter/, /trashchan/, etc.
- Sample fragment (trimmed):

```html
[No.] Cat Mother2026-08-16 09:55:46 [No.](alogs.space/cl/res/49657.html#49659) [49659](alogs.space/cl/res/49657.html#q49659) [Reply](alogs.space/cl/res/49657.html)
File: [1785932633380-0.jpg](alogs.space/cl/src/1785932633380-0.jpg)(76.21 KB, 720x720, 1:1, [68a420d643ad189e793a1c8a97....jpg](alogs.space/cl/src/1785932633380-0.jpg "Save as original filename (68a420d643ad189e793a1c8a976fdb0e.jpg)"))
[![](https://alogs.space/cl/thumb/1785932633380-0.png)](alogs.space/cl/src/1785932633380-0.jpg)
Fukc The Web (FTW) movementJohn Rain2026-08-05 05:23:55 [No.](alogs.space/cl/res/49657.html#49657) [49657](alogs.space/cl/res/49657.html#q49657) [Reply](alogs.space/cl/res/49657.html)
If you are looking to help start a new image board community then consider trying 3chan dot net...
>>Acid Burn2026-08-05 05:30:56 [No.](alogs.space/cl/res/49556.html#49557) [49557](alogs.space/cl/res/49556.html#q49557)
[>>49556](alogs.space/cl/res/49556.html#49556)
for logo use krita guess
ftw i guess
```

## 4. APIs & JSON feeds

- Native JSON API: **yes** — alogs.space LynxChan serves `?json=1` on board and thread pages
- JSON API URL: `https://alogs.space/<board>/?json=1` or `https://alogs.space/<board>/res/<id>.json`
- Example response shape: JSON object with `posts` array; each post has `no`, `com`, `sub`, `com_num`, `filename`, `filesize`, `ext`, `w`, `h`, `tim`, `md5`, `sub`, `com`, `capcode`, `resto`, `last_mod`, `flag`, `tim`
- Preferred approach: **use JSON API** — much cleaner than HTML parsing; reply graph is `resto` field (0 for OP, parent post No. for replies)
- Other endpoints: `/overboard/` — cross-board view

## 5. Anti-bot & rate limiting

- Cloudflare challenge on first access or high-frequency requests (per-board)
- No API key needed for basic JSON; some endpoints may have per-board limits
- Required headers: standard User-Agent; `Accept: application/json` for JSON endpoints
- Rate limit behavior: ~10 requests/sec per board before CAPTCHA appears; LynxChan's built-in throttle

## 6. Pitfalls

- JS-rendered content: board listing requires headless rendering or API call; HTML extraction alone gets skeleton only
- Randomized class names: LynxChan uses CSS modules — class names may change between versions; prefer data-attributes or JSON API
- Incomplete reply counts: board page may show truncated reply counts; thread page JSON has accurate `com_num`
- Relative URLs: file URLs are relative to `alogs.space`; always resolve against base
- "Omitted posts": LynxChan shows "X posts and Y images omitted" — must fetch full thread or use JSON to get full count
- Tripcodes: `name!ident@host` format — capture separately if needed for mirroring
- Cross-board references: frequent references to /shelter/, /trashchan/, /junkuchan/ — these are related imageboards; capture if building aggregator mirror
- Encoding: UTF-8; no special HTML entities observed beyond standard formatting

## 7. Key takeaways for scraper

- **Recommended approach:** LynxChan JSON API — fetch `?json=1` for board listing and thread pages; far more reliable than HTML parsing
- **Minimal request sequence:** fetch board `?json=1` → extract thread IDs → fetch each thread `?json=1` → parse posts from JSON
- **Throttling:** 5 requests/second per board is safe; alogs.space's API is more permissive than HTML rendering
- **Reply graph reconstruction:** JSON `resto` field = 0 for OP, parent post No. for replies; `com_num` gives accurate reply count
- **File extraction:** `filename` + `ext` + `w` + `h` + `filesize` from JSON; thumbnail at `r/thumb/{filename}`; full at `src/{filename}`; `country` from `data-country` attribute if present
- **Database mirror goal:** every post's No., name/trip, timestamp (Unix epoch from `tim` + displayed `YYYY-MM-DD HH:MM:SS`), subject (`sub`), comment body (`com`), file URL (thumb + original + dimensions + format), spoiler flag, `resto` for reply graph, cross-board references (/shelter/, /trashchan/ etc.)
- **Thread state:** sticky/locked from thread metadata in JSON; `/overboard/` for cross-board aggregator mirror
- **Related boards:** frequent references to /shelter/, /trashchan/, /junkuchan/ — these are related imageboards in the same ecosystem; capture board names and URLs if building aggregated database
- **Language:** English-only board content observed; some Italian (/ita/) boards may have non-UTF8 encoding edge cases