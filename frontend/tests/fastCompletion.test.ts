import assert from 'node:assert/strict'
import test from 'node:test'
import { standardMembers } from '../src/lib/standardMembers.ts'
import { withinCompletionBudget } from '../src/lib/completionBudget.ts'

test('standard receivers work before the language server is available', () => {
  for (const [language, source, expected] of [
    ['go', '  fmt.Pr', 'Println'], ['go', 'data, err := os.', 'ReadFile'],
    ['python', 'value = os.path.ba', 'basename'], ['python', 'sys.stdout', 'stdout'],
    ['csharp', 'Console.Writeln', 'WriteLine'], ['csharp', 'File.Re', 'ReadAllText'],
    ['csharp', 'System.Console.Wri', 'WriteLine'],
  ]) {
    assert.ok(standardMembers(language, source).some(item => item.label === expected), source)
  }
})
test('unknown variables and non-member positions do not get invented members', () => {
  for (const language of ['go', 'python', 'csharp']) {
    assert.deepEqual(standardMembers(language, 'student.'), [])
    assert.deepEqual(standardMembers(language, 'value + '), [])
    assert.deepEqual(standardMembers(language, ''), [])
  }
})
test('a cold server cannot block the initial completion list', async () => {
  let finish!: (value: string) => void
  const server = new Promise<string>(resolve => { finish = resolve })
  assert.equal(await withinCompletionBudget(server, 'local', 10), 'local')
  // The original request remains usable for a later semantic refresh.
  finish('semantic')
  assert.equal(await server, 'semantic')
})
test('already available server results win and failures are handled', async () => {
  assert.equal(await withinCompletionBudget(Promise.resolve('semantic'), 'local', 100), 'semantic')
  await assert.rejects(withinCompletionBudget(Promise.reject(new Error('offline')), 'local', 100), /offline/)
})
