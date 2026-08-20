import os
import sys

from huggingface_hub import hf_hub_download

if sys.platform == "win32":
    base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
else:
    base = os.environ.get("XDG_DATA_HOME") or os.path.join(os.path.expanduser("~"), ".local", "share")
dest = os.path.join(base, "malaikat", "models", "Ornith-1.5-35B-A3B")
os.makedirs(dest, exist_ok=True)
print("dest=", dest, flush=True)
p = hf_hub_download(
    repo_id="ornith-ai/Ornith-1.5-35B-A3B-GGUF",
    filename="Ornith-1.5-35B-Q4_K_M.gguf",
    local_dir=dest,
)
print("done=", p, flush=True)
print("size_gb=", round(os.path.getsize(p) / 1e9, 2), flush=True)
