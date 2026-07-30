/**
 * Shared a11y attribute helpers and CSS class constants.
 *
 * The codebase has minimal a11y attributes today (only `aria-label`
 * appears, and 3 of the 4 occurrences are in WindowControls.vue), but
 * centralising the few patterns that *are* repeated here means future
 * i18n keys / decoration flags land in one place instead of being
 * copy-pasted into every icon button.
 */

export const SR_ONLY_CLASS = "sr-only";

/**
 * Returns aria attributes for an icon-only button. Use as
 *
 *   <button v-bind="iconButtonAttrs(t('window.close'))" @click="...">
 *
 * so the label stays consistent with the i18n catalog. Keep the rendered
 * button text empty (or sr-only) — screen readers otherwise double-announce.
 */
export function iconButtonAttrs(label: string): {
  "aria-label": string;
  role: "button";
} {
  return { "aria-label": label, role: "button" };
}

/**
 * Decorative SVG / illustration. Marks the element hidden from screen
 * readers and from the tab order. The `aria-hidden` literal is fine,
 * but centralising it lets future tests assert a single shape.
 */
export const ARIA_HIDDEN = { "aria-hidden": true };

/**
 * Selectors that match every interactive element on the page. Used by
 * click-outside / focus-skip logic (e.g. AppHeader's "click is on a
 * control" guard) so the list lives in one place. Keep in sync with
 * the components that need it.
 */
export const INTERACTIVE_SELECTOR =
  'button, input, textarea, select, a, [role="button"], .tab-item, .tab-more, .window-controls';
