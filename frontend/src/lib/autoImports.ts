interface Candidate { label: string; namespace: string; symbol?: boolean }
export interface ImportSuggestion { label: string; detail: string; offset: number; text: string }
const candidates: Record<string, Candidate[]> = {
  go: ['fmt', 'os', 'io', 'bufio', 'strings', 'strconv', 'sort', 'slices', 'maps', 'bytes', 'math', 'regexp', 'unicode', 'time', 'errors', 'log', 'encoding/json', 'net/netip', 'path/filepath', 'unicode/utf8'].map(namespace => ({ label: namespace.split('/').at(-1)!, namespace })),
  python: [
    ...['os', 'sys', 're', 'json', 'math', 'collections', 'itertools', 'functools', 'heapq', 'bisect', 'csv', 'statistics', 'datetime'].map(namespace => ({ label: namespace, namespace })),
    ...['Counter', 'defaultdict', 'deque'].map(label => ({ label, namespace: 'collections', symbol: true })),
    { label: 'Path', namespace: 'pathlib', symbol: true },
  ],
  csharp: [
    ...['Console', 'Math', 'DateTime', 'Exception', 'Array', 'Convert'].map(label => ({ label, namespace: 'System' })),
    ...['File', 'Directory', 'Path', 'StreamReader', 'StreamWriter'].map(label => ({ label, namespace: 'System.IO' })),
    ...['List', 'Dictionary', 'HashSet', 'Queue', 'Stack'].map(label => ({ label, namespace: 'System.Collections.Generic' })),
    { label: 'Regex', namespace: 'System.Text.RegularExpressions' }, { label: 'StringBuilder', namespace: 'System.Text' }, { label: 'JsonSerializer', namespace: 'System.Text.Json' },
  ],
}
const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
function imported(language: string, source: string, item: Candidate): boolean {
  if (language === 'csharp') return new RegExp(`^\\s*(?:global\\s+)?using\\s+${escape(item.namespace)}\\s*;`, 'm').test(source)
  if (language === 'go') return Array.from(source.matchAll(/^\s*import\s*(?:\(([\s\S]*?)\)|([^\r\n]+))/gm)).some(match => new RegExp(`"${escape(item.namespace)}"`).test(match[1] ?? match[2]))
  if (item.symbol) return new RegExp(`^\\s*from\\s+${escape(item.namespace)}\\s+import\\s+[^#\\n]*\\b${escape(item.label)}\\b`, 'm').test(source)
  return Array.from(source.matchAll(/^\s*import\s+([^#\n]+)/gm)).some(match => match[1].split(',').some(part => part.trim().split(/\s+/)[0] === item.namespace))
}
export function getImportSuggestions(language: string, source: string, prefix: string): ImportSuggestion[] {
  if (!prefix) return []
  return (candidates[language] ?? []).filter(item => item.label.toLowerCase().startsWith(prefix.toLowerCase()) && !imported(language, source, item)).flatMap(item => {
    let offset = 0
    let text: string
    let detail: string
    if (language === 'go') {
      const pkg = /^package[ \t]+\w+[^\r\n]*(?:\r?\n|$)/m.exec(source)
      if (!pkg) return []
      const block = /^import[ \t]*\(/m.exec(source)
      const end = block ? source.indexOf(')', block.index + block[0].length) : -1
      detail = `import "${item.namespace}"`
      if (block && end >= 0) { offset = end; text = `\n\t"${item.namespace}"\n` }
      else { offset = pkg.index + pkg[0].length; text = `\nimport "${item.namespace}"\n` }
    } else if (language === 'python') {
      // Keep module docstrings and __future__ imports ahead of ordinary imports:
      // insert before the first code line, after the existing header/import section.
      const lines = source.split('\n')
      let inDoc: string | null = null
      for (const line of lines) {
        const trimmed = line.trim()
        if (inDoc) { offset += line.length + 1; if (trimmed.includes(inDoc)) inDoc = null; continue }
        const doc = /^(?:r|u)?('''|""")/i.exec(trimmed)
        if (doc) { if (!trimmed.slice(doc[0].length).includes(doc[1])) inDoc = doc[1]; offset += line.length + 1; continue }
        if (!trimmed || trimmed.startsWith('#') || /^(from\s|import\s)/.test(trimmed)) { offset += line.length + 1; continue }
        break
      }
      offset = Math.min(offset, source.length)
      detail = item.symbol ? `from ${item.namespace} import ${item.label}` : `import ${item.namespace}`
      text = `${offset > 0 && source[offset - 1] !== '\n' ? '\n' : ''}${detail}\n`
    } else { detail = `using ${item.namespace};`; text = detail + '\n' }
    return [{ label: item.label, detail, offset, text }]
  })
}
