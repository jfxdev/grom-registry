# Grom visual implementation plan

This plan translates `docs/visual-identity.md` into an incremental frontend
delivery. It defines scope, file ownership, interaction behavior, validation,
and approval gates.

## Implementation status

Implemented on July 26, 2026:

- Added the compact transparent crest at
  `frontend/src/assets/logos/grom-crest.png`.
- Added the production favicon at `frontend/public/favicon.png`.
- Added the code-native divider crystal at
  `frontend/src/assets/icons/crystal-green.svg`.
- Added shared brand components under
  `frontend/src/shared/components/brand`.
- Migrated the global theme from neon green to semantic obsidian, moss, ivory,
  bronze, warning, and danger tokens.
- Implemented raised buttons that move down through `transform` and collapse
  their contact shadow when pressed.
- Kept crystals out of every button.
- Rebuilt sign-in for desktop and mobile, including password visibility,
  loading semantics, and a borderless mobile layout.
- Applied the quieter token system to the application shell and existing pages.
- Added an accessible mobile navigation drawer with an exposed backdrop,
  changing labels and expanded state, link dismissal, and Escape dismissal.
- Added focused tests for Button, Input, and sign-in behavior.
- Validated the sign-in screen at `1440 × 900`, `390 × 844`, and `320 × 568`.
- Validated the authenticated Projects, Users, Service Accounts, and
  Integrations views on desktop, plus the Projects shell and drawer on mobile.

The implementation uses one 256px crest source rather than separate `@2x`
files. This source covers the approved rendered sizes while keeping the
production bundle smaller.

## Remaining work to complete the plan

The visual foundation and primary propagation are implemented, but the plan is
not complete until the following steps are finished in order. A later step must
not be marked complete while an earlier step still has an unresolved acceptance
failure.

Gate snapshot on July 28, 2026:

- `make test` passes, including backend tests, frontend lint, 23 frontend tests,
  and TypeScript checking.
- `make build` passed on July 29, 2026.
- The documented browser review covers sign-in at `1440 × 900`, `390 × 844`,
  and `320 × 568`; authenticated pages were reviewed on desktop, with the
  Projects shell and drawer also reviewed on mobile.
- The complete responsive, accessibility, touch-target, focus-management, and
  semantic-token acceptance matrix below still needs to be closed.

### Completion step 1: meet mobile touch-target requirements

Primary files:

- `frontend/src/shared/components/ui/button/Button.vue`
- `frontend/src/app/App.vue`
- authenticated pages and modals that use `size="sm"` or `size="icon"`

Work:

1. Inventory every interactive control rendered at widths up to `800px`,
   including header, drawer, table-row, modal, tag, deletion, lifecycle,
   policy, and access-key actions.
2. Keep the current compact desktop dimensions where appropriate.
3. At touch breakpoints, give icon buttons a minimum `48 × 48px` interactive
   area without enlarging their icons.
4. Give small text buttons a minimum height of `48px` when they are expected to
   be used directly on touch layouts.
5. Confirm that adjacent actions remain visually separated and do not overlap
   at `320px`.
6. Preserve the existing relief depth and pressed translation without causing
   layout reflow.

Acceptance:

- Every visible mobile icon button has a measured interactive box of at least
  `48 × 48px`.
- Every primary mobile form action is at least `48px` high.
- No action overlaps, clips, or forces horizontal page scrolling at `320px`.
- Desktop tables and headers retain their intended compact density.

Validation:

- Inspect computed element dimensions at `320 × 568`, `390 × 844`, and
  `768 × 1024`.
- Exercise every affected action with touch emulation and keyboard focus.

### Completion step 2: complete mobile drawer focus management

Primary file:

- `frontend/src/app/App.vue`

Work:

1. Keep a template reference to the menu trigger.
2. When the drawer opens, move focus to the first meaningful drawer action.
3. Keep keyboard focus inside the open drawer while it behaves as an overlay.
4. Make the obscured application workspace non-interactive to keyboard and
   assistive technology while the drawer is open.
5. Continue closing on backdrop click, Escape, and navigation.
6. After closing, return focus to the menu trigger unless navigation moved focus
   to a new page.
7. Prevent hidden drawer controls from entering the tab order.
8. Preserve the current `aria-controls`, changing `aria-expanded`, and changing
   accessible label.

Acceptance:

- Opening the drawer produces a predictable visible focus location.
- Tab and Shift+Tab cannot escape into obscured page content.
- Escape closes the drawer and returns focus to the trigger.
- Backdrop and link dismissal still work.
- Screen readers do not navigate the obscured workspace as though it were
  active.

Validation:

- Add a focused shell test for open, close, Escape, link dismissal, focus
  containment, and focus return.
- Repeat the flow manually using keyboard only at `320px` and `390px`.

### Completion step 3: finish semantic-token propagation

Primary files:

- `frontend/src/app/styles/main.css`
- `frontend/src/shared/components/ui/button/Button.vue`
- `frontend/src/shared/components/ui/badge/Badge.vue`
- files listed under Phase 7
- repository creation and policy modals
- reset-password and user-profile pages

Work:

1. Search Vue and CSS production files for hard-coded hexadecimal, `rgb()`, and
   `rgba()` values.
2. Classify each occurrence as:
   - semantic color that should use a shared token;
   - structural alpha used only for shadow, overlay, or translucency;
   - intentionally local brand artwork color.
3. Add narrowly named semantic tokens only when an existing token cannot express
   the meaning.
4. Replace hard-coded success, warning, danger, accent, text, border, and
   reusable surface colors.
5. Keep local alpha values only for non-semantic shadows and overlays.
6. Confirm that ruby brand accents are not reused as destructive status colors.
7. Re-run the search and document any intentional remaining hard-coded values
   beside the owning component when their purpose is non-obvious.

Acceptance:

- Shared status colors in Badge and Button come from semantic tokens.
- Repeated moss, ivory, warning, and danger values are not duplicated across
  pages.
- Page surfaces, modals, tabs, cards, and empty states use shared surface and
  border tokens.
- The resulting interface retains the approved obsidian, moss, ivory, bronze,
  warning, and danger relationships.

Validation:

- Run a repository search for hard-coded production colors.
- Inspect success, warning, danger, disabled, hover, active, and focus states in
  the browser.
- Run frontend lint, tests, and type checking.

### Completion step 4: close the planned automated-test gaps

Primary files:

- `frontend/src/shared/components/ui/button/Button.test.ts`
- `frontend/src/shared/components/ui/input/Input.test.ts`
- `frontend/src/modules/auth/pages/SignInPage.test.ts`
- a shell test for `frontend/src/app/App.vue`

Work:

1. Add Button coverage for:
   - the independent `disabled` prop;
   - `default`, `outline`, `ghost`, and `danger` variants;
   - default, small, and icon size modifiers;
   - loading `aria-busy` and duplicate-submission prevention.
2. Add Input coverage for:
   - disabled attribute forwarding;
   - invalid ARIA state;
   - trailing action rendering;
   - model updates.
3. Add sign-in coverage for:
   - edited email and password bindings;
   - password visibility and changing label;
   - a pending authentication promise and loading state;
   - duplicate-submit prevention;
   - error rendering and stale-error clearing;
   - configured redirect and default redirect.
4. Add application-shell coverage for the drawer behavior defined in completion
   step 2.
5. Keep physical press travel as a browser-validation concern rather than
   attempting to prove CSS rendering in unit tests.

Acceptance:

- Every primitive-test item in Phase 3 has a focused assertion.
- Every sign-in test item in Phase 5 has a focused assertion.
- Drawer keyboard behavior has automated regression coverage.
- Tests assert user-visible and accessibility behavior rather than internal
  implementation details where practical.

Validation:

```text
cd frontend && npm run lint
cd frontend && npm test
cd frontend && npm run typecheck
```

### Completion step 5: execute the full responsive browser matrix

Pages and surfaces:

- Sign in.
- Password reset.
- Projects list and project detail.
- Repository creation and policy modals.
- Repository tag, deletion, and lifecycle flows.
- Project membership flow.
- User list, reset-link modal, and profile.
- Service-account list and access-key panel.
- Integrations.
- Authenticated application shell and drawer.

Work:

1. Test every listed page at:
   - `320 × 568`;
   - `390 × 844`;
   - `768 × 1024`;
   - `1280 × 800`;
   - `1440 × 900`.
2. At each applicable viewport, check empty, populated, loading, error, modal,
   and destructive-confirmation states.
3. At `320px`, confirm there is no unintended horizontal page scrolling.
4. At short viewport heights, confirm forms and modals remain reachable through
   scrolling.
5. Test long validation and API error messages.
6. Test sign-in with a deliberately slow authentication response.
7. Repeat representative flows at 200% browser zoom.
8. Repeat representative flows with reduced motion enabled.

Acceptance:

- No content or focus target is clipped or unreachable.
- Tables and rows collapse deliberately rather than overflowing.
- Modals fit the viewport and scroll internally where required.
- Sign-in remains usable with a small viewport height and on-screen keyboard.
- Mobile composition remains deliberate rather than a scaled desktop layout.
- Reduced-motion mode removes animated travel without removing state feedback.

Evidence:

- Record the date and result for every viewport.
- Capture representative desktop, tablet, and mobile screenshots after visual
  approval.
- Store screenshot baselines only if the approved visual state is stable enough
  to avoid noisy iteration.

### Completion step 6: close accessibility acceptance

Work:

1. Measure WCAG AA contrast for body text, muted text, status badges, error
   text, controls, borders required to identify controls, and focus indicators.
2. Navigate every page and modal using only keyboard.
3. Confirm every interactive element has a visible focus indicator.
4. Confirm pressed button travel never clips or hides the focus outline.
5. Confirm errors and statuses use text or icons in addition to color.
6. Confirm decorative crest and crystal elements do not create duplicate
   announcements.
7. Confirm the password visibility action exposes its changing label.
8. Confirm loading state is announced and prevents duplicate submission.
9. Confirm modal and drawer focus entry, containment, dismissal, and return.
10. Repeat the critical flows at 200% zoom.

Acceptance:

- No known WCAG AA contrast failure remains.
- Every supported flow can be completed with keyboard only.
- Focus order follows the visible reading and interaction order.
- Overlay focus never escapes to obscured content.
- Meaning is not communicated by color alone.
- Decorative brand elements remain hidden from assistive technology unless they
  intentionally carry alternative text.

Evidence:

- Record manual findings and resolutions in the handoff or pull request.
- Capture any intentional exception with its reason and follow-up owner; do not
  silently waive a failed criterion.

### Completion step 7: run final gates and close the plan

Work:

1. Run the full automated test gate.
2. Run the production build gate.
3. Inspect the generated production application, not only the Vite development
   server.
4. Confirm the embedded frontend contains the production crest and favicon.
5. Re-run the responsive and keyboard smoke checks against the production
   output.
6. Update the implementation-status section with the completion date and final
   validation evidence.
7. Remove or rewrite this remaining-work section once every acceptance item is
   complete; do not leave completed work described as pending.

Required commands:

```text
make test
make build
```

Acceptance:

- Both commands exit successfully.
- Generated OpenAPI types remain fresh.
- The production frontend loads through the Go application.
- No authentication, navigation, or management behavior regressed.
- Documentation accurately distinguishes completed validation from pending
  follow-up work.

## Goal

Implement the approved Grom identity across the Vue application while preserving
the behavior, routes, domain language, and API integration of the current MVP.

The first implementation slice is the sign-in experience. The visual system is
propagated to the authenticated application only after desktop and mobile sign-in
have been reviewed in the browser.

## Confirmed decisions

- The product remains a modern developer tool with restrained RPG details.
- The sign-in page uses a centered form surface on desktop and a borderless
  single column on mobile.
- The large Grom character and illustrated temple background are not used.
- The compact crest and a small divider crystal may be used as brand details.
- Crystals are not used inside buttons.
- Solid buttons have visible relief and physically move down when pressed.
- Generic interface actions continue to use Lucide icons.
- Product terminology is not replaced by fantasy terminology.
- No backend, OpenAPI, generated model, route, or domain change is required.

## Out of scope

- Redesigning information architecture or navigation labels.
- Changing authentication behavior.
- Adding new product features or backend endpoints.
- Adding the Grom character to the persistent application shell.
- Animating particles, runes, ambient scenery, or other game effects.
- Converting every surface into an RPG-styled component.
- Creating a general animation or theming framework.
- Adding light mode in this delivery.

## Current frontend state

The existing UI already has a useful dark foundation, but its green is
technology-neon rather than organic, and many colors are embedded directly in
component classes.

The main implementation areas are:

| Area | Current state | Required direction |
|---|---|---|
| Global theme | Dark green with neon accent | Obsidian, moss, ivory, and bronze |
| Sign-in | Large two-column marketing panel | Centered brand form |
| Primary buttons | Flat fill | Raised surface with press travel |
| Inputs | Compact flat fields | Conventional fields with stronger focus |
| Brand mark | Generic container icon | Compact Grom crest |
| Application shell | Glassy green sidebar | Quieter charcoal and moss shell |
| Cards and tables | Mixed inline colors | Shared semantic surface tokens |
| Mobile sign-in | Collapsed desktop split panel | Purpose-built borderless column |

## Delivery strategy

Implementation is divided into seven phases. Each phase should leave the
frontend buildable and testable. Broad propagation begins only after the sign-in
checkpoint.

## Phase 1: production assets

### Deliverables

- Compact transparent Grom crest for application use.
- Favicon exports suitable for browser sizes.
- One small decorative crystal for the brand divider.
- Optimized image formats and predictable dimensions.

### Target paths

```text
frontend/src/assets/logos/
└── grom-crest.png

frontend/src/assets/icons/
└── crystal-green.svg
```

The final filenames may change if a vector crest becomes available. Imports must
use the `@/assets/...` alias.

### Rules

- Do not ship the concept screenshots in the frontend bundle.
- Do not import files directly from `frontend/src/assets/raw`.
- The crest must remain legible at approximately `48px`.
- Decorative assets must have transparent backgrounds.
- The crystal should be code-native SVG or CSS when visual fidelity permits.
- No button-specific crystal asset is needed.

### Validation

- Inspect the crest at 32, 48, 64, and 96 CSS pixels.
- Confirm transparent corners and no white fringe.
- Confirm optimized file size before committing.
- Confirm the favicon is recognizable at 16 and 32 pixels.

## Phase 2: semantic visual foundation

### Primary file

- `frontend/src/app/styles/main.css`

### Work

Replace the current neon-green theme with semantic tokens derived from the
identity guide:

- Page, surface, elevated surface, and overlay colors.
- Primary and muted text.
- Border and bronze-border colors.
- Accent, accent-hover, and accent-pressed colors.
- Danger and warning colors.
- Button highlight, button base, and button contact-shadow colors.
- Shared focus ring.
- Shared radii and clipped-corner measurements.
- Motion durations and easing.

Expose tokens through Tailwind's `@theme inline` block when utility classes need
them. Prefer semantic variables over page-specific hex values.

### Initial token groups

```text
--background
--foreground
--surface
--surface-raised
--muted
--muted-foreground
--border
--border-brand
--accent
--accent-hover
--accent-pressed
--accent-foreground
--bronze
--ruby
--danger
--focus-ring
--button-highlight
--button-base
--button-shadow
--radius
--radius-brand
--motion-fast
--motion-standard
```

### Global behavior

- Preserve `color-scheme: dark`.
- Replace the current bright radial glow with a restrained moss gradient.
- Preserve the `320px` minimum layout width.
- Keep reduced-motion handling.
- Avoid textures that require a large background image.

### Validation

- Search for obsolete neon values after the migration.
- Check text and focus contrast before propagating the theme.
- Confirm existing screens remain usable while still partially migrated.

## Phase 3: shared primitives

### Button

Primary file:

- `frontend/src/shared/components/ui/button/Button.vue`

#### Resting behavior

Solid buttons use a raised surface:

- Shallow vertical gradient.
- One-pixel highlighted top edge.
- Darker base edge.
- Contact shadow below the button.
- Slightly clipped or beveled corners.
- No crystals.

The main sign-in action uses the strongest relief. Ordinary primary actions use
the same language at lower intensity. Outline and danger buttons use relief
appropriate to their surface. Ghost buttons remain visually quiet but still
receive clear pressed feedback.

#### Press behavior

The button surface moves down without changing document layout:

1. Resting state uses `transform: translateY(0)`.
2. Contact shadow visually creates a base below the surface.
3. `:active` translates the surface down by the depth of the base.
4. `:active` reduces or removes the lower contact shadow.
5. Releasing returns the surface to its original position.

Recommended starting values:

```text
Default rise: 3px
Small rise: 2px
Icon rise: 2px
Press transition: 70–90ms
Color transition: 140–180ms
```

Use `transform`, not margin or padding, so surrounding content does not reflow.
The focus ring must remain visible in both resting and pressed states.

#### States

- `hover`: slightly brighter surface; relief remains stable.
- `active`: surface travels down; lower shadow collapses.
- `focus-visible`: independent high-contrast outline.
- `disabled`: no press travel and a distinct disabled surface.
- `loading`: preserve button width, expose `aria-busy`, and prevent repeated
  submission.
- `prefers-reduced-motion`: remove animated travel; apply the pressed shadow
  change immediately.

The existing variants remain `default`, `outline`, `ghost`, and `danger`. Do not
introduce a variant named `rpg`. The visual language belongs to the component
system rather than a themed exception.

### Input

Primary file:

- `frontend/src/shared/components/ui/input/Input.vue`

Add or confirm:

- Minimum `44px` desktop height and `48px` touch height.
- Visible focus ring and focus border.
- Invalid and disabled states.
- Attribute forwarding for identifiers and ARIA state.
- Optional trailing action support for password visibility.
- Adequate right padding when a trailing action exists.

Inputs remain flat. They do not receive relief, crystals, bronze ornament, or
decorative bevels.

### Card

Primary file:

- `frontend/src/shared/components/ui/card/Card.vue`

Use the semantic surface, border, radius, and shadow tokens. Ordinary cards
remain modern and restrained. Brand-specific clipped corners belong to the
sign-in composition rather than the global card default.

### Badge

Primary file:

- `frontend/src/shared/components/ui/badge/Badge.vue`

Map success, warning, danger, and neutral tones to the semantic palette. Ruby
brand accents must not be confused with destructive state.

### Primitive tests

Add focused component tests for:

- Button variants and disabled state.
- Loading semantics when implemented.
- Presence of the expected button state classes or data attributes.
- Input value propagation.
- Trailing action rendering.
- Invalid and disabled input attributes.

Browser rendering, rather than unit tests, validates the physical press effect.

## Phase 4: brand components

### Proposed files

```text
frontend/src/shared/components/brand/
├── GromBrand.vue
├── GromCrest.vue
└── CrystalDivider.vue
```

### Responsibilities

`GromCrest.vue`

- Owns approved crest sizing.
- Supports decorative or meaningful alternative text.
- Does not contain page-specific layout.

`GromBrand.vue`

- Combines the compact crest and product name.
- Supports the sign-in and application-shell arrangements.
- Keeps typography modern and readable.

`CrystalDivider.vue`

- Renders one small centered crystal with restrained bronze hairlines.
- Is decorative by default and hidden from assistive technology.
- Is never rendered inside a button.

Keep these components narrow. Do not create a general fantasy component library.

## Phase 5: sign-in implementation

### Primary file

- `frontend/src/modules/auth/pages/SignInPage.vue`

### Structure

Replace the current split composition with:

```text
signin-page
└── signin-surface
    ├── GromBrand
    ├── CrystalDivider
    └── form
        ├── heading and description
        ├── email field
        ├── password field and visibility action
        ├── error message
        ├── raised submit button
        └── compatibility note
```

### Copy

Use clear product language:

- Brand: `Grom Registry`
- Heading: `Sign in`
- Description: `Manage projects, repositories and access keys.`
- Compatibility: `OCI compatible · Distribution powered`

Keep the bootstrap-credential help only if it remains operationally useful. It
must not compete with the compatibility line.

### Desktop behavior

- Center the surface horizontally and vertically where viewport height permits.
- Use a narrow form width around `420–460px`.
- Apply a thin bronze hairline and clipped brand corners.
- Do not use fixed vertical height.
- Allow the page to scroll on short viewports.

### Mobile behavior

- Remove the visible card border, elevated background, and desktop shadow.
- Use a single column with approximately `24px` horizontal padding.
- Keep the crest around `48–56px`.
- Use full-width inputs and submit button.
- Account for `env(safe-area-inset-*)`.
- Keep the form usable with the on-screen keyboard.
- Do not vertically center content when doing so risks clipping it.

### Interaction

- Toggle password visibility without losing focus unexpectedly.
- Clear stale error messaging before a new submission.
- Disable repeated submission while loading.
- Expose loading through text and `aria-busy`.
- Preserve the current redirect behavior after successful authentication.

### Tests

Add a `SignInPage` test covering:

- Email and password bindings.
- Password visibility control.
- Loading state.
- Error message rendering.
- Successful redirect behavior with mocked session and router dependencies.

### Approval gate

Before changing the authenticated shell:

1. Run the sign-in page locally.
2. Capture desktop and mobile screenshots.
3. Compare them with the approved direction, accounting for the explicit removal
   of button crystals.
4. Review resting, hover, focus, pressed, disabled, loading, and error states.
5. Obtain visual approval.

## Phase 6: authenticated application shell

### Primary files

- `frontend/src/app/App.vue`
- `frontend/src/app/styles/main.css`

### Work

- Replace the generic container brand mark with `GromBrand` or `GromCrest`.
- Apply the charcoal, moss, ivory, and bronze tokens.
- Reduce the current glassy green treatment.
- Keep navigation active state clear without neon glow.
- Keep crystals out of navigation links, user controls, and sign-out actions.
- Apply button press behavior consistently.
- Preserve the existing desktop sidebar and mobile drawer architecture.
- Confirm the drawer still closes after navigation.

### Mobile checks

- Sticky header remains readable over page content.
- Drawer has a clear overlay and focus behavior.
- Brand remains compact.
- All icon buttons meet the touch target requirement.

## Phase 7: page propagation

Apply shared tokens and primitives in this order:

1. `frontend/src/modules/projects/pages/ProjectsPage.vue`
2. `frontend/src/modules/projects/pages/ProjectPage.vue`
3. `frontend/src/modules/service-accounts/pages/ServiceAccountsPage.vue`
4. `frontend/src/modules/service-accounts/components/ServiceAccountKeysPanel.vue`
5. `frontend/src/modules/users/pages/UsersPage.vue`
6. `frontend/src/modules/integrations/pages/IntegrationsPage.vue`

### Propagation rules

- Do not change page behavior or API calls.
- Replace hard-coded surface and accent colors with semantic tokens.
- Use raised buttons consistently, without crystals.
- Keep tables, tabs, badges, and modals comparatively flat.
- Do not add the crest to individual page headers.
- Do not add decorative crystals to cards, tables, tabs, or empty states.
- Preserve monospaced technical content.
- Keep destructive actions unmistakable.
- Avoid changing multiple page layouts unless responsive behavior requires it.

## Verification plan

### Automated checks

Run after each meaningful phase:

```text
cd frontend && npm run lint
cd frontend && npm run typecheck
cd frontend && npm test
```

Run before handoff:

```text
make test
make build
```

### Responsive matrix

Validate at minimum:

| Viewport | Purpose |
|---|---|
| `320 × 568` | Minimum supported narrow screen |
| `390 × 844` | Common modern phone |
| `768 × 1024` | Tablet and breakpoint behavior |
| `1280 × 800` | Short desktop |
| `1440 × 900` | Primary desktop review |

Also validate:

- Browser zoom at 200%.
- Long validation errors.
- Small viewport height with scroll.
- Keyboard-only navigation.
- Touch interaction.
- `prefers-reduced-motion`.
- Slow authentication response.

### Accessibility

- WCAG AA contrast for body text and controls.
- Visible keyboard focus on every action.
- Decorative crest or crystal details do not create duplicate announcements.
- Password visibility control has a changing accessible label.
- Loading state is announced and prevents duplicate submission.
- Button press travel does not hide the focus outline.
- Color is not the sole indicator of errors or status.

### Browser visual validation

After automated checks, inspect:

- Button top highlight and lower base at rest.
- Button travel and collapsed shadow during press.
- No layout shift around a pressed button.
- No clipped focus ring.
- No white fringe on the crest.
- Card removal at the mobile breakpoint.
- Correct scroll behavior on short screens.

Screenshot baselines should be added only after the implementation receives
visual approval, so temporary design iteration does not create noisy snapshots.

## Documentation maintenance during implementation

Update:

- `docs/visual-identity.md` if visual rules change.
- This plan if file ownership or delivery sequence changes materially.
- `docs/code-map.md` when new persistent brand component paths are introduced.
- `frontend/src/assets/README.md` when production asset categories or rules
  change.
- `AGENTS.md` when future frontend agents need a new visual workflow or pitfall.

No update is expected for:

- `docs/domain-model.md`
- `docs/architecture-and-mvp.md`
- `backend/api/openapi.yaml`
- Generated backend or frontend API code

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Relief feels like a game control | Keep depth shallow and typography modern |
| Press travel causes layout movement | Use `transform`, never margin or padding |
| Focus ring is clipped by overflow | Avoid clipping button containers; test keyboard focus |
| Crest looks blurry or has a white halo | Validate transparent exports at actual CSS sizes |
| RPG details spread into data-heavy UI | Restrict crystals to brand compositions and dividers |
| Mobile content is vertically clipped | Avoid fixed heights and desktop vertical centering |
| Hard-coded legacy greens remain | Search the frontend after token migration |
| Disabled state looks merely faded | Define separate surface, text, and shadow behavior |

## Definition of done

The implementation is complete when:

- Production crest and divider assets are optimized and correctly located.
- Semantic visual tokens replace the legacy neon theme.
- Buttons have consistent relief and press down without layout reflow.
- No button contains crystals.
- Inputs, cards, and badges use the approved component language.
- Sign-in matches the approved desktop and mobile direction.
- The authenticated shell and existing pages use the quieter shared system.
- Existing authentication, navigation, and management behavior is unchanged.
- Accessibility and responsive checks pass.
- `make test` and `make build` pass.
- Documentation reflects the final implementation.
