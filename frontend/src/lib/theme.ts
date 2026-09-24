import { createSignal } from 'solid-js';

export type ThemeChoice = 'light' | 'dark' | 'system';

const STORAGE_KEY = 'theme';
const ORDER: ThemeChoice[] = ['system', 'light', 'dark'];

const darkQuery = window.matchMedia('(prefers-color-scheme: dark)');

// The OS preference is a signal so the map and the previews re-render when it
// changes, not only when the user picks a theme by hand.
const [systemPrefersDark, setSystemPrefersDark] = createSignal(darkQuery.matches);
darkQuery.addEventListener('change', (event) => setSystemPrefersDark(event.matches));

function readChoice(): ThemeChoice {
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system';
}

function apply(choice: ThemeChoice): void {
  // Leaving data-theme unset lets the stylesheet's prefers-color-scheme rules
  // decide, which is exactly the "system" behaviour.
  if (choice === 'system') {
    delete document.documentElement.dataset.theme;
  } else {
    document.documentElement.dataset.theme = choice;
  }
}

const [choice, setChoice] = createSignal<ThemeChoice>(readChoice());

/** themeChoice is the user's stored preference. */
export function themeChoice(): ThemeChoice {
  return choice();
}

/** visibleTheme resolves the preference to the two themes the maps know. */
export function visibleTheme(): 'light' | 'dark' {
  const current = choice();
  if (current !== 'system') return current;
  return systemPrefersDark() ? 'dark' : 'light';
}

/** setTheme stores and applies a preference. */
export function setTheme(next: ThemeChoice): void {
  localStorage.setItem(STORAGE_KEY, next);
  apply(next);
  setChoice(next);
}

/** cycleTheme advances system → light → dark → system. */
export function cycleTheme(): void {
  setTheme(ORDER[(ORDER.indexOf(choice()) + 1) % ORDER.length] ?? 'system');
}
