"""Time the Python picker's startup path, for comparison with the Go binary.

Run it from the project folder:

    python startup_probe.py

It prints how long the Python picker needs to reach the point where the
branch list is ready, which is the number the Go binary is measured against.
"""
import importlib.util
import os
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))

spec = importlib.util.spec_from_file_location(
    "git_picker", os.path.join(HERE, "git_picker.py")
)
gp = importlib.util.module_from_spec(spec)
sys.modules["git_picker"] = gp
spec.loader.exec_module(gp)

t0 = time.perf_counter()
root = gp.find_repo_root(os.getcwd())
branches = gp.list_branches(root)
current = gp.current_branch(root)
elapsed = (time.perf_counter() - t0) * 1000
print(f"branches={len(branches)} in-process={elapsed:.0f} ms")
