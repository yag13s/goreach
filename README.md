# goreach

Go カバレッジ（到達性）計測・集約

稼働中のシステムで実際に到達していないコードパスを特定する
Go 1.20+ のネイティブカバレッジ機能（`go build -cover`, `GOCOVERDIR`）の上に薄いレイヤーを載せるだけ
出力は JSON 形式

## インストール

```bash
go install github.com/yag13s/goreach/cmd/goreach@latest
```

## アーキテクチャ

```
┌──────────────────────────────────────────────────┐
│  Layer 1: flush ライブラリ（SDK）                   │
│  計測対象アプリに組み込み、カバレッジを定期書き出し    │
└────────────────────┬─────────────────────────────┘
                     │ covmeta + covcounters
                     ▼
┌──────────────────────────────────────────────────┐
│  Layer 2: ストレージ（S3 / ローカル等）              │
│  ビルドバージョン単位でディレクトリ分離               │
└────────────────────┬─────────────────────────────┘
                     │ Download + Merge
                     ▼
┌──────────────────────────────────────────────────┐
│  Layer 3: goreach CLI（分析）                      │
│  マージ → AST 解析 → 未到達コード特定 → JSON 出力    │
└──────────────────────────────────────────────────┘
```

## クイックスタート（コード変更ゼロ）

```bash
# 1. カバレッジ付きビルド
go build -cover -covermode=set -o myserver ./cmd/myserver

# 2. GOCOVERDIR を指定して実行
mkdir -p /tmp/coverage
GOCOVERDIR=/tmp/coverage ./myserver

# 3. プロセス停止（カバレッジ自動書き出し）
kill -TERM <pid>

# 4. 分析
goreach analyze -coverdir /tmp/coverage -pretty
```

## CLI リファレンス

### `goreach analyze`

GOCOVERDIR またはテキストプロファイルから未到達コードを分析し、JSON レポートを出力する

```bash
# GOCOVERDIR から分析（再帰探索）
goreach analyze -coverdir /var/coverage -r -pretty

# テキストプロファイルから分析
goreach analyze -profile coverage.txt -pretty

# 特定パッケージのみ、完全未到達関数だけ表示
goreach analyze -coverdir /var/coverage -r -pkg "myapp/internal" -threshold 0

# ファイルに出力
goreach analyze -coverdir /var/coverage -r -o report.json
```

| フラグ | 説明 | デフォルト |
|--------|------|-----------|
| `-profile <file>` | テキスト形式カバレッジプロファイルのパス | — |
| `-coverdir <dir>` | GOCOVERDIR パス（`-profile` と排他） | — |
| `-r` | `-coverdir` 配下を再帰探索 | `false` |
| `-pkg <prefixes>` | パッケージフィルタ（カンマ区切り） | 全パッケージ |
| `-threshold <float>` | カバレッジがこの%以下の関数のみ表示 | `100`（全関数） |
| `-min-statements <n>` | 未到達ステートメントが N 以上の関数のみ | `0` |
| `-o <file>` | 出力ファイル | stdout |
| `-pretty` | JSON を整形出力 | `false` |

### `goreach summary`

カバレッジサマリをテキストで表示する

```bash
goreach summary -coverdir /var/coverage -r
goreach summary -profile coverage.txt
```

### JSON 出力例

```json
{
  "version": 1,
  "generated_at": "2026-02-27T10:30:00Z",
  "mode": "set",
  "total": {
    "total_statements": 1250,
    "covered_statements": 890,
    "coverage_percent": 71.2
  },
  "packages": [
    {
      "import_path": "myapp/internal/auth",
      "total": { "total_statements": 200, "covered_statements": 145, "coverage_percent": 72.5 },
      "files": [
        {
          "file_name": "myapp/internal/auth/oauth.go",
          "total": { "total_statements": 120, "covered_statements": 85, "coverage_percent": 70.8 },
          "functions": [
            {
              "name": "(*OAuthHandler).RefreshToken",
              "line": 112,
              "total_statements": 25,
              "covered_statements": 0,
              "coverage_percent": 0.0,
              "unreached_blocks": [
                { "start_line": 113, "start_col": 2, "end_line": 135, "end_col": 3, "num_statements": 25 }
              ]
            }
          ]
        }
      ]
    }
  ]
}
```

## flush ライブラリ（長時間実行プロセス向け SDK）

長時間稼働するサーバーやバッチ処理でカバレッジデータを定期的にフラッシュするためのライブラリ

```go
import "github.com/yag13s/goreach/flush"

// HTTP エンドポイントも使う場合のみ追加
import "github.com/yag13s/goreach/flush/flushhttp"
```

### 基本的な使い方

```go
flush.Enable(flush.Config{
    Storage:      flush.LocalStorage{Dir: "/var/coverage/myserver"},
    ServiceName:  "myserver",
    BuildVersion: version, // ldflags で埋め込んだコミットハッシュ等
    Interval:     5 * time.Minute,
    Clear:        true,
})
defer flush.Stop()
```

### Storage インターフェース（DI）

保存先はインターフェースで抽象化
ビルトイン実装のほか、独自実装を注入可能

```go
// Storage はカバレッジデータの保存先を抽象化する
type Storage interface {
    Store(ctx context.Context, files []string, meta Metadata) error
}
```

**ビルトイン実装:**

```go
// ローカルディスク保存（GOCOVERDIR 互換）
flush.LocalStorage{Dir: "/var/coverage/myserver"}

// 標準出力に書き出し（デバッグ用）
flush.WriterStorage{W: os.Stdout}
```

**S3 保存の例（利用者が実装）:**

```go
type S3Storage struct {
    Client *s3.Client
    Bucket string
}

func (s *S3Storage) Store(ctx context.Context, files []string, meta flush.Metadata) error {
    for _, f := range files {
        key := fmt.Sprintf("goreach/%s/%s/%s/%s",
            meta.ServiceName, meta.BuildVersion, meta.PodName, filepath.Base(f))
        body, _ := os.Open(f)
        defer body.Close()
        _, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
            Bucket: &s.Bucket, Key: &key, Body: body,
        })
        if err != nil {
            return err
        }
    }
    return nil
}
```

### フラッシュのトリガー方式

| 方式 | ユースケース | コード |
|------|-------------|--------|
| 定期実行 | 常時稼働サーバー | `Config{Interval: 5 * time.Minute}` |
| HTTP エンドポイント | k8s CronJob トリガー | `mux.Handle("/internal/coverage/", flushhttp.Handler())` |
| シグナル | バッチ処理、非HTTP プロセス | `flush.HandleSignal(syscall.SIGUSR1)` |
| プロセス終了時 | 全プロセス共通 | `defer flush.Stop()` |

### HTTP エンドポイント（`flush/flushhttp` パッケージ）

HTTP 経由でのカバレッジ制御が必要な場合のみインポートする。`flush` パッケージ本体は `net/http` に依存しない。

```go
import "github.com/yag13s/goreach/flush/flushhttp"

mux.Handle("/internal/coverage/", flushhttp.Handler())
```

| メソッド | パス | 動作 |
|---------|------|------|
| `GET` | `/internal/coverage` | カバレッジデータを返す |
| `POST` | `/internal/coverage/flush` | Storage にフラッシュ |
| `POST` | `/internal/coverage/clear` | カウンタリセット |

## ワークフロー例

### A: ローカル開発（コード変更ゼロ）

```bash
go build -cover -covermode=set -o myserver ./cmd/myserver
mkdir -p /tmp/coverage
GOCOVERDIR=/tmp/coverage ./myserver
# テスト実行後にプロセス停止
kill -TERM $(pgrep myserver)
goreach analyze -coverdir /tmp/coverage -pretty
```

### B: k8s 本番環境（push 型 — S3 保存）

```go
func main() {
    flush.Enable(flush.Config{
        Storage:      &myS3Storage{Bucket: "coverage-data"},
        ServiceName:  "myserver",
        BuildVersion: version,
        Interval:     10 * time.Minute,
    })
    defer flush.Stop()
    // ... 既存のサーバーコード ...
}
```

```bash
aws s3 sync s3://coverage-data/goreach/myserver/abc123/ /tmp/coverage/
goreach analyze -coverdir /tmp/coverage -r -pretty
```

### C: k8s 本番環境（CronJob トリガー型）

```go
mux.Handle("/internal/coverage/", flushhttp.Handler())
```

```yaml
apiVersion: batch/v1
kind: CronJob
spec:
  schedule: "0 */6 * * *"
  jobTemplate:
    spec:
      containers:
      - name: coverage-trigger
        command: ["curl", "-X", "POST", "http://myserver:8080/internal/coverage/flush"]
```

### D: バッチ処理

```bash
go build -cover -o mybatch ./cmd/mybatch
GOCOVERDIR=/tmp/coverage ./mybatch --input data.csv
goreach analyze -coverdir /tmp/coverage -pretty
```

## 設計方針

- `-cover` なしでビルドされたバイナリでも flush ライブラリはパニックしない（no-op）
- flush パッケージは外部依存ゼロ（`runtime/coverage` + stdlib のみ、`net/http` 不要）
- HTTP ハンドラは `flush/flushhttp` に分離。不要なら import しなければ `net/http` の依存も入らない
- S3 等のクラウド SDK は利用者が持ち込む（Storage インターフェースで DI）
- covmeta + covcounters のバイナリファイルをそのまま保存（テキスト変換は分析時に実施）
- ビルドバージョンが変わると covmeta の互換性が壊れるため、バージョン単位でデータを分離

## 必要要件

- Go 1.20+
- `go tool covdata`（Go ツールチェインに同梱）

## License

[MIT](LICENSE)
