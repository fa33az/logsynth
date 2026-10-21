package emit

// Python helper blocks emitted after the parser body.

const pyHelpers = `
def _as_str(value: object) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return str(value)


def _as_int(value: object) -> Optional[int]:
    if value is None or value == "" or value == "-":
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def _as_float(value: object) -> Optional[float]:
    if value is None or value == "" or value == "-":
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def _as_bool(value: object) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.lower() in ("true", "1")
    return bool(value)

`

const pyPairHelper = `
def _pairs(line: str) -> dict[str, str]:
    """Split key=value pairs, unwrapping double-quoted values."""
    out: dict[str, str] = {}
    for key, raw in _PAIR.findall(line):
        if len(raw) >= 2 and raw[0] == '"' and raw[-1] == '"':
            raw = raw[1:-1].replace('\\"', '"').replace("\\\\", "\\")
        out[key] = raw
    return out

`

const pyFlattenHelper = `
def _flatten(prefix: str, value: dict[str, Any], out: dict[str, Any], depth: int = 3) -> None:
    """Flatten nested objects into dotted keys, matching the schema names."""
    for key, item in value.items():
        full = key if not prefix else prefix + "." + key
        if isinstance(item, dict) and depth > 1:
            _flatten(full, item, out, depth - 1)
            continue
        out[full] = item

`
