import { useEffect, useRef } from 'react'
import { monaco } from './runtime'
import { attachAssist } from './lsp'

export default function Monaco({ value, language, readOnly, onChange, original, height, assist, onAssistStatus }: {
  value: string
  language: string
  readOnly: boolean
  onChange?: (value: string) => void
  original?: string
  height?: number | string
  assist?: { taskId: string; token: string }
  onAssistStatus?: (text: string) => void
}) {
  const host = useRef<HTMLDivElement>(null)
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null)
  const diff = useRef<monaco.editor.IStandaloneDiffEditor | null>(null)
  const synchronizing = useRef(false)
  const change = useRef(onChange)
  change.current = onChange
  const isDiff = original !== undefined
  const reportStatus = useRef(onAssistStatus)
  reportStatus.current = onAssistStatus

  useEffect(() => {
    if (!host.current) return
    const model = monaco.editor.createModel('', language)
    const options = {
      automaticLayout: true, theme: 'vs-dark', fontSize: 14,
      minimap: { enabled: false }, scrollBeyondLastLine: false,
      tabSize: 4, readOnly, ariaLabel: 'Исходный код',
      quickSuggestions: { other: 'on' as const, comments: 'off' as const, strings: 'on' as const },
      quickSuggestionsDelay: 120, suggestOnTriggerCharacters: true,
      fixedOverflowWidgets: true, parameterHints: { enabled: true },
      suggest: { showWords: false, preview: false }, wordBasedSuggestions: 'off' as const,
    }
    let before: monaco.editor.ITextModel | undefined
    if (isDiff) {
      before = monaco.editor.createModel('', language)
      diff.current = monaco.editor.createDiffEditor(host.current, { ...options, readOnly: true, originalEditable: false })
      diff.current.setModel({ original: before, modified: model })
    } else {
      editor.current = monaco.editor.create(host.current, { ...options, model })
    }
    let suggestionTimer: number | undefined
    const subscription = model.onDidChangeContent(() => {
      if (synchronizing.current) return
      change.current?.(model.getValue())
      window.clearTimeout(suggestionTimer)
      // A completion requested at the start of a word may be canceled while
      // typing over HTTP. Retry at the current cursor after the user pauses.
      suggestionTimer = window.setTimeout(() => {
        const instance = editor.current
        if (!instance || !instance.hasTextFocus() || instance.getOption(monaco.editor.EditorOption.readOnly)) return
        const position = instance.getPosition()
        if (!position) return
        const prefix = model.getLineContent(position.lineNumber).slice(0, position.column - 1)
        if (/[\p{L}\p{N}_.]$/u.test(prefix)) instance.trigger('typing', 'editor.action.triggerSuggest', {})
      }, 220)
    })
    return () => {
      window.clearTimeout(suggestionTimer)
      subscription.dispose()
      editor.current?.dispose()
      diff.current?.dispose()
      editor.current = null
      diff.current = null
      model.dispose()
      before?.dispose()
    }
    // Models live for the lifetime of this viewer; props synchronize below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isDiff])

  useEffect(() => {
    synchronizing.current = true
    const model = editor.current?.getModel() ?? diff.current?.getModel()?.modified
    if (model) {
      if (model.getValue() !== value) model.setValue(value)
      monaco.editor.setModelLanguage(model, language)
    }
    const before = diff.current?.getModel()?.original
    if (before) {
      if (before.getValue() !== original) before.setValue(original ?? '')
      monaco.editor.setModelLanguage(before, language)
    }
    editor.current?.updateOptions({ readOnly })
    synchronizing.current = false
  }, [value, language, readOnly, original])

  const assistTask = assist?.taskId
  const assistToken = assist?.token
  useEffect(() => {
    const model = editor.current?.getModel()
    if (!model || readOnly || !assistTask || !assistToken) return
    const connection = attachAssist(model, language, { taskId: assistTask, token: assistToken }, text => reportStatus.current?.(text), () => {
      const instance = editor.current
      const position = instance?.getPosition()
      if (!instance?.hasTextFocus() || !position) return
      const prefix = model.getLineContent(position.lineNumber).slice(0, position.column - 1)
      if (/[\p{L}\p{N}_.]$/u.test(prefix)) instance.trigger('analyzer-ready', 'editor.action.triggerSuggest', {})
    })
    return () => connection.dispose()
  }, [language, readOnly, assistTask, assistToken, isDiff])

  return <div ref={host} style={{ height: height ?? (isDiff ? 520 : 360), width: '100%', minWidth: 0 }} />
}
