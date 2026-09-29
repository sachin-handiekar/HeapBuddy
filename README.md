# HeapBuddy — project site

This branch (`gh-pages`) hosts the static landing page for
[HeapBuddy](https://github.com/sachin-handiekar/HeapBuddy), served by GitHub Pages.

- `index.html` — the whole site (inline CSS/JS, no build step)
- `assets/` — screenshots copied from `docs/screenshots/` on `main`
- `.nojekyll` — serve files as-is

To preview locally: `python -m http.server` in this directory, then open http://localhost:8000.

Enable it under **Settings → Pages → Deploy from a branch → `gh-pages` / root**.
