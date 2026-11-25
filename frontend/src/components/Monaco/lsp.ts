import { monaco } from './runtime'
import { importSuggestions, memberSuggestions } from './importSuggestions'
import { withinCompletionBudget } from '../../lib/completionBudget'

interface Auth { taskId: string; token: string }
interface LspRange { start: { line: number; character: number }; end: { line: number; character: number } }
interface TextEdit { range: LspRange; newText: string }
// Servers have language-specific completion payloads; opaque data is sent back
// only for resolution, never interpreted as an editor command.
type LspItem = Record<string, any>
const clients = new Map<string, AssistClient>()
const actionData = new WeakMap<object, { client: AssistClient; item: LspItem; model: monaco.editor.ITextModel; version: number }>()
const completionData = new WeakMap<object, { client: AssistClient; item: LspItem }>()
let installed = false
const API = import.meta.env.VITE_API_URL || ''
const range = (r: LspRange) => new monaco.Range(r.start.line + 1, r.start.character + 1, r.end.line + 1, r.end.character + 1)
const edit = (e: TextEdit) => ({ range: range(e.range), text: e.newText })
function markdown(value: any): monaco.IMarkdownString[] {
  if (!value) return []
  if (Array.isArray(value)) return value.flatMap(markdown)
  const text = typeof value === 'string' ? value : value.value
  if (typeof text !== 'string') return []
  return [{ value: value.language ? `\`\`\`${value.language}\n${text}\n\`\`\`` : text, isTrusted: false, supportHtml: false }]
}
function clientFor(model: monaco.editor.ITextModel) { return clients.get(model.uri.toString()) }
const kindNames = ['Text', 'Method', 'Function', 'Constructor', 'Field', 'Variable', 'Class', 'Interface', 'Module', 'Property', 'Unit', 'Value', 'Enum', 'Keyword', 'Snippet', 'Color', 'File', 'Reference', 'Folder', 'EnumMember', 'Constant', 'Struct', 'Event', 'Operator', 'TypeParameter'] as const
function completion(item: LspItem, model: monaco.editor.ITextModel, position: monaco.Position, defaults: any = {}): monaco.languages.CompletionItem {
  const word = model.getWordUntilPosition(position)
  const fallback = new monaco.Range(position.lineNumber, word.startColumn, position.lineNumber, word.endColumn)
  const lspRange = item.textEdit?.range ?? item.textEdit?.replace ?? defaults.editRange?.replace ?? defaults.editRange
  const name = kindNames[(item.kind ?? 1) - 1] ?? 'Text'
  return {
    label: item.label ?? '', kind: monaco.languages.CompletionItemKind[name],
    detail: item.detail, documentation: markdown(item.documentation)[0],
    insertText: item.textEdit?.newText ?? item.textEditText ?? item.insertText ?? item.label,
    insertTextRules: (item.insertTextFormat ?? defaults.insertTextFormat) === 2 ? monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet : undefined,
    range: lspRange ? range(lspRange) : fallback,
    sortText: item.sortText, filterText: item.filterText,
    additionalTextEdits: item.additionalTextEdits?.map(edit),
    preselect: item.preselect,
  }
}
function installProviders() {
  if (installed) return
  installed = true
  for (const language of ['go', 'python', 'csharp']) {
    monaco.languages.registerCompletionItemProvider(language, {
      triggerCharacters: ['.', '"'],
      async provideCompletionItems(model, position, _context, cancel) {
        const client = clientFor(model)
        if (!client) return { suggestions: [] }
        const local = [...importSuggestions(model, position), ...memberSuggestions(model, position)]
        const result: any = await client.complete(position, cancel)
        const items: LspItem[] = Array.isArray(result) ? result : result?.items ?? []
        return { incomplete: result == null || result?.pending === true || (result?.isIncomplete ?? false), suggestions: [...local.filter(suggestion => !items.some(item => item.label === suggestion.label)), ...items.slice(0, 200).map(item => {
          const value = completion(item, model, position, result?.itemDefaults)
          completionData.set(value, { client, item })
          return value
        })] }
      },
      async resolveCompletionItem(value, cancel) {
        const data = completionData.get(value)
        if (!data) return value
        const resolved: any = await data.client.request('resolveCompletion', undefined, data.item, cancel)
        if (!resolved) return value
        return { ...value, detail: resolved.detail ?? value.detail,
          documentation: markdown(resolved.documentation)[0] ?? value.documentation,
          additionalTextEdits: resolved.additionalTextEdits?.map(edit) ?? value.additionalTextEdits }
      },
    })
    monaco.languages.registerHoverProvider(language, {
      async provideHover(model, position, cancel) {
        const result: any = await clientFor(model)?.request('hover', position, undefined, cancel)
        return result ? { contents: markdown(result.contents), range: result.range ? range(result.range) : undefined } : null
      },
    })
    monaco.languages.registerSignatureHelpProvider(language, {
      signatureHelpTriggerCharacters: ['(', ','],
      signatureHelpRetriggerCharacters: [','],
      async provideSignatureHelp(model, position, cancel) {
        const result: any = await clientFor(model)?.request('signature', position, undefined, cancel)
        if (!result?.signatures?.length) return null
        return { value: { activeSignature: result.activeSignature ?? 0, activeParameter: result.activeParameter ?? 0,
          signatures: result.signatures.map((signature: any) => ({ ...signature,
            documentation: markdown(signature.documentation)[0],
            parameters: (signature.parameters ?? []).map((parameter: any) => ({ ...parameter, documentation: markdown(parameter.documentation)[0] })) })) }, dispose() {} }
      },
    })
    monaco.languages.registerDocumentFormattingEditProvider(language, {
      async provideDocumentFormattingEdits(model, _options, cancel) {
        const result: any = await clientFor(model)?.request('format', undefined, undefined, cancel)
        return Array.isArray(result) ? result.map(edit) : []
      },
    })
    monaco.languages.registerCodeActionProvider(language, {
      async provideCodeActions(model, selection, context, cancel) {
        const client = clientFor(model)
        if (!client) return { actions: [], dispose() {} }
        const actionKind = context.only?.startsWith('source.organizeImports') ? 'imports' : 'fixes'
        const version = model.getVersionId()
        const result: any = await client.request(actionKind, new monaco.Position(selection.startLineNumber, selection.startColumn), undefined, cancel)
        const actions: monaco.languages.CodeAction[] = []
        for (const item of Array.isArray(result) ? result : []) {
          const edits = workspaceEdits(item, client, model)
          // Commands are never forwarded or executed. Deferred edits may be
          // resolved through the fixed codeAction/resolve operation.
          if (!edits.length && item.data === undefined) continue
          const action: monaco.languages.CodeAction = { title: item.title, kind: item.kind,
            isPreferred: item.isPreferred, edit: edits.length ? { edits } : undefined }
          actionData.set(action, { client, item, model, version })
          actions.push(action)
        }
        return { actions, dispose() {} }
      },
      async resolveCodeAction(action, cancel) {
        const data = actionData.get(action)
        if (!data || action.edit || data.model.isDisposed() || data.model.getVersionId() !== data.version) return action
        const resolved: any = await data.client.request('resolveAction', undefined, data.item, cancel)
        if (!resolved || data.model.isDisposed() || data.model.getVersionId() !== data.version) return action
        return { ...action, edit: { edits: workspaceEdits(resolved, data.client, data.model) } }
      },
    }, { providedCodeActionKinds: ['quickfix', 'source.organizeImports'] })
  }
}

function workspaceEdits(action: LspItem, client: AssistClient, model: monaco.editor.ITextModel): monaco.languages.IWorkspaceTextEdit[] {
  const changes: TextEdit[] = [...(action.edit?.changes?.[client.uri] ?? [])]
  for (const change of action.edit?.documentChanges ?? []) {
    if (change.textDocument?.uri === client.uri) changes.push(...(change.edits ?? []))
  }
  return changes.map(change => ({ resource: model.uri, versionId: model.getVersionId(), textEdit: edit(change) }))
}

class AssistClient {
  readonly uri: string
  private id = Array.from(crypto.getRandomValues(new Uint8Array(16)), b => b.toString(16).padStart(2, '0')).join('')
  private revision = 1
  private disposed = false
  private diagnosticsAttempts = 0
  private readyRevision = 0
  private timer: number | undefined
  private completionWork: Promise<unknown> | undefined
  private completionWorkKey = ''
  private completionCache = new Map<string, { expires: number; value: unknown }>()
  private backoffUntil = 0
  private controllers = new Set<AbortController>()
  private diagnosticController: AbortController | undefined
  private warmup: Promise<void> | undefined
  private preparing = true
  private connected = false
  private subscription: monaco.IDisposable
  private model: monaco.editor.ITextModel
  private language: string
  private auth: Auth
  private status: (text: string) => void
  private ready: () => void
  constructor(model: monaco.editor.ITextModel, language: string, auth: Auth, status: (text: string) => void, ready: () => void) {
    this.model = model; this.language = language; this.auth = auth; this.status = status; this.ready = ready
    this.uri = `file:///work/${language === 'go' ? 'main.go' : language === 'python' ? 'main.py' : 'Program.cs'}`
    this.subscription = model.onDidChangeContent(() => {
      this.revision++
      this.completionCache.clear()
      this.diagnosticsAttempts = 0
      monaco.editor.setModelMarkers(model, 'contest-lsp', [])
      this.schedule()
    })
    // Start as soon as the task/language is mounted, before the student types.
    // Completion must not cancel this request while the sandbox initializes.
    this.warmup = this.diagnostics().finally(() => {
      this.preparing = false
      this.warmup = undefined
      // Edits made during initialization still need fresh diagnostics.
      if (!this.disposed) this.schedule(0)
    })
  }
  private schedule(delay = 700) {
    if (this.disposed) return
    window.clearTimeout(this.timer)
    this.timer = window.setTimeout(() => { void this.diagnostics() }, delay)
  }
  private async diagnostics() {
    this.diagnosticsAttempts++
    const revision = this.revision
    const result: any = await this.request('diagnostics')
    if (this.disposed || revision !== this.revision) return
    if (result?.pending) { this.schedule(result.queued ? 250 : 1200); return }
    if (!Array.isArray(result)) return
    if (this.readyRevision !== revision) { this.readyRevision = revision; this.ready() }
    monaco.editor.setModelMarkers(this.model, 'contest-lsp', result.map((item: any) => ({
      startLineNumber: item.range.start.line + 1, startColumn: item.range.start.character + 1,
      endLineNumber: item.range.end.line + 1, endColumn: item.range.end.character + 1,
      message: item.message, code: item.code == null ? undefined : String(item.code), source: item.source,
      severity: item.severity === 2 ? monaco.MarkerSeverity.Warning : item.severity === 3 ? monaco.MarkerSeverity.Info : item.severity === 4 ? monaco.MarkerSeverity.Hint : monaco.MarkerSeverity.Error,
    })))
    if (this.language === 'go' && result.some((item: any) => /undefined:/.test(item.message))) {
      const actions: any = await this.request('imports')
      if (this.disposed || revision !== this.revision) return
      for (let action of Array.isArray(actions) ? actions : []) {
        if (!action.edit && action.data !== undefined) action = await this.request('resolveAction', undefined, action)
        if (!action || this.disposed || revision !== this.revision) return
        const edits = workspaceEdits(action, this, this.model).map(change => change.textEdit)
        // Only accept edits that introduce an import, not unrelated quick fixes.
        const introduced = edits.some(change => Array.from(change.text.matchAll(/"([^"\n]+)"/g)).some(match => !this.model.getValue().includes(`"${match[1]}"`)))
        if (introduced && edits.length) {
          this.model.pushEditOperations([], edits, () => null)
          return
        }
      }
    }
    if (this.diagnosticsAttempts < 4) this.schedule(1200)
  }
  async complete(position: monaco.Position, cancel: monaco.CancellationToken): Promise<unknown> {
    if (this.disposed || cancel.isCancellationRequested) return null
    const key = `${this.revision}:${position.lineNumber}:${position.column}`
    const cached = this.completionCache.get(key)
    if (cached && cached.expires > Date.now()) return cached.value
    // At most one background completion per editor. Typing does not cancel a
    // cold start or pile up requests; the next refresh uses the newest source.
    if (!this.completionWork) {
      this.completionWorkKey = key
      const revision = this.revision
      this.completionWork = this.request('completion', position).then(result => {
        if (!this.disposed && (revision !== this.revision || (result != null && !(result as any).pending))) {
          window.setTimeout(() => { if (!this.disposed) this.ready() }, 0)
        }
        return result
      }).finally(() => { this.completionWork = undefined })
    }
    if (this.completionWorkKey !== key) return { pending: true }
    return withinCompletionBudget(this.completionWork, { pending: true })
  }
  async request(action: string, position?: monaco.Position, item?: LspItem, cancel?: monaco.CancellationToken): Promise<unknown> {
    if (this.disposed || cancel?.isCancellationRequested) return null
    if (action === 'completion' && this.warmup) {
      await this.warmup
      if (this.disposed || cancel?.isCancellationRequested) return null
    }
    const cacheKey = `${this.revision}:${position?.lineNumber ?? 0}:${position?.column ?? 0}`
    if (action === 'completion') {
      const cached = this.completionCache.get(cacheKey)
      if (cached && cached.expires > Date.now()) return cached.value
    }
    if (Date.now() < this.backoffUntil) return null
    if (action === 'diagnostics' && this.controllers.size) { this.schedule(750); return null }
    if (action === 'completion') this.diagnosticController?.abort()
    const source = this.model.getValue()
    if (new TextEncoder().encode(source).length > 256 * 1024) { this.status('Подсказки: код превышает 256 КиБ.'); return null }
    const revision = this.revision
    const controller = new AbortController()
    this.controllers.add(controller)
    if (action === 'diagnostics') this.diagnosticController = controller
    const subscription = cancel?.onCancellationRequested(() => controller.abort())
    try {
      if (!this.connected) this.status(this.preparing ? 'Подготавливаем подсказки… Можно читать задачу и писать код.' : 'Анализатор загружает проект…')
      const response = await fetch(`${API}/api/v1/tasks/${encodeURIComponent(this.auth.taskId)}/assist`, {
        method: 'POST', headers: { Authorization: `Bearer ${this.auth.token}`, 'Content-Type': 'application/json' },
        signal: AbortSignal.any([controller.signal, AbortSignal.timeout(55000)]),
        body: JSON.stringify({ clientId: this.id, language: this.language, action, source, revision,
          position: { line: (position?.lineNumber ?? 1) - 1, character: (position?.column ?? 1) - 1 }, item }),
      })
      if (this.disposed || revision !== this.revision) return null
      if (response.status === 409) return null
      if (!response.ok) {
        this.backoffUntil = Date.now() + (response.status === 429 ? 2000 : 10000)
        this.diagnosticsAttempts = 0
        this.status(response.status === 503 ? 'Подсказки отключены — преподавателю нужно запустить сервис анализаторов.' : response.status === 429 ? 'Анализаторы заняты. Повторим запрос.' : 'Не удалось получить подсказки.')
        this.schedule(this.backoffUntil - Date.now() + 100)
        return null
      }
      const data = await response.json()
      if (this.disposed || revision !== this.revision) return null
      if (data.pending) {
        if (data.queued) this.status('Подсказки: ожидание свободного анализатора…')
        else if (!this.connected) this.status('Анализатор загружает проект…')
        if (action === 'completion') { this.readyRevision = 0; this.schedule(data.queued ? 250 : 1200) }
        return { pending: true, queued: data.queued === true }
      }
      this.connected = true
      this.status('Подсказки подключены')
      if (action === 'completion' && data.result != null) {
        if (this.completionCache.size >= 8) this.completionCache.clear()
        this.completionCache.set(cacheKey, { expires: Date.now() + 2000, value: data.result })
      }
      return data.result
    } catch {
      if (!this.disposed && !controller.signal.aborted) {
        this.status('Нет связи с анализатором. Черновик сохраняется отдельно.')
        this.schedule(10000)
      }
      return null
    } finally {
      subscription?.dispose()
      this.controllers.delete(controller)
      if (this.diagnosticController === controller) this.diagnosticController = undefined
    }
  }
  dispose() {
    this.disposed = true
    this.subscription.dispose()
    window.clearTimeout(this.timer)
    this.completionCache.clear()
    this.controllers.forEach(controller => controller.abort())
    if (!this.model.isDisposed()) monaco.editor.setModelMarkers(this.model, 'contest-lsp', [])
    void fetch(`${API}/api/v1/tasks/${encodeURIComponent(this.auth.taskId)}/assist`, {
      method: 'POST', keepalive: true,
      headers: { Authorization: `Bearer ${this.auth.token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ clientId: this.id, language: this.language, action: 'close', source: '', revision: this.revision, position: { line: 0, character: 0 } }),
    }).catch(() => {})
  }
}
export function attachAssist(model: monaco.editor.ITextModel, language: string, auth: Auth, status: (text: string) => void, ready: () => void = () => {}) {
  installProviders()
  const client = new AssistClient(model, language, auth, status, ready)
  clients.set(model.uri.toString(), client)
  return { dispose() {
    if (clients.get(model.uri.toString()) === client) clients.delete(model.uri.toString())
    client.dispose()
  } }
}
