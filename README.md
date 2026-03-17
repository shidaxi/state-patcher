# state-patcher

Modify contract storage slots in a geth PebbleDB database.

Built against go-ethereum v1.14.12.

## Usage

```bash
state-patcher \
  --datadir /path/to/geth/datadir \
  --set 0xAddr:0xSlot=0xValue \
  --set 0xAddr:0xSlot=0xValue
```

### Flags

| Flag | Description |
|------|-------------|
| `--datadir` | geth data directory (required) |
| `--set` | storage patch `0xAddr:0xSlot=0xValue` (repeatable) |
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
```
