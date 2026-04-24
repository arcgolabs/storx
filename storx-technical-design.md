# storx 技术设计文档

## 1. 项目定位

`storx` 是面向 Go 嵌入式存储引擎的现代化封装库，目标是为 bbolt、Badger 等本地持久化 KV 引擎提供更符合现代 Go 使用习惯的 API。

项目重点不在于屏蔽所有底层差异，也不追求构建一个统一的通用 KV 抽象，而是围绕各个存储引擎自身的数据模型，提供：

- 泛型化的读写 API；
- 可复用的 value codec 机制；
- 可复用的 key codec 机制；
- 更清晰的事务、迭代、前缀扫描封装；
- 更统一的错误处理；
- 更易接入业务系统的 namespace / bucket / repository 风格接口；
- 后续可扩展到 Pebble、LevelDB、SQLite KV layer 等嵌入式存储引擎。

仓库名称：

```text
github.com/arcgolabs/storx
```

项目定位：

```text
Modern typed APIs for embedded Go storage engines.
```

中文描述：

```text
面向 Go 嵌入式存储引擎的现代化封装，提供 bbolt、Badger 的泛型 API，并复用统一的 codec 与 key codec 机制。
```

---

## 2. 设计目标

### 2.1 核心目标

`storx` 的核心目标是降低 bbolt、Badger 等嵌入式存储引擎在业务代码中的使用成本。

主要目标包括：

1. 提供泛型化 API，减少业务层重复的序列化、反序列化代码。
2. 提供统一的 codec 与 key codec 机制，让 value 编码和 key 编码可插拔。
3. 保留 bbolt 与 Badger 各自的数据模型，不强行做统一抽象。
4. 封装事务、游标、迭代器、前缀扫描等高频能力。
5. 提供更自然的错误处理方式，例如 `not found`、`closed`、`codec error` 等。
6. 提供足够薄的封装，避免把底层引擎能力隐藏得过深。
7. 支持后续扩展更多嵌入式存储引擎。

### 2.2 非目标

以下内容不是 `storx` 初期目标：

1. 不做完整 ORM。
2. 不做 SQL 查询引擎。
3. 不做分布式存储。
4. 不做统一的强 KV 抽象。
5. 不屏蔽所有底层引擎差异。
6. 不在底层强制提供跨引擎 repository 接口。
7. 不实现复杂索引系统，除非后续上层模块需要。

---

## 3. 总体设计原则

### 3.1 独立封装，不强行统一

bbolt 和 Badger 虽然都是嵌入式 KV，但模型不同：

```text
bbolt:
DB -> Tx -> Bucket -> Cursor -> Key/Value

Badger:
DB -> Txn -> Key/Value -> Prefix Iterator -> Entry Options
```

因此 `storx` 不应该把两者压平成一个统一接口。

正确的设计方式是：

```text
bboltx:
围绕 bucket、tx、cursor 设计 API。

badgerx:
围绕 namespace、txn、iterator、TTL、entry options 设计 API。
```

共享部分只放在 codec、keycodec、错误定义、辅助类型等通用模块中。

### 3.2 尊重底层引擎能力

bbolt 有 bucket，Badger 没有 bucket。

Badger 有 TTL、value log GC、entry options，bbolt 没有这些能力。

因此：

- bbolt 不应该伪造 TTL；
- Badger 不应该强行模拟 bbolt bucket；
- 通用层不应该抹平底层差异；
- 各引擎特有能力应该在各自包中自然暴露。

### 3.3 业务友好，而不是底层替代品

`storx` 不是为了完全替代 bbolt / Badger 原生 API，而是给常见业务场景提供更舒服的 API。

如果使用方需要非常底层的能力，仍然可以通过封装对象拿到底层实例。

例如：

```go
raw := store.Raw()
```

或者：

```go
db := bucket.DB()
```

是否暴露底层对象可以在实现阶段谨慎设计，但原则上不应把底层能力完全封死。

---

## 4. 仓库结构设计

建议仓库结构如下：

```text
storx/
  README.md
  go.mod

  codec/
    codec.go
    json.go
    bytes.go
    string.go
    gob.go
    binary.go

  keycodec/
    keycodec.go
    string.go
    bytes.go
    uint64.go
    int64.go
    time.go
    composite.go

  bboltx/
    db.go
    bucket.go
    tx.go
    cursor.go
    options.go
    sequence.go
    errors.go

  badgerx/
    db.go
    namespace.go
    tx.go
    iterator.go
    entry.go
    options.go
    gc.go
    errors.go

  internal/
    bytesx/
      copy.go

  error.go
```

公共错误建议放在根包：

```go
import "github.com/arcgolabs/storx"

if errors.Is(err, storx.ErrNotFound) {
    // handle not found
}
```

---

## 5. 公共模块设计

## 5.1 Value Codec

value codec 用于对象与字节数组之间的转换。

接口设计：

```go
package codec

type Codec[T any] interface {
    Marshal(value T) ([]byte, error)
    Unmarshal(data []byte) (T, error)
}
```

常见实现：

```go
codec.JSON[T]()
codec.Gob[T]()
codec.Bytes()
codec.String()
codec.Binary[T]()
```

示例：

```go
type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

userCodec := codec.JSON[User]()
```

使用：

```go
data, err := userCodec.Marshal(user)
value, err := userCodec.Unmarshal(data)
```

### 5.1.1 Bytes Codec

`[]byte` 需要特殊处理。

```go
type BytesCodec struct{}

func (BytesCodec) Marshal(value []byte) ([]byte, error) {
    return value, nil
}

func (BytesCodec) Unmarshal(data []byte) ([]byte, error) {
    return append([]byte(nil), data...), nil
}
```

默认情况下，反序列化应复制数据，避免业务代码长期持有底层引擎返回的临时内存。

如果后续确实需要极致性能，可以再提供 unsafe 模式：

```go
codec.BytesNoCopy()
```

但不建议初版暴露过多 unsafe 能力。

---

## 5.2 Key Codec

key codec 和 value codec 应该分开。

原因是 key 不只是编码问题，还涉及：

- 排序；
- prefix scan；
- 字节序；
- 范围查询；
- 复合 key；
- 数值 key 的有序编码；
- 时间 key 的有序编码。

接口设计：

```go
package keycodec

type Codec[K any] interface {
    EncodeKey(key K) ([]byte, error)
    DecodeKey(data []byte) (K, error)
}
```

常见实现：

```go
keycodec.String()
keycodec.Bytes()
keycodec.Uint64BE()
keycodec.Int64BE()
keycodec.TimeUnixNanoBE()
```

示例：

```go
users := bboltx.NewBucket[string, User](
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
)
```

### 5.2.1 为什么 key codec 重要

如果 key 是 `uint64`，并且希望按照数值顺序扫描，不能直接用字符串编码。

错误示例：

```text
"1"
"10"
"100"
"2"
```

正确方式是使用大端序编码：

```go
keycodec.Uint64BE()
```

这样底层字节排序和数值排序一致。

---

## 5.3 复合 Key 设计

后续可以提供复合 key 支持。

例如业务 key：

```text
tenant_id + user_id
```

可以编码为：

```text
tenant/{tenantID}/user/{userID}
```

或者二进制编码：

```text
[len(tenantID)] [tenantID] [len(userID)] [userID]
```

初版可以先提供简单字符串拼接工具：

```go
keycodec.Join("/", keycodec.String(), keycodec.String())
```

示例：

```go
type UserKey struct {
    TenantID string
    UserID   string
}
```

后续可以做：

```go
keycodec.Composite[UserKey](
    func(k UserKey) []any {
        return []any{k.TenantID, k.UserID}
    },
)
```

不过这类泛型复合 key 设计容易复杂，初版可以暂缓。

---

## 6. bboltx 设计

## 6.1 设计定位

`bboltx` 是对 bbolt 的现代化封装。

核心围绕：

```text
DB -> Bucket -> Tx -> Cursor
```

设计。

主要能力：

- typed bucket；
- typed transaction；
- typed cursor；
- value codec；
- key codec；
- prefix scan；
- sequence；
- bucket 自动创建；
- 只读事务与写事务分离。

---

## 6.2 Bucket 定义

```go
package bboltx

type Bucket[K any, V any] struct {
    db     *bbolt.DB
    name   []byte
    keys   keycodec.Codec[K]
    values codec.Codec[V]
    opts   bucketOptions
}
```

创建方式：

```go
users := bboltx.NewBucket[string, User](
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
)
```

建议使用 `NewBucket`，语义更清晰。

---

## 6.3 基础 API

可以提供便捷方法：

```go
func (b *Bucket[K, V]) Get(ctx context.Context, key K) (V, bool, error)

func (b *Bucket[K, V]) Put(ctx context.Context, key K, value V) error

func (b *Bucket[K, V]) Delete(ctx context.Context, key K) error

func (b *Bucket[K, V]) Exists(ctx context.Context, key K) (bool, error)
```

核心仍然应该是事务 API：

```go
func (b *Bucket[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error

func (b *Bucket[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error
```

原因是 bbolt 天然以事务为边界。只提供单次 Get/Put 会让用户忽略事务模型，也容易导致多次操作时重复开启事务。

---

## 6.4 事务接口

```go
type ViewTx[K any, V any] interface {
    Get(key K) (V, bool, error)
    Exists(key K) (bool, error)
    ForEach(fn func(key K, value V) error) error
    ScanPrefix(prefix []byte, fn func(key K, value V) error) error
    Cursor() Cursor[K, V]
}

type UpdateTx[K any, V any] interface {
    ViewTx[K, V]

    Put(key K, value V) error
    Delete(key K) error
    NextSequence() (uint64, error)
}
```

`ScanPrefix` 的参数建议使用 `[]byte`，因为 prefix 不一定是一个完整 key。

---

## 6.5 Cursor 设计

```go
type Cursor[K any, V any] interface {
    First() (K, V, bool, error)
    Last() (K, V, bool, error)
    Seek(prefix []byte) (K, V, bool, error)
    Next() (K, V, bool, error)
    Prev() (K, V, bool, error)
}
```

Cursor 的生命周期依赖事务，因此不能逃逸出事务函数。

示例：

```go
err := users.View(ctx, func(tx bboltx.ViewTx[string, User]) error {
    return tx.ForEach(func(key string, user User) error {
        fmt.Println(key, user.Name)
        return nil
    })
})
```

---

## 6.6 Bucket 自动创建

bbolt 需要 bucket 存在。

写事务中可以自动创建 bucket：

```go
bboltx.WithCreateBucketIfMissing(true)
```

默认建议：

```text
Update 自动创建 bucket
View 不自动创建 bucket
```

如果 `View` 时 bucket 不存在：

- `Get` 返回 `ok=false, err=nil`；
- `ForEach` 直接返回 `nil`；
- `Put` 自动创建 bucket；
- 如果显式关闭自动创建，返回 `ErrBucketNotFound`。

这样对业务更友好。

---

## 6.7 bboltx 使用示例

```go
db, err := bbolt.Open("data.db", 0600, nil)
if err != nil {
    panic(err)
}
defer db.Close()

users := bboltx.NewBucket[string, User](
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
)

err = users.Update(context.Background(), func(tx bboltx.UpdateTx[string, User]) error {
    return tx.Put("u_1001", User{
        ID:   "u_1001",
        Name: "Alice",
    })
})

user, ok, err := users.Get(context.Background(), "u_1001")
```

---

## 7. badgerx 设计

## 7.1 设计定位

`badgerx` 是对 Badger 的现代化封装。

核心围绕：

```text
DB -> Namespace -> Txn -> Iterator
```

设计。

Badger 没有 bucket，因此不应该在 API 中引入 bucket 概念。

更自然的封装是 namespace：

```go
users := badgerx.NewNamespace[string, User](
    db,
    "users/",
    keycodec.String(),
    codec.JSON[User](),
)
```

namespace 本质是 key prefix。

---

## 7.2 Namespace 定义

```go
package badgerx

type Namespace[K any, V any] struct {
    db     *badger.DB
    prefix []byte
    keys   keycodec.Codec[K]
    values codec.Codec[V]
    opts   namespaceOptions
}
```

创建：

```go
users := badgerx.NewNamespace[string, User](
    db,
    "users/",
    keycodec.String(),
    codec.JSON[User](),
)
```

实际底层 key：

```text
users/u_1001
users/u_1002
```

业务层只看到：

```text
u_1001
u_1002
```

---

## 7.3 基础 API

```go
func (n *Namespace[K, V]) Get(ctx context.Context, key K) (V, bool, error)

func (n *Namespace[K, V]) Set(ctx context.Context, key K, value V, opts ...SetOption) error

func (n *Namespace[K, V]) Delete(ctx context.Context, key K) error

func (n *Namespace[K, V]) Exists(ctx context.Context, key K) (bool, error)
```

Badger 更习惯叫 `Set`，bbolt 更习惯叫 `Put`。

这里可以保留差异：

```text
bboltx.Put
badgerx.Set
```

不必为了统一而强行改名。

---

## 7.4 事务 API

```go
func (n *Namespace[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error

func (n *Namespace[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error
```

事务接口：

```go
type ViewTx[K any, V any] interface {
    Get(key K) (V, bool, error)
    Exists(key K) (bool, error)
    Scan(fn func(key K, value V) error) error
    ScanPrefix(prefix []byte, fn func(key K, value V) error) error
}

type UpdateTx[K any, V any] interface {
    ViewTx[K, V]

    Set(key K, value V, opts ...SetOption) error
    Delete(key K) error
}
```

---

## 7.5 SetOption 设计

Badger 支持 TTL 等 entry option。

封装：

```go
type SetOption interface {
    apply(*setOptions)
}

func WithTTL(ttl time.Duration) SetOption
func WithMeta(meta byte) SetOption
func WithDiscard() SetOption
```

使用：

```go
err := users.Set(ctx, "u_1001", user, badgerx.WithTTL(time.Hour))
```

事务中：

```go
err := users.Update(ctx, func(tx badgerx.UpdateTx[string, User]) error {
    return tx.Set("u_1001", user, badgerx.WithTTL(time.Hour))
})
```

---

## 7.6 Iterator 设计

Badger 的 iterator 需要处理 item value 生命周期。

封装时应默认复制 value，避免回调外持有无效引用。

```go
func (tx *viewTx[K, V]) ScanPrefix(prefix []byte, fn func(K, V) error) error {
    // 内部处理 iterator、prefix、decode、value copy
}
```

使用：

```go
err := users.View(ctx, func(tx badgerx.ViewTx[string, User]) error {
    return tx.ScanPrefix([]byte("active/"), func(key string, user User) error {
        fmt.Println(key, user.Name)
        return nil
    })
})
```

---

## 7.7 Badger GC 封装

Badger 有 value log GC。

可以在 namespace 或 db wrapper 上提供：

```go
func RunValueLogGC(db *badger.DB, discardRatio float64) error
```

或者：

```go
type DB struct {
    raw *badger.DB
}

func (db *DB) RunValueLogGC(discardRatio float64) error
```

初版如果只是 namespace wrapper，可以先提供工具函数：

```go
badgerx.RunValueLogGC(db, 0.5)
```

后续再考虑封装 DB 类型。

---

## 7.8 badgerx 使用示例

```go
db, err := badger.Open(badger.DefaultOptions("./badger-data"))
if err != nil {
    panic(err)
}
defer db.Close()

users := badgerx.NewNamespace[string, User](
    db,
    "users/",
    keycodec.String(),
    codec.JSON[User](),
)

err = users.Update(context.Background(), func(tx badgerx.UpdateTx[string, User]) error {
    return tx.Set("u_1001", User{
        ID:   "u_1001",
        Name: "Alice",
    }, badgerx.WithTTL(time.Hour))
})

user, ok, err := users.Get(context.Background(), "u_1001")
```

---

## 8. 错误设计

公共错误：

```go
package storx

var (
    ErrNotFound       = errors.New("storx: not found")
    ErrClosed         = errors.New("storx: closed")
    ErrInvalidKey     = errors.New("storx: invalid key")
    ErrInvalidValue   = errors.New("storx: invalid value")
    ErrCodec          = errors.New("storx: codec error")
    ErrKeyCodec       = errors.New("storx: key codec error")
    ErrBucketNotFound = errors.New("storx: bucket not found")
)
```

封装错误时保留原始错误：

```go
return fmt.Errorf("%w: marshal value: %v", storx.ErrCodec, err)
```

使用方可以：

```go
if errors.Is(err, storx.ErrCodec) {
    // handle codec error
}
```

对于 `Get`：

```go
value, ok, err := users.Get(ctx, key)
```

如果不存在：

```text
ok == false
err == nil
```

不建议 `Get` 不存在时返回 `ErrNotFound`，因为 Go 里 `value, ok, err` 更适合 KV 场景。

---

## 9. Context 设计

bbolt 和 Badger 的原生 API 并不是完整 context-aware。

但 `storx` 的 API 仍然建议带 `context.Context`：

```go
Get(ctx context.Context, key K) (V, bool, error)
```

原因：

1. 保持业务层接口统一。
2. 后续可以接入 metrics、trace、logger。
3. 可以在进入事务前检查 context。
4. 长扫描过程中可以定期检查 context。
5. 未来封装其他支持 context 的引擎时更自然。

实现中至少应在以下位置检查：

```go
select {
case <-ctx.Done():
    return ctx.Err()
default:
}
```

对于 scan/iterator，每轮循环可以检查一次。

---

## 10. 编码与内存安全

### 10.1 默认复制数据

bbolt 和 Badger 返回的数据在事务生命周期内有效。

业务代码如果把 `[]byte` 持有到事务外，可能存在风险。

因此默认策略：

```text
decode value 前复制数据，或者保证 Unmarshal 不持有原始切片。
```

对于 JSON/Gob 这类 codec 问题不大。

但对于 `[]byte` codec，必须复制。

```go
func (BytesCodec) Unmarshal(data []byte) ([]byte, error) {
    return append([]byte(nil), data...), nil
}
```

### 10.2 零拷贝选项

后续可以提供：

```go
codec.UnsafeBytes()
```

但初版不建议重点设计，避免误用。

---

## 11. Options 设计

### 11.1 bboltx Options

```go
type BucketOption func(*bucketOptions)

func WithCreateBucketIfMissing(enabled bool) BucketOption
func WithReadOnlyMissingAsEmpty(enabled bool) BucketOption
```

示例：

```go
users := bboltx.NewBucket[string, User](
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
    bboltx.WithCreateBucketIfMissing(true),
)
```

### 11.2 badgerx Options

```go
type NamespaceOption func(*namespaceOptions)

func WithPrefixSeparator(separator string) NamespaceOption
func WithCopyValue(enabled bool) NamespaceOption
```

Set option：

```go
type SetOption func(*setOptions)

func WithTTL(ttl time.Duration) SetOption
func WithMeta(meta byte) SetOption
```

---

## 12. 推荐 API 草案

### 12.1 codec

```go
package codec

type Codec[T any] interface {
    Marshal(value T) ([]byte, error)
    Unmarshal(data []byte) (T, error)
}

func JSON[T any]() Codec[T]
func Gob[T any]() Codec[T]
func String() Codec[string]
func Bytes() Codec[[]byte]
```

### 12.2 keycodec

```go
package keycodec

type Codec[K any] interface {
    EncodeKey(key K) ([]byte, error)
    DecodeKey(data []byte) (K, error)
}

func String() Codec[string]
func Bytes() Codec[[]byte]
func Uint64BE() Codec[uint64]
func Int64BE() Codec[int64]
func TimeUnixNanoBE() Codec[time.Time]
```

### 12.3 bboltx

```go
package bboltx

type Bucket[K any, V any] struct {
    // unexported
}

func NewBucket[K any, V any](
    db *bbolt.DB,
    name string,
    keys keycodec.Codec[K],
    values codec.Codec[V],
    opts ...BucketOption,
) *Bucket[K, V]

func (b *Bucket[K, V]) Get(ctx context.Context, key K) (V, bool, error)
func (b *Bucket[K, V]) Put(ctx context.Context, key K, value V) error
func (b *Bucket[K, V]) Delete(ctx context.Context, key K) error
func (b *Bucket[K, V]) Exists(ctx context.Context, key K) (bool, error)

func (b *Bucket[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error
func (b *Bucket[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error
```

### 12.4 badgerx

```go
package badgerx

type Namespace[K any, V any] struct {
    // unexported
}

func NewNamespace[K any, V any](
    db *badger.DB,
    prefix string,
    keys keycodec.Codec[K],
    values codec.Codec[V],
    opts ...NamespaceOption,
) *Namespace[K, V]

func (n *Namespace[K, V]) Get(ctx context.Context, key K) (V, bool, error)
func (n *Namespace[K, V]) Set(ctx context.Context, key K, value V, opts ...SetOption) error
func (n *Namespace[K, V]) Delete(ctx context.Context, key K) error
func (n *Namespace[K, V]) Exists(ctx context.Context, key K) (bool, error)

func (n *Namespace[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error
func (n *Namespace[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error
```

---

## 13. 模块边界

### 13.1 codec

只负责 value 编码，不知道任何存储引擎。

### 13.2 keycodec

只负责 key 编码，不知道任何存储引擎。

### 13.3 bboltx

只依赖：

```text
codec
keycodec
storx common errors
bbolt
```

### 13.4 badgerx

只依赖：

```text
codec
keycodec
storx common errors
badger
```

### 13.5 不做跨引擎接口

初版不要定义：

```go
type Store[K, V any] interface {
    Get(...)
    Put(...)
}
```

原因是它会诱导使用方忽略 bbolt 和 Badger 的差异。

如果未来确实需要统一 repository，可以在更上层做，例如：

```text
repositoryx
statex
```

而不是在 `storx` 底层强行统一。

---

## 14. 典型使用场景

### 14.1 本地 metadata

适合使用 bbolt：

```text
agent metadata
local registry
configuration snapshot
small index
read-heavy state
```

使用：

```go
configs := bboltx.NewBucket[string, Config](
    db,
    "configs",
    keycodec.String(),
    codec.JSON[Config](),
)
```

### 14.2 本地任务状态

bbolt 和 Badger 都可以。

如果写入频率不高，bbolt 更简单。

如果任务量较大、状态更新频繁，可以使用 Badger。

```go
tasks := badgerx.NewNamespace[string, TaskState](
    db,
    "tasks/",
    keycodec.String(),
    codec.JSON[TaskState](),
)
```

### 14.3 TTL 缓存

适合 Badger。

```go
cache := badgerx.NewNamespace[string, CacheValue](
    db,
    "cache/",
    keycodec.String(),
    codec.JSON[CacheValue](),
)

_ = cache.Set(ctx, "token:xxx", value, badgerx.WithTTL(30*time.Minute))
```

### 14.4 有序 ID 扫描

使用 `Uint64BE`：

```go
events := bboltx.NewBucket[uint64, Event](
    db,
    "events",
    keycodec.Uint64BE(),
    codec.JSON[Event](),
)
```

这样可以按 ID 顺序扫描。

---

## 15. 测试设计

### 15.1 codec 测试

需要覆盖：

- JSON 正常编码/解码；
- Bytes codec 是否复制；
- String codec；
- Gob codec；
- 错误输入；
- nil / zero value。

### 15.2 keycodec 测试

需要覆盖：

- 字符串 key；
- bytes key；
- uint64 大端序排序；
- int64 排序；
- time 排序；
- Encode 后 Decode 是否一致。

### 15.3 bboltx 测试

需要覆盖：

- Put/Get/Delete；
- bucket 不存在；
- 自动创建 bucket；
- View 事务；
- Update 事务；
- ForEach；
- ScanPrefix；
- Cursor；
- Sequence；
- codec error；
- keycodec error；
- context canceled。

### 15.4 badgerx 测试

需要覆盖：

- Set/Get/Delete；
- namespace prefix；
- Scan；
- ScanPrefix；
- TTL；
- iterator；
- codec error；
- keycodec error；
- context canceled；
- value copy；
- RunValueLogGC 工具方法。

---

## 16. Benchmark 设计

可以设计以下 benchmark：

```text
BenchmarkBboltPutJSON
BenchmarkBboltGetJSON
BenchmarkBboltScanPrefixJSON

BenchmarkBadgerSetJSON
BenchmarkBadgerGetJSON
BenchmarkBadgerScanPrefixJSON

BenchmarkCodecJSON
BenchmarkCodecGob
BenchmarkKeyCodecUint64BE
```

benchmark 不是为了证明谁比谁快，而是用于观察封装层额外开销。

重点关注：

- codec 开销；
- keycodec 开销；
- scan 中 decode 开销；
- value copy 开销；
- generic wrapper 是否带来明显额外成本。

---

## 17. 后续扩展方向

### 17.1 Pebble 支持

未来可以加入：

```text
pebblex/
```

Pebble 更适合高写入和 LSM 场景。

### 17.2 LevelDB 支持

可以加入：

```text
leveldbx/
```

但优先级可以低于 Pebble。

### 17.3 SQLite KV Layer

如果后续需要嵌入式 SQL + KV，可以考虑：

```text
sqlitex/
```

但这会拉大项目边界，需要谨慎。

### 17.4 Repository 层

如果多个业务模块确实需要统一使用方式，可以另开上层包：

```text
repository/
```

例如：

```go
type Repository[K any, V any] interface {
    Get(ctx context.Context, key K) (V, bool, error)
    Save(ctx context.Context, key K, value V) error
    Delete(ctx context.Context, key K) error
}
```

但这个不建议初版就做。

---

## 18. 推荐落地顺序

### 第一阶段：公共能力

```text
codec
keycodec
common errors
```

先实现：

```text
codec.JSON
codec.String
codec.Bytes

keycodec.String
keycodec.Bytes
keycodec.Uint64BE
keycodec.Int64BE
```

### 第二阶段：bboltx

先实现：

```text
NewBucket
Get
Put
Delete
Exists
View
Update
ForEach
ScanPrefix
```

Sequence 和 Cursor 可以后置。

### 第三阶段：badgerx

先实现：

```text
NewNamespace
Get
Set
Delete
Exists
View
Update
Scan
ScanPrefix
WithTTL
```

GC 工具可以后置。

### 第四阶段：补测试与 benchmark

重点补：

```text
codec roundtrip
key ordering
basic CRUD
prefix scan
value copy
context canceled
```

### 第五阶段：完善 README

README 应该突出：

```text
storx is not a unified KV abstraction.
storx provides typed wrappers for embedded storage engines.
```

避免别人误解成数据库抽象层。

---

## 19. README 示例

```md
# storx

Modern typed APIs for embedded Go storage engines.

`storx` provides generic, codec-based wrappers for embedded storage engines such as bbolt and Badger.

It does not try to hide all differences between engines. Instead, each engine gets an API that matches its own data model:

- `bboltx`: typed bucket and transaction APIs for bbolt.
- `badgerx`: typed namespace and transaction APIs for Badger.
- `codec`: reusable value codecs.
- `keycodec`: reusable key codecs with ordering support.

## Example: bbolt

```go
users := bboltx.NewBucket[string, User](
    db,
    "users",
    keycodec.String(),
    codec.JSON[User](),
)

err := users.Put(ctx, "u_1001", User{
    ID: "u_1001",
    Name: "Alice",
})
```

## Example: Badger

```go
users := badgerx.NewNamespace[string, User](
    db,
    "users/",
    keycodec.String(),
    codec.JSON[User](),
)

err := users.Set(ctx, "u_1001", User{
    ID: "u_1001",
    Name: "Alice",
}, badgerx.WithTTL(time.Hour))
```
```

---

## 20. 最终建议

`storx` 的边界应该保持清晰：

```text
storx = embedded storage typed wrappers
```

不要把它扩展成：

```text
ORM
distributed database
general repository framework
unified KV abstraction
```

初版最有价值的部分是：

```text
codec + keycodec + bboltx + badgerx
```

其中：

- `bboltx` 保留 bucket/tx/cursor 模型；
- `badgerx` 保留 namespace/txn/iterator/TTL 模型；
- `codec` 解决 value 泛型编解码；
- `keycodec` 解决 key 编码、排序、prefix scan 的基础问题。

这个方向比较适合 `arcgolabs` 的基础库体系：足够通用，但不膨胀；能复用到 agent、本地 registry、scheduler state、配置快照、轻量 metadata 等多个项目里。
