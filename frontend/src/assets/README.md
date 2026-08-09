# Frontend assets

- `icons/`: product-specific SVG or raster icons that are not available in the shared Lucide icon set.
- `logos/`: Grom product logos.
- `raw/`: source identity material that must not be imported by production UI code.

Prefer SVG for scalable interface artwork and use descriptive lowercase kebab-case filenames.
Import assets through the `@/assets/...` alias so Vite can fingerprint them during builds.
Production assets must have transparent backgrounds, be inspected at their rendered size, and be optimized before use.
Decorative crystals are reserved for brand compositions and dividers; do not place them inside buttons.
Do not place secrets, user uploads, or runtime-generated files here.
