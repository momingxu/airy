import {
  argbFromHex,
  hexFromArgb,
  themeFromSourceColor,
} from '@material/material-color-utilities'

export type ThemeMode = 'light' | 'dark'

export const DEFAULT_THEME_SEED = '#675378'
export const THEME_SEED_STORAGE_KEY = 'airy-theme-seed'

export const THEME_PRESETS = [
  { name: 'Airy 紫', value: DEFAULT_THEME_SEED },
  { name: '海雾蓝', value: '#42628a' },
  { name: '森林绿', value: '#386a20' },
  { name: '珊瑚红', value: '#9b4148' },
  { name: '琥珀金', value: '#825500' },
  { name: '石墨灰', value: '#5f5e62' },
] as const

export function normalizeThemeSeed(value: string | null | undefined) {
  const normalized = value?.trim().toLowerCase() ?? ''
  return /^#[0-9a-f]{6}$/.test(normalized) ? normalized : null
}

export function applyMaterialTheme(seed: string, mode: ThemeMode, target = document.documentElement) {
  const normalizedSeed = normalizeThemeSeed(seed) ?? DEFAULT_THEME_SEED
  const generated = themeFromSourceColor(argbFromHex(normalizedSeed))
  const scheme = mode === 'dark' ? generated.schemes.dark : generated.schemes.light
  const palette = generated.palettes
  const dark = mode === 'dark'
  const color = (argb: number) => hexFromArgb(argb)

  const variables: Record<string, string> = {
    '--accent': color(scheme.primary),
    '--on-accent': color(scheme.onPrimary),
    '--accent-strong': color(scheme.onPrimaryContainer),
    '--accent-deep': color(palette.primary.tone(dark ? 90 : 25)),
    '--accent-soft': color(scheme.primaryContainer),
    '--on-accent-soft': color(scheme.onPrimaryContainer),
    '--secondary': color(scheme.secondary),
    '--tertiary': color(scheme.tertiary),
    '--surface': color(palette.neutral.tone(dark ? 6 : 98)),
    '--surface-raised': color(palette.neutral.tone(dark ? 10 : 100)),
    '--surface-muted': color(palette.neutralVariant.tone(dark ? 20 : 92)),
    '--text': color(scheme.onSurface),
    '--text-muted': color(scheme.onSurfaceVariant),
    '--line': color(scheme.outlineVariant),
    '--sidebar': color(palette.neutral.tone(dark ? 8 : 94)),
    '--error': color(scheme.error),
    '--error-soft': color(scheme.errorContainer),
    '--on-error-soft': color(scheme.onErrorContainer),
    '--shadow': `0 ${dark ? 20 : 18}px ${dark ? 50 : 46}px color-mix(in srgb, ${color(scheme.shadow)} ${dark ? 28 : 12}%, transparent)`,
  }

  for (const [property, value] of Object.entries(variables)) {
    target.style.setProperty(property, value)
  }
  target.style.colorScheme = mode
  target.dataset.themeSeed = normalizedSeed
}
