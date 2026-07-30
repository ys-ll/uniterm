/**
 * Centralised test-id builder (ARCH-033).
 *
 * Components previously inlined test-id strings like `data-testid="skill-card-${name}"`
 * in many places. Use this helper so a future renaming convention lives in
 * one spot and the resulting IDs are stable across the codebase.
 *
 *   <div :data-testid="testId('skill-card', skill.name)" />
 */
export function testId(
  ...parts: Array<string | number | undefined | null>
): string {
  return parts
    .filter((p) => p !== undefined && p !== null && p !== "")
    .join("-");
}

/**
 * Common id roots so the frontend tests can rely on a single namespace.
 * Add new roots here instead of inventing ad-hoc strings in components.
 */
export const TEST_IDS = {
  skillCard: (name: string) => testId("skill", name),
  connectionRow: (id: string) => testId("conn", id),
  tunnelRow: (id: string) => testId("tunnel", id),
} as const;
