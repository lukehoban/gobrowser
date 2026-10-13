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

This renderer targets Hacker News story tables: ranked `.athing` rows with `.titleline` titles and optional `.sitebit` domains, followed by `.subtext` metadata (`.score`, `.hnuser`, `.age`, and comments links). It draws a fixed Hacker News-inspired layout with an orange navigation bar, ranked titles, domains, and metadata using an embedded Go font.

For story titles, it reads embedded `<style>` rules and inline styles using simple descendant selectors (tags, classes, and IDs). It applies `color` (basic named colors, `#rgb`, `#rrggbb`, and `rgb()`) and `font-size` in `px` or `pt`, clamped to 6–32 CSS pixels.

It does not implement general CSS parsing or cascading, external stylesheets, arbitrary page layout, images, or interactive browser behavior. See [issue #5](https://github.com/lukehoban/gobrowser/issues/5) for the supported CSS slice.
