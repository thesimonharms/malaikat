import os
import sys

from huggingface_hub import hf_hub_download

if sys.platform == "win32":
    base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
else:
    base = os.environ.get("XDG_DATA_HOME") or os.path.join(os.path.expanduser("~"), ".local", "share")
dest = os.path.join(base, "malaikat", "models", "Qwen3.8-27B-MTP")
os.makedirs(dest, exist_ok=True)
print("dest=", dest, flush=True)
p = hf_hub_download(
    repo_id="unsloth/Qwen3.8-27B-GGUF",
    filename="Qwen3.8-27B-UD-Q4_K_XL.gguf",
    local_dir=dest,
)
print("done=", p, flush=True)
print("size_gb=", round(os.path.getsize(p) / 1e9, 2), flush=True)
