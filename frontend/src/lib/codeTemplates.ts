import type { Language } from './languages'

export const codeTemplates: Record<Language, string> = {
  go: 'package main\n\nimport (\n\n)\n\nfunc main() {\n\n}\n',
  python: 'def main():\n    pass\n\n\nif __name__ == "__main__":\n    main()\n',
  csharp: 'using System;\n\nclass Program\n{\n    static void Main()\n    {\n\n    }\n}\n',
}
export const inputExamples: Record<Language, string> = {
  go: '// Внутри main(); импорт: \"os\"\ndata, err := os.ReadFile("input.txt")\nif err != nil {\n    panic(err)\n}',
  python: 'with open("input.txt", encoding="utf-8") as file:\n    data = file.read()',
  csharp: '// Внутри Main(); добавьте using System.IO;\nvar data = File.ReadAllText("input.txt");',
}

export const outputExamples: Record<Language, string> = {
  go: '// Внутри main(); импорт: "os"\nstdout, err := os.Create("stdout")\nif err != nil {\n    panic(err)\n}\ndefer stdout.Close()',
  python: 'with open("stdout", "w", encoding="utf-8") as stdout:\n    pass',
  csharp: '// Внутри Main(); добавьте using System.IO;\nusing var stdout = new StreamWriter("stdout");',
}
