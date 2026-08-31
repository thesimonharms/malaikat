#!/usr/bin/env python3
"""Read key GGUF hyperparameters and estimate KV-cache footprint per context.

Prints JSON: {arch, n_layers, n_kv_heads, head_dim, kv_bytes_per_token,
              kv_gib_at_256k, kv_gib_at_512k, kv_gib_at_1m}
KV type assumed q8_0 (1 byte/element) for both K and V, matching
--cache-type-k/v q8_0 used by coding.yaml.
"""
import json
import struct
import sys

MAGIC = b"GGUF"
# GGUF metadata value types
T_UINT8, T_INT8, T_UINT16, T_INT16, T_UINT32, T_INT32, T_FLOAT32, T_BOOL, \
    T_STRING, T_ARRAY, T_UINT64, T_INT64, T_FLOAT64 = range(13)


def read_value(buf, off, t):
    if t == T_UINT8:
        return struct.unpack_from("<B", buf, off)[0], off + 1
    if t == T_INT8:
        return struct.unpack_from("<b", buf, off)[0], off + 1
    if t == T_UINT16:
        return struct.unpack_from("<H", buf, off)[0], off + 2
    if t == T_INT16:
        return struct.unpack_from("<h", buf, off)[0], off + 2
    if t == T_UINT32:
        return struct.unpack_from("<I", buf, off)[0], off + 4
    if t == T_INT32:
        return struct.unpack_from("<i", buf, off)[0], off + 4
    if t == T_FLOAT32:
        return struct.unpack_from("<f", buf, off)[0], off + 4
    if t == T_BOOL:
        return bool(struct.unpack_from("<B", buf, off)[0]), off + 1
    if t == T_UINT64:
        return struct.unpack_from("<Q", buf, off)[0], off + 8
    if t == T_INT64:
        return struct.unpack_from("<q", buf, off)[0], off + 8
    if t == T_FLOAT64:
        return struct.unpack_from("<d", buf, off)[0], off + 8
    if t == T_STRING:
        (n,) = struct.unpack_from("<Q", buf, off)
        off += 8
        s = buf[off:off + n].decode("utf-8", "replace")
        return s, off + n
    if t == T_ARRAY:
        (et,) = struct.unpack_from("<I", buf, off)
        off += 4
        (n,) = struct.unpack_from("<Q", buf, off)
        off += 8
        vals = []
        for _ in range(n):
            v, off = read_value(buf, off, et)
            vals.append(v)
        return vals, off
    raise ValueError("unknown gguf type %d" % t)


def main():
    path = sys.argv[1]
    with open(path, "rb") as f:
        header = f.read(1 << 26)  # first 64 MiB holds metadata
    assert header[:4] == MAGIC, "not a GGUF file"
    off = 4
    (version,) = struct.unpack_from("<I", header, off); off += 4
    (n_tensors,) = struct.unpack_from("<Q", header, off); off += 8
    (n_kv,) = struct.unpack_from("<Q", header, off); off += 8
    kv = {}
    for _ in range(n_kv):
        (klen,) = struct.unpack_from("<Q", header, off)
        off += 8
        key = header[off:off + klen].decode("utf-8", "replace")
        off += klen
        (vt,) = struct.unpack_from("<I", header, off)
        off += 4
        val, off = read_value(header, off, vt)
        kv[key] = val

    arch = kv.get("general.architecture")
    p = arch + "." if arch else ""
    n_layers = kv.get(p + "block_count")
    emb = kv.get(p + "embedding_length")
    n_heads = kv.get(p + "attention.head_count")
    n_kv_heads = kv.get(p + "attention.head_count_kv")
    key_len = kv.get(p + "attention.key_length")
    val_len = kv.get(p + "attention.value_length")
    head_dim = key_len or (emb // n_heads if emb and n_heads else None)
    # KV cache elements per token (K and V): layers * kv_heads * head_dim
    kv_elem_per_tok = n_layers * n_kv_heads * head_dim
    # q8_0 -> 1 byte per element for K and V
    kv_bytes_per_token = 2 * kv_elem_per_tok * 1

    def gib(n_ctx):
        return round(kv_bytes_per_token * n_ctx / (1 << 30), 2)

    out = {
        "arch": arch,
        "n_layers": n_layers,
        "n_kv_heads": n_kv_heads,
        "head_dim": head_dim,
        "kv_bytes_per_token": kv_bytes_per_token,
        "kv_gib_256k": gib(262144),
        "kv_gib_512k": gib(524288),
        "kv_gib_1m": gib(1048576),
    }
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
