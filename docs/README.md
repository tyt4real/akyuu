# Imageboard Scraper Analysis

Recon folder for building scrapers for imageboard-style websites.

## Contents

- One markdown file per analyzed website: `<domain>.md` (e.g. `4chan.org.md`)
- `_TEMPLATE.md` — the section layout every analysis follows

## How the analyses were produced

- Raw HTML fetched via `curl` (source of truth for selectors/structure)
- Browser inspection for JS-rendered content, redirects, anti-bot checks
- Board + thread pages inspected at the DOM level; class names, IDs, and
  data-attributes captured verbatim
- JSON/API endpoints sniffed from the network layer where present
- Only structure is documented — no scraped content is stored

## What each analysis contains

- Site identity + board list (with URLs)
- Engine identification (vichan, LynxChan, TinyIB, danbooru-style, custom...)
- Board (thread listing) page: URL pattern, pagination, thread card structure,
  exact selectors
- Thread page: OP + reply structure, post IDs, timestamps, file/image markup,
  reply cross-references (`>>12345`), exact selectors
- File/image extraction: thumbnail + full-size URL patterns, filename,
  size, dimensions, spoiler flags
- Post metadata: name, tripcode, subject, capcode, flag, timestamps (displayed
  + raw epoch), ID-based hash
- Thread state: sticky, locked, cycled, archived, deleted flags
- API endpoints (JSON feeds, ?json=1, /api/... routes) — preferred over HTML
- Anti-bot measures (Cloudflare, captchas, rate limits) + observed behavior
- Pitfalls (lazy loading, randomized class names, missing reply counts, etc.)
- Sample HTML fragments that pin down the structure

## Handover notes for the scraper agent

**Goal:** full-content mirror — every post body, every file, reply graph intact,
suitable for re-hosting in a vichan-style frontend grouped by site → board → thread.

- Verify selectors against a live page before trusting them — boards are
  frequently modified
- Respect `robots.txt` and be polite: throttle requests, use a real User-Agent
- Many engines ship a JSON API — prefer it over HTML parsing when available
- Timestamps are often stored as Unix epoch in markup attributes even when the
  visible text is human-readable
- Reply links (`>>12345`) may be bare text or wrapped in `<a>` — capture both
  forms if reconstructing the reply graph
- File URLs may be relative — always resolve against the board or site root
- Spoiler images often share a thumbnail path but have a separate full-size URL
- "Omitted posts" on board pages need a thread-page fetch to expand — don't
  trust listing-page reply counts as ground truth
