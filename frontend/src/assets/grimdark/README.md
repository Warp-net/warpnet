# Grimdark — the dark theme

This *is* the app's dark theme: blackened iron, bone text, brass trim, dried
blood as the accent. There is no separate class or switch — the existing
light/dark toggle in `SideNav.vue` turns it on, and `darktheme` in
`tailwind.config.js` now holds this palette (the key name stayed so the
`dark:bg-darktheme-card` utilities in the views keep working).

The mastodon theme and the light theme are untouched.

## Where it lives

- `tailwind.config.js` — the `darktheme` palette, plus a `gold` key for trim.
- `src/assets/tailwind.css` — the existing `.dark` mappings carry most of the
  theme once the palette changes; below the base layer sit the classes that had
  no dark mapping at all (`bg-lightblue`, `border-dark`, the brand blue) and the
  ornamental layer.
- `frame.svg` — 9-slice ornamental frame (`border-image`, slice 32 of 96),
  applied to `.card`, `.modal-main` and the dashboard's `.rounded-lg.bg-lightest`
  panels.
- `preview-source.html` / `preview.html` — a component gallery; the second has
  the compiled CSS inlined and opens standalone. Regenerate after theme edits:

      npx tailwindcss -c tailwind.config.js -i src/assets/tailwind.css \
        -o /tmp/demo.css --content src/assets/grimdark/preview-source.html

  then inline `/tmp/demo.css` into the page and replace the
  `url("./grimdark/frame.svg")` reference with a data URI of `frame.svg`.

## Gotchas

- Rules for a class that appears in no template are dropped from the build when
  written inside `@layer`. `.dark` is safe (it occurs in `SideNav.vue`), but
  anything genuinely new must stay unlayered — that is why the ornamental block
  sits outside the layer.
- The display font is applied via size utilities (`.text-2xl` etc.) because the
  views mark headings up with classes rather than `h1`-`h3`. Font Awesome icons
  carry the same utilities, so the selectors need `:not(i)` or every icon
  renders as tofu.

## Left to do

- Vendor Grenze Gotisch (OFL) into `src/assets/fonts` and drop the Google Fonts
  `@import` from `tailwind.css` — as shipped, every load hits a Google CDN.
- Rebuild `frontend/dist` (separate commit) or the theme never reaches users.
- Remove `preview*.html` before the real PR if they are not wanted in-tree.

## Licensing

All artwork here is original: `frame.svg` is hand-authored, the checkbox glyphs
are inline data-URI SVG, the page grain is procedural `feTurbulence` (not a
scanned texture). The display font is OFL. No Games Workshop assets, marks or
derivatives are used — the theme borrows the genre (gothic, brass, rusted
iron), which is not protectable, not any specific imagery.
