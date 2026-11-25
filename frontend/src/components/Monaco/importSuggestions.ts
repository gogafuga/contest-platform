import { monaco } from './runtime'
import { getImportSuggestions } from '../../lib/autoImports'
import { standardMembers } from '../../lib/standardMembers'

// Language-server completion remains the primary source. These standard-library
// candidates also work while its private service is unavailable.
export function importSuggestions(model: monaco.editor.ITextModel, position: monaco.Position): monaco.languages.CompletionItem[] {
  const word = model.getWordUntilPosition(position)
  const before = model.getLineContent(position.lineNumber).slice(0, word.startColumn - 1)
  if (before.trimEnd().endsWith('.')) return []
  return getImportSuggestions(model.getLanguageId(), model.getValue(), word.word).map(item => {
    const insertion = model.getPositionAt(item.offset)
    return {
      label: item.label, kind: model.getLanguageId() === 'csharp' ? monaco.languages.CompletionItemKind.Class : monaco.languages.CompletionItemKind.Module,
      detail: item.detail, insertText: item.label,
      range: new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn),
      additionalTextEdits: [{ range: new monaco.Range(insertion.lineNumber, insertion.column, insertion.lineNumber, insertion.column), text: item.text }],
    }
  })
}

export function memberSuggestions(model: monaco.editor.ITextModel, position: monaco.Position): monaco.languages.CompletionItem[] {
  const word = model.getWordUntilPosition(position)
  const before = model.getLineContent(position.lineNumber).slice(0, position.column - 1)
  const members = standardMembers(model.getLanguageId(), before)
  if (!members.length) return []
  const receiver = members[0].receiver.split('.')[0]
  const missingImport = getImportSuggestions(model.getLanguageId(), model.getValue(), receiver).find(candidate => candidate.label === receiver)
  const insertion = missingImport ? model.getPositionAt(missingImport.offset) : undefined
  return members.map(item => ({
    label: item.label, insertText: item.label,
    kind: monaco.languages.CompletionItemKind.Text,
    detail: `${item.receiver}.${item.label} · стандартная библиотека`,
    additionalTextEdits: insertion && missingImport ? [{
      range: new monaco.Range(insertion.lineNumber, insertion.column, insertion.lineNumber, insertion.column), text: missingImport.text,
    }] : undefined,
    range: new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn),
  }))
}
