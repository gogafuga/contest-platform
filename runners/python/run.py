"""Trusted in-container launcher: compile syntax and execute in one interpreter."""
import sys
import tokenize
import traceback
import types

with tokenize.open("/work/main.py") as source_file:
    source = source_file.read()
try:
    program = compile(source, "/work/main.py", "exec")
except SyntaxError as error:
    traceback.print_exception(error, file=sys.stderr)
    raise SystemExit(65)

sys.argv = ["/work/main.py"]
module = types.ModuleType("__main__")
module.__file__ = "/work/main.py"
module.__package__ = None
module.__cached__ = None
sys.modules["__main__"] = module
exec(program, module.__dict__)
