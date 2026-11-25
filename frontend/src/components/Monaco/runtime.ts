import * as monaco from 'monaco-editor/editor/editor.api.js'
import EditorWorker from 'monaco-editor/editor/editor.worker.js?worker'
import 'monaco-editor/languages/definitions/go/register.js'
import 'monaco-editor/languages/definitions/python/register.js'
import 'monaco-editor/languages/definitions/csharp/register.js'

import 'monaco-editor/features/find/register.js'
import 'monaco-editor/features/bracketMatching/register.js'
import 'monaco-editor/features/comment/register.js'

// In Monaco 0.57 the suggest feature entry registers inline completions only.
// The suggestion widget and editing keybindings must be loaded explicitly.
import 'monaco-editor/editor/browser/coreCommands.js'
import 'monaco-editor/editor/contrib/suggest/browser/suggestController.js'
import 'monaco-editor/editor/contrib/wordOperations/browser/wordOperations.js'
import 'monaco-editor/editor/contrib/clipboard/browser/clipboard.js'
import 'monaco-editor/features/hover/register.js'
import 'monaco-editor/features/parameterHints/register.js'
import 'monaco-editor/features/codeAction/register.js'
import 'monaco-editor/features/format/register.js'

self.MonacoEnvironment = { getWorker: () => new EditorWorker() }
export { monaco }
