# Hacker News homepage snapshot

This directory contains a frozen capture of the public Hacker News homepage for
offline rendering and visual-fidelity work. It is not the output of gobrowser;
`docs/hackernews-reference.png` is a reference captured from Chromium.

- Source: <https://news.ycombinator.com/>
- HTML capture time: 2026-10-14T07:10:36Z
- CSS viewport: 1280 × 900 pixels
- Reference image: [`docs/hackernews-reference.png`](../../docs/hackernews-reference.png), captured full-page (1280 × 1209 pixels)
- Browser: Chromium 154, device scale factor 1

`index.html` is the captured response body. Its same-origin stylesheet,
JavaScript, favicon, spacer image, and CSS vote-arrow image are included beside
it. The snapshot has no external resource loads on initial page display; story
links and form actions are retained as captured, but should not be activated
when viewing the fixture.

## View the frozen capture

Open `index.html` directly in a browser with network access disabled. Relative
CSS and image references resolve to files in this directory. To regenerate the
browser reference image with Chromium and Playwright CLI:

```sh
npm exec --yes --package=playwright -- playwright screenshot \
  --channel chrome \
  --full-page \
  --viewport-size=1280,900 \
  --wait-for-timeout=1000 \
  "file://$(pwd)/testdata/hn-snapshot/index.html" \
  docs/hackernews-reference.png
```

The screenshot is a browser reference, not a claim that the current Go renderer
matches it. The current renderer does not yet support this page structure.

## Refresh the capture

To intentionally update the baseline, fetch the homepage and each required
same-origin resource into this directory, update the HTML capture time and
resource inventory here, and regenerate the full-page reference image at the
same viewport and device scale factor. Do not follow story links or load
third-party assets. Review the HTML/CSS references before adding resources;
the fixture should remain self-contained and must not require network access
to display.
