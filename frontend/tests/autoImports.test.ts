import assert from 'node:assert/strict'
import test from 'node:test'
import { getImportSuggestions } from '../src/lib/autoImports.ts'

function insert(language: string, source: string, label: string) {
  const suggestion = getImportSuggestions(language, source, label).find(item => item.label === label)
  assert.ok(suggestion)
  return source.slice(0, suggestion.offset) + suggestion.text + source.slice(suggestion.offset)
}
test('Go autoimport preserves the package and uses an existing empty import block', () => {
  const result = insert('go', 'package main\n\nimport (\n)\n\nfunc main() { fm }\n', 'fmt')
  assert.match(result, /^package main/)
  assert.match(result, /import\s*\([\s\S]*"fmt"[\s\S]*\)/)
  assert.equal(getImportSuggestions('go', result, 'fmt').length, 0)
  assert.match(insert('go', 'package main\nfunc main() { os }', 'os'), /package main\n\nimport "os"/)
})
test('Python preserves future imports and offers both modules and imported names', () => {
  const result = insert('python', '"""Module."""\nfrom __future__ import annotations\n\ndef main():\n    os\n', 'os')
  assert.ok(result.indexOf('import os') > result.indexOf('from __future__'))
  assert.equal(getImportSuggestions('python', result, 'os').length, 0)
  assert.match(insert('python', 'Counter()', 'Counter'), /^from collections import Counter\n/)
  assert.ok(getImportSuggestions('python', 'from os import path\nos', 'os').length > 0)
})
test('C# using is inserted once and existing namespaces are recognized', () => {
  const result = insert('csharp', 'using System;\nclass Program { File }', 'File')
  assert.match(result, /^using System.IO;\nusing System;/)
  assert.equal(getImportSuggestions('csharp', result, 'File').length, 0)
  assert.equal(getImportSuggestions('csharp', 'global using System.IO;\nclass P {}', 'File').length, 0)
})
