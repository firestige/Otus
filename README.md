# Otus
Optimized Traffic Unveiling Suite

## intsall

### install dependency

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### generate protobuf code

```bash
protoc --go_out=. --go-grpc_out=. api/v1/daemon.proto
```

## Structure

```
otus/
├── .devcontainer/
│
├── api/
│
├── cmd/
│
├── configs/
│
├── doc/
│
├── internal/                        # 私有实现（不可被外部导入）
│   ├── pipeline/
│   │   ├── pipeline.go              # 核心引擎实现
│   │   ├── executor.go
│   │   └── stage.go
│   ├── plugin/
│   │   ├── loader.go                # 插件加载器
│   │   ├── manager.go               # 插件管理器
│   │   └── registry.go              # 注册表实现
│   ├── config/
│   │   └── loader.go                # 配置加载
│   └── app/
│       ├── app.go                   # 应用主逻辑
│       └── lifecycle.go             # 生命周期管理
│
├── pkg/                             # 公开 API（供插件开发者使用）
│   ├── gather/
│   │   └── interface.go             # ✅ 公开接口
│   ├── processor/
│   │   └── interface.go             # ✅ 公开接口
│   ├── forwarder/
│   │   └── interface.go             # ✅ 公开接口
│   ├── models/
│   │   ├── packet.go                # ✅ 公开数据模型
│   │   └── metadata.go
│   └── plugin/
│       └── registry.go              # ✅ 公开注册函数
│
│
├── plugins/                          # 插件实现
│   ├── gatherers/
│   ├── processors/
│   └── forwarders/
│
├── scripts/
│
├── .gitignore
├── go.mod
├── go.sum
├── LICENSE
├── main.go
├── Makefile
└── README.md
```