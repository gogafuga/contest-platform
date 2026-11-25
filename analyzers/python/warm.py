"""Build indexes from trusted installed libraries, never participant code."""
import argparse
import os
from pathlib import Path
import shutil
import jedi
from rope.base.project import Project
from rope.contrib.autoimport.sqlite import AutoImport

parser = argparse.ArgumentParser()
parser.add_argument('--project-root', default='/work')
parser.add_argument('--cache-root', default='/opt')
parser.add_argument('--jedi-root', default='/tmp/cache')
options = parser.parse_args()
project_root = Path(options.project_root)
cache_root = Path(options.cache_root)
jedi_root = Path(options.jedi_root)
project_root.mkdir(parents=True, exist_ok=True)
cache_root.mkdir(parents=True, exist_ok=True)
# Rope interprets resource paths relative to the project, even when they look
# absolute. Use a relative folder and copy its actual resolved path.
project = Project(str(project_root), ropefolder='.ropeproject')
rope_root = Path(project.ropefolder.real_path)
index = AutoImport(project, memory=False)
index.generate_modules_cache(single_thread=True)
index.close()
project.close()
if not (rope_root / 'autoimport.db').is_file():
    raise RuntimeError('Rope did not create its persistent autoimport index')
shutil.copytree(rope_root, cache_root / 'rope-cache')
jedi.settings.cache_directory = str(jedi_root / 'jedi')
for module, member in [('os', 'pa'), ('sys', 'std'), ('re', 'se'), ('json', 'lo'), ('collections', 'Co')]:
    jedi.Script(f'import {module}\n{module}.{member}').complete(2, len(module) + 1 + len(member))
jedi_root.mkdir(parents=True, exist_ok=True)
shutil.copytree(jedi_root, cache_root / 'jedi-cache')
for folder in [cache_root / 'rope-cache', cache_root / 'jedi-cache']:
    for root, dirs, files in os.walk(folder):
        os.chmod(root, 0o755)
        for filename in files:
            os.chmod(os.path.join(root, filename), 0o644)
