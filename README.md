# st8

st8 is a transactional in-memory store for Go with file-backed persistence.
It is built for app state where you want simple transactions without pulling in a DB.

## Guarantees

- `Update` is atomic for in-process state.
- If `Update` returns an error, in-memory state is unchanged.
- Commit happens when st8 atomically replaces the state file.

## Durability

- st8 syncs directory metadata on supported platforms as best effort.
- That best-effort sync does not change commit success or failure.

## Scope

- st8 coordinates goroutines in one process.
- st8 does not coordinate multiple processes writing the same file.

## License

This project is licensed under the [MIT License](https://github.com/dasvh/st8/raw/main/LICENSE).
