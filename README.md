# storx

Modern typed APIs for embedded Go storage engines.

`storx` is a Go workspace with separate modules for shared codecs, key codecs, engine-specific wrappers, and lightweight observation hooks.

## Modules

- `github.com/arcgolabs/storx` - shared errors and internal helpers
- `github.com/arcgolabs/storx/observer` - pluggable operation observers
- `github.com/arcgolabs/storx/codec` - typed value codecs
- `github.com/arcgolabs/storx/keycodec` - typed key codecs with ordering support
- `github.com/arcgolabs/storx/bboltx` - typed bucket API for bbolt
- `github.com/arcgolabs/storx/badgerx` - typed namespace API for Badger

## Design Boundary

`storx` does not try to flatten every engine into one generic KV interface.

- `bboltx` keeps the `DB -> Bucket -> Tx -> Cursor` model.
- `badgerx` keeps the `DB -> Namespace -> Txn -> Prefix Scan / TTL` model.
- `codec` and `keycodec` are the main reusable shared layers.
- `observer` lets you attach logging or metrics hooks without coupling the storage packages to a specific observability backend.

## bboltx Example

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

db, err := bboltx.Open(
    "data.db",
    0o600,
    nil,
    bboltx.WithDBLogger(logger),
    bboltx.WithDBObserver(observer.NewSlog(logger)),
)
if err != nil {
    panic(err)
}
defer db.Close()

users := bboltx.NewBucketWithDB(
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
)

if err := users.Put(ctx, "u_1001", User{ID: "u_1001", Name: "Alice"}); err != nil {
    panic(err)
}

user, ok, err := users.Get(ctx, "u_1001")
```

Full example: [`examples/bboltx-basic/main.go`](./examples/bboltx-basic/main.go)

## badgerx Example

```go
options := badger.DefaultOptions("data").WithLogger(nil)

db, err := badgerx.Open(
    options,
    badgerx.WithDBLogger(logger),
    badgerx.WithDBObserver(observer.NewSlog(logger)),
)
if err != nil {
    panic(err)
}
defer db.Close()

users := badgerx.NewNamespaceWithDB(
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
)

if err := users.Set(ctx, "u_1001", User{ID: "u_1001", Name: "Alice"}, badgerx.WithTTL(time.Hour)); err != nil {
    panic(err)
}

user, ok, err := users.Get(ctx, "u_1001")
```

Full example: [`badgerx/examples/basic/main.go`](./badgerx/examples/basic/main.go)

## Observer Example

```go
obs := observer.NewSlog(logger, observer.WithSlogMessage("storx operation"))

db, err := bboltx.Open(
    "data.db",
    0o600,
    nil,
    bboltx.WithDBObserver(obs),
)
```

You can also implement your own observer:

```go
obs := observer.ObserverFunc(func(ctx context.Context, event observer.Event) {
    // send metrics, traces, audit events, or custom logs
})
```

## Development

Run tests across the workspace:

```bash
go test ./... ./observer/... ./codec/... ./keycodec/... ./bboltx/... ./badgerx/...
```

Run lint:

```bash
golangci-lint run
```
