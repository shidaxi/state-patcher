# state-patcher

Modify contract storage slots in a geth PebbleDB database.

Built against go-ethereum v1.14.12.

## Usage

```bash
# Inline patches
state-patcher \
  --datadir /path/to/geth/datadir \
  --set 0xAddr:0xSlot=0xValue \
  --set 0xAddr:0xSlot=0xValue

# From a JSON file
state-patcher --datadir /path/to/geth/datadir --file patch.json

# From a YAML file
state-patcher --datadir /path/to/geth/datadir --file patch.yaml

# Combine file and inline patches
state-patcher --datadir /path/to/geth/datadir --file patch.yaml --set 0xAddr:0xSlot=0xValue
```

### Patch file format

JSON:

```json
{
  "0xAddr": {
    "0xSlot": "0xValue"
  }
}
```

YAML:

```yaml
"0xAddr":
  "0xSlot": "0xValue"
```

### Flags

| Flag | Description |
|------|-------------|
| `--datadir` | geth data directory (required) |
| `--set` | storage patch `0xAddr:0xSlot=0xValue` (repeatable) |
| `--file` | patch file in JSON or YAML format (`.json`, `.yaml`, `.yml`) |
| `--dry-run` | validate inputs only, do not modify database |

## Build

```bash
make build
```

### Local snapshot build (goreleaser)

```bash
make snapshot
```

## Docker

```bash
docker run --rm -v /path/to/datadir:/data ghcr.io/shidaxi/state-patcher \
  --datadir /data \
  --set 0xAddr:0xSlot=0xValue

# Or with a patch file
docker run --rm \
  -v /path/to/datadir:/data \
  -v /path/to/patch.yaml:/patch.yaml \
  ghcr.io/shidaxi/state-patcher \
  --datadir /data --file /patch.yaml
```
