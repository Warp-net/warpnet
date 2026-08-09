# Grimdark theme (WIP)

Fourth theme alongside light / dark / mastodon, in the same mechanism: a
`grimdark` class on `<html>`, a palette in `tailwind.config.js`, and a block of
scoped overrides at the end of `src/assets/tailwind.css`.

Enable it by hand until a UI switch exists:

    localStorage.setItem('theme', 'grimdark')   // then reload

## Files

- `frame.svg` — 9-slice ornamental frame (`border-image`, slice 32 of 96),
  applied to `.card`, `.modal-main` and the dashboard's `.rounded-lg.bg-lightest`
  panels.
- `preview-source.html` — component gallery markup, uses the app's own classes.
- `preview.html` — the same gallery with the compiled CSS inlined, openable
  standalone. Regenerate after theme edits:

      npx tailwindcss -c tailwind.config.js -i src/assets/tailwind.css \
        -o /tmp/demo.css --content src/assets/grimdark/preview-source.html

  then inline `/tmp/demo.css` into the page and replace the
  `url("./grimdark/frame.svg")` reference with a data URI of `frame.svg`.

## Gotchas hit while building this

- Theme rules must stay **outside** `@layer`. Tailwind drops custom layered
  styles whose selector classes never appear in the content files, and
  `grimdark` appears in no template yet — inside `@layer base` the whole theme
  vanished from the build. `dark` / `mastodon` survive only because those
  strings exist in component JS.
- The display font is applied via size utilities (`.text-2xl` etc.) because the
  views mark headings up with classes rather than `h1`-`h3`. Font Awesome icons
  carry the same utilities, so the selectors need `:not(i)` or every icon
  renders as tofu.

## Left to do

- Vendor Grenze Gotisch (OFL) into `src/assets/fonts` and drop the Google Fonts
  `@import` from `tailwind.css` — no external request from the app.
- Add a theme switch in `SideNav.vue` next to the existing light/dark toggle.
- Rebuild `frontend/dist` (separate commit) or the theme never reaches users.
- Remove `preview*.html` before the real PR if they are not wanted in-tree.

## Licensing

All artwork here is original: `frame.svg` is hand-authored, the checkbox glyphs
are inline data-URI SVG, the page grain is procedural `feTurbulence` (not a
scanned texture). The display font is OFL. No Games Workshop assets, marks or
derivatives are used — the theme borrows the genre (gothic, brass, rusted
iron), which is not protectable, not any specific imagery.
