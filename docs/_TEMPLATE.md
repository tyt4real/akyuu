# <Domain> — Imageboard Analysis

> File: `<domain>.md` · Analyzed: <date> · Tools: curl + browser

## 1. Site overview

| Field | Value |
|---|---|
| URL | https://... |
| Name | |
| Boards | count + short list w/ URLs |
| Engine | e.g. vichan / LynxChan / TinyIB / danbooru / custom |
| JS-rendered? | static HTML / client-side JS / hybrid |
| robots.txt | allow/disallow summary |

## 2. Board (thread listing) page

- URL pattern: `https://site/<board>/` (+ index variants like `index.html`, `0.html`, `?page=N`)
- Pagination: how it works, max pages, URL scheme
- Thread card structure: enclosing element, thread ID location, title, OP preview,
  reply/file counts, sticky/archived/deleted flags
- Exact selectors:
  - Thread container: `...`
  - Thread ID: `...`
  - Title: `...`
  - OP post link / href pattern: `...`
  - Reply count: `...`
  - File count: `...`
  - Sticky / locked / archived indicators: `...`
- Sample fragment (trimmed):

```html
...
```

## 3. Thread page

- URL pattern: `https://site/<board>/thread/<id>` (+ variants: `/<id>.html`, trailing slug, `?json=1`)
- Post structure:
  - Post container: `...`
  - Post ID: `...`
  - Name/tripcode/capcode: `...`
  - Country flag (if 4chan-style): `...`
  - Date (displayed + raw epoch): `...`
  - Subject/comment: `...` (with reply link format)
  - File/image block: `...`
    - Thumbnail URL: `...`
    - Full-size URL: `...`
    - Filename (original + renamed): `...`
    - File size, dimensions, spoiler flag
- OP vs reply differences (banners, sticky, locked, `[BUMP]` etc.)
- Reply cross-reference format: inline `<a href="...">` vs bare `>>12345`
- Sample fragment (trimmed):

```html
...
```

## 4. APIs & JSON feeds

- Native JSON API? URL + example response shape
- `?json=1` or similar query param support
- Catalog JSON: URL + structure
- Other endpoints (recent, search, post-by-ID)

## 5. Anti-bot & rate limiting

- Cloudflare / challenge / captcha observed?
- Rate limit behavior (HTTP codes, headers)
- Required headers/cookies for baseline access

## 6. Pitfalls

- Lazy loading / infinite scroll
- Randomized or per-request class names
- Incomplete reply counts on board pages
- Encoding issues
- Relative URLs for files/posts
- Anything else that bit during analysis

## 7. Key takeaways for scraper

- Recommended approach: API vs HTML
- Minimal request sequence to get all posts of a board (full content)
- Throttling recommendation
- Reply-graph reconstruction notes
