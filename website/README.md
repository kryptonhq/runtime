# Krypton website

Source for [krypton.dev](https://krypton.dev). Built with
[Hugo extended](https://gohugo.io/) and the [Docsy](https://www.docsy.dev/)
theme (the same stack as kubernetes.io).

## Local development

```bash
# from repo root
make docs-serve     # http://127.0.0.1:1313 with live reload
```

Or directly:

```bash
cd website
npm install         # postcss + autoprefixer (first time only)
hugo server         # http://127.0.0.1:1313
```

## One-shot build

```bash
make docs           # → website/public/
```

## Adding pages

1. Create a Markdown file under `content/en/docs/<section>/<page>.md`
   with Hugo front matter:

   ```yaml
   ---
   title: My new page
   weight: 5
   description: One-line summary for nav + SEO.
   ---
   ```

2. Sidebar nav is auto-generated from the file tree and `weight:`. No
   central config to edit.

3. Cross-links use absolute paths so they survive directory moves:
   `[Components](/docs/architecture/components/)`.

## Vercel deploy

[`vercel.json`](../vercel.json) at the repo root tells Vercel to:

1. `cd website && npm install && hugo --minify`
2. Serve `website/public/`
3. Use Hugo `HUGO_VERSION` (extended is the default on Vercel's builders)

Point a Vercel project at the repo — no further config.
