package confort

import (
	"context"

	"github.com/daichitakahashi/confort/wait"
	"github.com/docker/go-connections/nat"
)

// projectDir
// projectFile
// env
// profiles -- これによって、Projectをフィルタリングする。一貫して同じ値を使った方が良い。作成されたコンテナはprofileに関するラベル等のメタデータを保持していない

// 最初に、上記の設定を元にサービスの情報を取得した方が良い
// 取得したい情報
//   - サービスの情報
//   - サービスのポート情報
//
// 取得方法
//   - `docker compose convert --format json --profile PROFILE`
//   - あるいはパースした types.Project
type ProjectConfig struct {
	Name        string
	ConfigFiles []string
	Services    []struct {
		// services that has been enabled by profiles
		Name         string
		ExposedPorts nat.PortMap
	}
}

type ComposeBackend interface {
	Load(ctx context.Context, workingDir string, configFiles []string, envFile *string, profiles []string) (Composer, error)
}

type LoadOptions struct {
	// WorkingDir  string
	ConfigFiles []string
	EnvFile     *string
	Profiles    []string
}

type Composer interface {
	// Up
	// *
	Up(ctx context.Context, service string, opts UpOptions) (Ports, error)
	Down(ctx context.Context, services []string) error
}

func WithComposeBackend(b ComposeBackend) {}

// TODO
//  * scaleオプションがあるのが`docker compose up`だけであるため、`create & start`では代替にならない。

// TODO: コンテナを自分が作成したのかどうかを確認したい
//  * 排他制御しているから、実行前に存在しないコンテナが作成されていれば、それは自分が起動させたものだと判定して良い

// UpOptions
// --always-recreate-deps		Recreate dependent containers. Incompatible with --no-recreate.
// --build		Build images before starting containers.
// --detach , -d		Detached mode: Run containers in the background
// --force-recreate		Recreate containers even if their configuration and image haven't changed.
// --no-deps		Don't start linked services. TODO: 依存するコンテナを起動しない
// --no-recreate		If containers already exist, don't recreate them. Incompatible with --force-recreate.
// --pull	missing	Pull image before running ("always"|"missing"|"never")
// --remove-orphans		Remove containers for services not defined in the Compose file.
// --renew-anon-volumes , -V		Recreate anonymous volumes instead of retrieving data from the previous containers.
// --scale		Scale SERVICE to NUM instances. Overrides the scale setting in the Compose file if present.
// --timeout , -t	10	Use this timeout in seconds for container shutdown when attached or when containers are already running.
// --timestamps		Show timestamps.
// --wait		Wait for services to be running|healthy. Implies detached mode.
type UpOptions struct {
	Scale  int
	Waiter *wait.Waiter // TODO: スケールに対応した Waiter が必要
}

// コンテナごとに取得したい情報
// // インデックス - com.docker.compose.container-number
// コンテナID
// ポート
