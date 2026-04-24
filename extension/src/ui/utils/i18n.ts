// Thin wrapper over `chrome.i18n.getMessage` so callers don't have to care
// about:
//   - Substitutions needing to be stringified (chrome only accepts strings
//     in the `substitutions` array — numbers silently render as `""`).
//   - Missing keys returning `""` (we fall back to the key itself so a
//     forgotten translation is visible rather than silently empty).
//
// Messages live in `_locales/{en,ko}/messages.json`. Placeholders follow the
// Chrome i18n format: `$name$` inside the `message` field maps through the
// `placeholders: { name: { content: "$1" } }` table to the first array arg.

export function t(key: string, subs?: Array<string | number>): string {
  // `chrome.i18n.getMessage` expects string | string[]. Normalise numbers
  // up-front so callers can pass raw counts / sizes without ceremony.
  const normalised = subs?.map((v) => (typeof v === "string" ? v : String(v)));
  const msg = chrome.i18n.getMessage(key, normalised);
  // Empty string = key missing from the active locale. Surface the key so
  // the missing translation is obvious in UI instead of a silent blank.
  return msg || key;
}
