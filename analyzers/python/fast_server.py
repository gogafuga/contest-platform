"""Use the immutable, build-time Rope library index in each private sandbox.

The image pins pylsp/rope versions. Local project indexing remains enabled;
only redundant rescanning of the fixed, read-only installed libraries is skipped.
"""
from pathlib import Path


def use_prebuilt_libraries(cache_root=Path('/opt/rope-cache')):
    if not (cache_root / 'autoimport.db').is_file():
        raise RuntimeError('Prepared Rope library index is missing')

    from pylsp.plugins import rope_autoimport

    if not hasattr(rope_autoimport.AutoimportCache, '_reload_cache'):
        raise RuntimeError('Unsupported pylsp Rope cache interface')

    def reload_project(self, workspace, autoimport, resources=None):
        task_handle = rope_autoimport.PylspTaskHandle(workspace)
        autoimport.generate_cache(task_handle=task_handle, resources=resources)

    rope_autoimport.AutoimportCache._reload_cache = reload_project


if __name__ == '__main__':
    use_prebuilt_libraries()
    from pylsp.__main__ import main
    main()
