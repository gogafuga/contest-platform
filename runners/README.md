# Выполнение Go, Python и C#

В редакторе кнопка «Выполнить и проверить» сохраняет черновик, отправляет снимок
кода в очередь и показывает результат. Правильный stdout начисляет полные баллы
через существующий Judge; неверный ответ получает ноль и допускает новую попытку.

Публичный API принимает до 32 незавершённых запусков, включая выполняющиеся;
у одного участника одновременно может быть только один запуск. Очередь FIFO,
два worker и два изолированных контейнера остаются прежними. Дополнительные
ожидающие задания не получают контейнер до своей очереди. При максимальном
размере исходника очередь хранит до 8 МиБ кода; лимиты частоты и истории сохранены.
Файл с ответом по-прежнему можно отправить вручную.

Программа получает настоящий dataset задачи сразу тремя способами:
`input.txt`, `<taskId>.txt` в рабочем каталоге и стандартный ввод. Ответ нужно
печатать в stdout. Эталонный ответ в контейнер не передаётся. Доступны стандартные
библиотеки; загрузка пакетов из сети отключена.

## Локальная установка

Нужен официальный Docker, доступный через `/usr/bin/docker`, и gVisor `runsc`.
Исполнение без gVisor запрещено в supervisor: запасного запуска через runc нет.

Из корня репозитория:

```bash
./scripts/install-runsc.sh
```

Скрипт скачивает официальный архив с runsc и каталогом gvisor-bin, проверяет
SHA-512 и регистрирует runtime. Для активации выполните reload, сохраняющий
работающие контейнеры:

```bash
sudo systemctl reload docker
sudo /usr/bin/docker info --format '{{json .Runtimes}}'
```

В списке должен появиться `runsc`. Установщик сам Docker не перезагружает.
Соберите образы и запустите отдельный supervisor:

```bash
./scripts/runner.sh build
./scripts/runner.sh start
```

Оставьте supervisor работающим. Он создаёт приватный Unix socket
`/run/contest-runner/run.sock`, доступный группе пользователя, запустившего скрипт.
Публичный backend продолжает работать без root и без доступа к Docker socket.
`RUNNER_SOCKET` позволяет изменить путь клиента, если supervisor настроен отдельно.

Для существующей базы примените миграцию `backend/db/migrations/006_code_runs.sql`
один раз; на новой базе она применяется с остальными миграциями. Перезапустите backend.

Проверьте реальное исполнение и ограничения через приватный сервис:

```bash
cd backend
go run ./cmd/check-runner
```

Эта проверка исполняет фиксированные тестовые программы только внутри runner,
проверяя три языка, оба имени файла, stdin, ошибку компиляции, ошибку выполнения,
таймаут и переполнение вывода. Обычные unit/API тесты не заменяют эту проверку.

## Границы и ограничения

Каждый запуск компилируется и выполняется в новом контейнере с `--runtime=runsc`,
без сети, bind mounts, секретов, Docker socket, дополнительных capabilities и root.
Корневая файловая система read-only; временные `/tmp` и `/work` имеют noexec,
`/exec` разрешает выполнение собранного бинарника. Лимиты: 1 CPU, 1 ГиБ памяти,
128 процессов, 256 файловых дескрипторов, 45 секунд компиляции и 5 секунд выполнения.
Внешний таймаут — 75 секунд. Общий stdout+stderr ограничен 1 МиБ.
После выполнения контейнер принудительно удаляется; ошибка удаления блокирует
приём новых работ до перезапуска supervisor и успешной очистки.

Supervisor принимает два одновременных запуска. Backend рассчитан на **один
экземпляр**: до восьми активных/ожидающих работ, по одной на участника, максимум
10 новых запусков в минуту. Идентификатор запроса предотвращает повторное
выполнение после потери HTTP-ответа. После перезапуска backend незавершённые
работы помечаются прерванными при чтении результата; автоматически повторять их нельзя.

В PostgreSQL сохраняются код и результат: до 100 запусков на участника и 2000
на весь конкурс. При достижении лимита новые запуски отклоняются. Сохраняются
первые 64 КиБ каждого потока, но Judge проверяет полный stdout в пределах 1 МиБ.
Начисление баллов и финальный результат записываются в одной транзакции.
Результаты доступны только своему участнику. Недоступный runner не начисляет баллы.

Это локальная реализация с дополнительной изоляцией gVisor, а не гарантия
неуязвимости. Для публичного конкурса runner следует размещать на отдельной
машине; перед мероприятием нужны проверки контейнеров, параллельных запусков,
памяти и производительности на фактическом компьютере.

C# limits managed GC heap to 256 MiB per process, uses one logical processor,
disables workload resolution and build servers, and builds with one MSBuild node.
The overall sandbox memory limit remains 1 GiB.

The per-file RLIMIT_FSIZE is 32 MiB for Go/Python and 128 MiB for C#.
.NET's executable-code mapper also depends on this limit: the 32 MiB setting
caused Roslyn to report Out of memory for a minimal program. A fixed console
fixture passed under gVisor at 128 MiB on the deployment machine. The total
sandbox memory (1 GiB), tmpfs capacities and output limit remain independent.

## Go build cache

The Go image prebuilds common standard-library packages with matching trimpath
settings. Its entrypoint copies the immutable trusted cache to the sandbox's
private /tmp/go-cache. Fresh user code is compiled on every run; participant cache
entries disappear with the container and are never reused by other participants.
Rebuild with `sudo /usr/bin/docker build -f runners/go/Dockerfile -t contest-run-go:local .`.
The UI reports total compile+run time, not execution time alone.

## Named answer file (user-requested revision)

Participants can write a regular file named `stdout` (no extension) in /work.
After a successful program exit run-agent reads that file with no-follow and
non-blocking open, verifies regular-file type and enforces the same 1 MiB combined
answer/stderr limit. If present, it is the graded answer, even if empty; console
output is the backward-compatible fallback when no file exists. Templates show
os.Create + fmt.Fprintln in Go, open + print(file=...) in Python and StreamWriter
in C#. All three runner images must be rebuilt to activate file collection;
cmd/check-runner now tests each language's answer-file path.

## Compilation startup optimization

C# now calls the SDK Roslyn csc.dll directly with a trusted build-time response
file containing the net10 reference assemblies and fixed paths/options. No
MSBuild, restore, package downloads or participant build scripts are invoked.
The image build compiles and runs a fixed trusted fixture before it is tagged.
Python's trusted in-container launcher compiles and executes source in one
interpreter; syntax failure exits 65 and is reported as compile_error. A user
program explicitly exiting 65 also receives this error classification (never
points). Go retains a private copy of the trusted precompiled standard-lib cache.
All participant compilation/execution remains inside fresh mandatory gVisor
containers with the existing network/filesystem/resource constraints. No shared
participant cache, relaxed security options or host-side execution is introduced.
Rebuild all three runner images, then run cmd/check-runner to measure real
compile/run and wall times and verify syntax errors, I/O and limits.
