# gobrowser

![Example Hacker News page rendered to PNG](docs/hackernews.png)

Example generated from the bundled HTML fixture.

## Run

Render the live Hacker News homepage:

```sh
go run . -out hackernews.png
```

Render a saved HTML page:

```sh
go run . -input testdata/hackernews.html -out hackernews.png
```

Options:

- `-url`: page URL to fetch (default: `https://news.ycombinator.com/`)
- `-input`: saved HTML file; when set, it is used instead of `-url`
- `-out`: PNG output path (default: `hackernews.png`)
- `-width`: image width in pixels, from 640 to 4096 (default: `1000`)
- `-stories`: maximum number of stories, from 1 to 30 (default: `30`)

## Supported page structure

This renderer targets Hacker News story tables: ranked `.athing` rows with `.titleline` titles and optional `.sitebit` domains, followed by `.subtext` metadata (`.score`, `.hnuser`, `.age`, and comments links). It draws a fixed Hacker News-inspired layout with an orange navigation bar, ranked titles, domains, and metadata using an embedded Go font. Embedded `<style>` rules and inline styles can set story-title `color` and `font-size`; selectors support tag, ID, and class components with descendant and child combinators. Color accepts common names, hex, and integer `rgb()` values. Font sizes accept `px`, `pt`, `em`, `rem`, and `%`, bounded to 1–64 pixels.

CSS support is intentionally limited to those story-title declarations; external stylesheets, general page layout, images, and interactive browser behavior are not implemented. See [issue #4](https://github.com/lukehoban/gobrowser/issues/4) for the initial rendering slice and [issue #5](https://github.com/lukehoban/gobrowser/issues/5) for focused CSS support.
