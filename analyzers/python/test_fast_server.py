"""Pinned pylsp/rope smoke test; only trusted local fixtures are indexed."""
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from rope.base.project import Project
from rope.contrib.autoimport.sqlite import AutoImport
from pylsp.plugins import rope_autoimport
from fast_server import use_prebuilt_libraries


class PreparedIndexTest(unittest.TestCase):
    def test_local_names_and_library_autoimports_survive_without_rescan(self):
        with tempfile.TemporaryDirectory() as root:
            project = Project(root, ropefolder='.ropeproject')
            index = AutoImport(project, memory=False)
            index.generate_modules_cache(modules=['collections'], single_thread=True)
            cache = Path(project.ropefolder.real_path)
            Path(root, 'main.py').write_text('def local_fixture():\n    pass\n')
            original = rope_autoimport.AutoimportCache._reload_cache
            try:
                use_prebuilt_libraries(cache)
                from rope.base.taskhandle import DEFAULT_TASK_HANDLE
                with patch.object(index, 'generate_modules_cache', side_effect=AssertionError('unexpected library rescan')), \
                     patch.object(rope_autoimport, 'PylspTaskHandle', return_value=DEFAULT_TASK_HANDLE):
                    rope_autoimport.AutoimportCache()._reload_cache(None, index)
                self.assertIn('collections', index.get_modules('Counter'))
                self.assertIn('main', index.get_modules('local_fixture'))
            finally:
                rope_autoimport.AutoimportCache._reload_cache = original
                index.close()
                project.close()

    def test_missing_prepared_index_fails(self):
        with tempfile.TemporaryDirectory() as root:
            with self.assertRaises(RuntimeError):
                use_prebuilt_libraries(Path(root))


if __name__ == '__main__':
    unittest.main()
