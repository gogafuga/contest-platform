export const languages = [
  { id: 'go', label: 'Go', extension: 'go' },
  { id: 'python', label: 'Python', extension: 'py' },
  { id: 'csharp', label: 'C#', extension: 'cs' },
] as const
export type Language = typeof languages[number]['id']
export function isLanguage(value: unknown): value is Language {
  return languages.some(language => language.id === value)
}
