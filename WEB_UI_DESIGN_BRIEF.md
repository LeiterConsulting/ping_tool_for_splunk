# Ping Monitor Web UI Design Brief

## Intent

- Preserve the interaction model and visual rhythm of the existing SNMP for Splunk web UI without copying its code or assets.
- Give Ping Monitor its own identity through a distinct accent palette and product-specific navigation.
- Keep the first implementation aligned with the current Go runtime, where `endpoints.csv` remains the initial source of truth.

## Design Goals

- Familiar to operators who already use both products.
- Clean-room implementation in this repository only.
- Dense operational UI, not a marketing site.
- Fast scanning of status, tables, filters, and actions.
- Strong continuity in layout, shapes, and interaction patterns.
- Clear visual separation from the SNMP product.

## UI Language To Mirror

These are the pieces worth preserving from the existing SNMP application:

- A fixed left navigation rail with icon + label items, a compact product badge, version text, and a simple footer.
- A sticky page header with title, short description, and right-aligned actions.
- Dark layered surfaces with restrained borders and muted secondary text.
- Rounded geometry throughout: large-radius cards, rounded buttons, pill badges, and softened table states.
- Reusable primitives instead of page-specific styling: card, stat card, button, input, select, textarea, badge, table, modal, status dot.
- Compact spacing rhythm suitable for operations workflows: 12 px to 24 px gaps, short labels, dense but readable tables.
- Color used for emphasis and state, not as decoration.

## Ping Monitor Visual Direction

Keep the same overall shell and surface treatment, but move away from the SNMP app's green-led identity.

### Proposed Palette

- Primary accent: `#0F6CBD`
- Secondary accent: `#12B3A8`
- Success: `#73BF69`
- Warning: `#E5A63A`
- Danger: `#D64545`
- Background 900: `#11161C`
- Surface 800: `#1A222B`
- Border 700: `#2A3642`
- Text 300: `#C7D2DD`
- Muted 500: `#738395`

This keeps continuity in contrast and depth while making Ping Monitor read as a separate tool.

## Product-Specific Shape

- Product mark can use `PM` inside the sidebar badge instead of reusing the SNMP badge treatment literally.
- Status-heavy views should bias toward blue and teal for neutral operational states.
- Dev/test segmentation can use amber accents so it stands apart from production without implying failure.

## Recommended Information Architecture

For a first Ping Monitor UI, the page model should stay close to the SNMP shell but map to Ping Monitor tasks:

- Overview
- Endpoints
- Dev Devices
- Discovery
- Groups and Metadata
- Outputs and HEC
- Diagnostics
- Settings

This preserves familiarity while avoiding one-to-one imitation of the SNMP product's feature map.

## Recommended Technical Approach

The current Go runtime does not expose a management HTTP surface. The web UI therefore needs two parts:

1. A clean-room frontend in a new `web/` directory.
2. A lightweight API layer in the Go runtime to read, validate, and write endpoint/config state.

### Frontend

- React + Vite
- Tailwind CSS
- React Router for the shell and page model
- TanStack Query for API state
- Lucide icons or equivalent lightweight icon set

This matches the productivity and component ergonomics of the SNMP UI without importing that codebase.

### Backend

- Serve built frontend assets from the Go binary with `embed.FS`.
- Add a small authenticated local API for configuration management.
- Keep CSV as the first persistence layer so the runtime's existing hot reload continues to work.
- Use atomic write + backup on every endpoint save.

### Initial API Shape

- `GET /api/endpoints`
- `PUT /api/endpoints`
- `POST /api/endpoints/import`
- `GET /api/config`
- `PUT /api/config`
- `POST /api/discovery/run`
- `GET /api/discovery/jobs/:id`
- `POST /api/discovery/merge`

## MVP Scope

Phase 1 should cover:

- App shell, navigation, header, cards, tables, forms, modals
- Endpoint CRUD
- CSV import of existing endpoint files
- `dev` toggle and metadata editing
- Validation and preview before save
- Save-to-CSV with backup and runtime hot reload

Phase 2 can add:

- Discovery job execution
- Review-and-merge workflow for discovered hosts
- Bulk edits
- Audit trail and change history

## Guardrails

- Do not copy code, CSS, or assets from the SNMP repository.
- Do not make Splunk dashboards the primary configuration surface for this feature.
- Do not replace `endpoints.csv` immediately; use it as the compatibility bridge for the first release.
- Keep `dev` as the final optional endpoint column in any import/export workflow.

## Acceptance Criteria

- An operator who knows the SNMP UI can immediately recognize the navigation model and component behavior.
- The Ping Monitor UI still reads as a distinct product because the accent palette and page taxonomy are different.
- Existing endpoint files can be imported without schema breakage.
- Saving endpoint changes updates the runtime through the existing reload path instead of introducing a second config source.