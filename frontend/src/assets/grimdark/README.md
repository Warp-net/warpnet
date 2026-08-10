# Grimdark — both app themes

Both themes are grimdark now, and there is no separate class or switch: the
existing light/dark toggle in `SideNav.vue` picks between them.

- **Dark** — blackened iron, bone text, brass trim, dried blood accent.
- **Light** — aged parchment, sepia ink, bronze trim (brass has no contrast on
  a pale ground), same blood accent.

The mastodon theme is untouched.

## Where it lives

- `tailwind.config.js` — the `darktheme` palette for dark, and the top-level
  brand slots (`blue`, `lightblue`, `lighter`, `lightest`, `paper`, …) for
  light. `blue` has been the primary accent slot since the Twitter-clone days,
  so it now holds blood red rather than magenta.
- `src/assets/tailwind.css` — the existing `.dark` mappings carry the dark
  theme once its palette changes; below the base layer sit the classes that had
  no dark mapping at all (`bg-lightblue`, `border-dark`, the brand blue) and the
  ornamental layer.
- The ornamental layer is **shared** by both themes. Everything that differs
  comes from custom properties set on `:root` and `.dark`: `--gd-frame`,
  `--gd-trim`, `--gd-plate`, `--gd-outline-text`, `--gd-logo`.
- `frame.svg` / `frame-light.svg` — 9-slice ornamental frames (`border-image`,
  slice 32 of 96), applied to `.card`, `.modal-main` and the dashboard's
  `.rounded-lg.bg-lightest` panels. Same geometry, brass vs bronze.
- `../fonts/` — the self-hosted display font (see below).
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

## The display font

Grenze Gotisch is self-hosted in `src/assets/fonts` — the app makes no
third-party request. It is a variable font, so one file per subset covers every
weight; latin and latin-ext are vendored (73 KB together), vietnamese is not.
The family has **no Cyrillic**, so Russian headings fall back to the serif
stack — that is expected, not a bug. `GrenzeGotisch-OFL.txt` sits beside the
woff2 as the license requires.

## Left to do

- Rebuild `frontend/dist` (separate commit) or the themes never reach users.
- Remove `preview*.html` before the real PR if they are not wanted in-tree.

## Licensing

All artwork here is original: the frames are hand-authored, the checkbox glyphs
are inline data-URI SVG, the page grain is procedural `feTurbulence` (not a
scanned texture). The display font is OFL. No Games Workshop assets, marks or
derivatives are used — the theme borrows the genre (gothic, brass, rusted
iron), which is not protectable, not any specific imagery.
