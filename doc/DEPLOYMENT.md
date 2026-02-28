# capture-agent 部署指南

本文档涵盖 capture-agent 在多种环境下的部署方式。

## 目录

- [通用要求](#通用要求)
- [裸金属/物理服务器部署](#裸金属物理服务器部署)
- [虚拟机部署 (VMware/KVM/ECS)](#虚拟机部署)
- [Kubernetes 部署](#kubernetes-部署)
- [故障排查](#故障排查)

---

## 通用要求

### 系统要求

- **操作系统**: Linux 发行版
  - ✅ Ubuntu 20.04+ / Debian 11+
  - ✅ RHEL/CentOS/Rocky 8+
  - ✅ SUSE Linux Enterprise 15+
  - ✅ Fedora 35+
  - ✅ Gentoo (kernel 5.4+)
- **架构**: x86_64 (amd64) 或 ARM64 (aarch64)
- **内核**: Linux 3.10+ (推荐 4.4+，支持 AF_PACKET v3)
- **权限**: root 或 `CAP_NET_RAW` + `CAP_NET_ADMIN` 能力

### 二进制文件获取

#### 方式 1: 从源码构建（推荐）

```bash
# 克隆仓库
git clone https://github.com/firestige/capture-agent.git
cd capture-agent

# 构建当前架构的静态二进制
make build-static

# 或构建所有架构
make build-all
# 输出在 dist/ 目录：
#   dist/capture-agent-linux-amd64
#   dist/capture-agent-linux-arm64
```

#### 方式 2: 使用 Docker 构建

```bash
# 构建当前架构镜像
make docker-build

# 打包分发制品（用于裸金属/VM部署）
make dist
# 输出: dist/capture-agent-{version}-linux-{arch}.tar.gz

# 构建 Kubernetes sidecar 镜像（本地 Docker 中可用）
make sidecar-build
# 输出: capture-agent-sidecar:latest
# 推送到私仓后可在 k8s 中使用
```

#### 方式 3: 下载预编译二进制（TODO）

```bash
# 从 GitHub Releases 下载
# curl -LO https://github.com/firestige/capture-agent/releases/download/v1.0.0/otus-linux-amd64
```

---

## 裸金属/物理服务器部署

### 1. 安装二进制文件

```bash
# 复制到系统路径
sudo install -m 755 otus-linux-amd64 /usr/local/bin/capture-agent

# 验证安装
capture-agent version
```

### 2. 创建必要目录

```bash
# 配置目录
sudo mkdir -p /etc/capture-agent

# 数据目录
sudo mkdir -p /var/lib/capture-agent

# 日志目录
sudo mkdir -p /var/log/capture-agent
```

### 3. 部署配置文件

```bash
# 复制默认配置
sudo cp configs/config.yml /etc/capture-agent/

# 编辑配置（调整网络接口、Kafka地址等）
sudo vim /etc/capture-agent/config.yml
```

### 4. 安装 systemd 服务

```bash
# 复制服务文件
sudo cp configs/capture-agent.service /etc/systemd/system/

# 重载 systemd
sudo systemctl daemon-reload

# 启用开机自启
sudo systemctl enable capture-agent

# 启动服务
sudo systemctl start capture-agent

# 查看状态
sudo systemctl status capture-agent
```

### 5. 验证运行

```bash
# 查看日志
sudo journalctl -u capture-agent -f

# 检查监听端口（gRPC命令接口）
sudo ss -tlnp | grep capture-agent

# 查看 Prometheus 指标
curl http://localhost:9091/metrics
```

### 6. 创建抓包任务

```bash
# 准备任务配置文件（JSON 格式）
cat > sip-capture.json <<'EOF'
{
  "id": "sip-capture-01",
  "workers": 1,
  "capture": {
    "name": "afpacket",
    "interface": "eth0",
    "bpf_filter": "udp port 5060 or udp port 5061",
    "snap_len": 65536
  },
  "parsers": [{"name": "sip", "config": {}}],
  "processors": [],
  "reporters": [{
    "name": "kafka",
    "config": {
      "brokers": ["kafka:9092"],
      "topic": "capture-agent-sip-data"
    }
  }]
}
EOF

# 通过 UDS 创建任务
capture-agent task create -f sip-capture.json

# 查看任务列表
capture-agent task list

# 查看任务状态
capture-agent task status sip-capture-01

# 停止任务
capture-agent task stop sip-capture-01
```

---

## 虚拟机部署

适用于 VMware ESXi、KVM、Hyper-V、阿里云 ECS、AWS EC2 等虚拟化环境。

### 部署步骤

与[裸金属部署](#裸金属物理服务器部署)相同，无需额外配置。

### 注意事项

1. **网卡混杂模式**: 确保虚拟机网卡支持混杂模式
   ```bash
   # 启用混杂模式（临时）
   sudo ip link set eth0 promisc on
   
   # 永久启用（在配置文件中设置）
   # /etc/capture-agent/config.yml:
   #   capture:
   #     promiscuous: true
   ```

2. **云服务器限制**:
   - **阿里云/AWS**: 默认禁止混杂模式，仅能抓取本机流量
   - **解决方案**: 使用镜像端口或流量复制功能
   ```bash
   # 阿里云示例：配置流量镜像
   # 在控制台创建流量镜像会话，将源实例流量镜像到 capture-agent 实例
   ```

3. **VLAN/虚拟网络**: 确保虚拟机在正确的 VLAN/子网

---

## Kubernetes 部署

### 快速部署

仓库内置了基于 [Kustomize](https://kustomize.io/) 的 k8s 清单，位于 `deploy/k8s/`：

```
deploy/k8s/
├── base/                    # 基础资源（Namespace, SA, ConfigMap, DaemonSet, Service）
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── configmap.yaml
│   ├── daemonset.yaml
│   └── service.yaml
└── overlays/
    ├── dev/                 # 开发环境（低资源、debug 日志）
    │   ├── kustomization.yaml
    │   └── configs/config.yml
    └── prod/                # 生产环境（Nexus 镜像仓库、Snappy 压缩）
        ├── kustomization.yaml
        └── configs/config.yml
```

#### 1. 构建并推送 sidecar 镜像

```bash
# 编译 + 打包 sidecar 镜像
make sidecar-build

# 推送到内网镜像仓库
docker tag capture-agent-sidecar:latest registry.example.com/infra/capture-agent-sidecar:1.0.0
docker push registry.example.com/infra/capture-agent-sidecar:1.0.0
```

然后在 `deploy/k8s/overlays/prod/kustomization.yaml` 中更新 `images.newName` 和 `newTag`。

#### 2. 配置 overlay

编辑对应环境的 `configs/config.yml`，修改 Kafka Broker 地址、topic 等：

```bash
vim deploy/k8s/overlays/prod/configs/config.yml
```

#### 3. 预览生成的清单

```bash
# 预览（不实际部署）
make k8s-render-dev
make k8s-render-prod
# 或直接：kubectl kustomize deploy/k8s/overlays/prod
```

#### 4. 部署

```bash
# 开发环境
make k8s-apply-dev

# 生产环境
make k8s-apply-prod

# 等效命令
kubectl apply -k deploy/k8s/overlays/prod
```

#### 5. 验证

```bash
# 标记节点（只在贴了此标签的节点上运行）
kubectl label node <node-name> capture-agent-enabled=true

# 查看 Pod 状态
kubectl get pods -n monitoring -l app.kubernetes.io/name=capture-agent -o wide

# 查看日志
kubectl logs -n monitoring -l app.kubernetes.io/name=capture-agent --tail=100 -f

# 验证 Prometheus 指标
kubectl exec -n monitoring \
  $(kubectl get pod -n monitoring -l app.kubernetes.io/name=capture-agent -o name | head -1) \
  -- curl -s localhost:9091/metrics | head -20
```

#### 6. 卸载

```bash
make k8s-delete-prod
# 或：kubectl delete -k deploy/k8s/overlays/prod
```

---

### 部署架构

#### 方案 A: DaemonSet（推荐）

每个节点运行一个 capture-agent 实例，抓取该节点的全部流量：

- `hostNetwork: true` — Pod 使用宿主机网络命名空间，可见所有节点网卡
- `nodeSelector: capture-agent-enabled=true` — 仅在打了标签的节点部署
- Capabilities: `CAP_NET_RAW + CAP_NET_ADMIN`（不需要 `privileged: true`）
- ConfigMap 挂载到 `/etc/capture-agent/config.yml`
- 数据目录 `/var/lib/capture-agent` 通过 `hostPath` 持久化

#### 方案 B: 单节点 Deployment

适合测试或指定节点场景，修改 `daemonset.yaml` 的 `kind: DaemonSet` → `kind: Deployment` 并添加 `replicas: 1`。

---

### Prometheus 集成

`base/service.yaml` 部署了一个 Headless Service（`clusterIP: None`），Prometheus 可通过以下方式发现所有 Pod：

```yaml
# prometheus scrape config（添加到 prometheus.yml）
- job_name: capture-agent
  kubernetes_sd_configs:
    - role: endpoints
      namespaces:
        names: [monitoring]
  relabel_configs:
    - source_labels: [__meta_kubernetes_service_name]
      action: keep
      regex: capture-agent-metrics
```

如果集群安装了 Prometheus Operator，可以额外添加 ServiceMonitor：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: capture-agent
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: capture-agent
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

---

### 重要注意事项

1. **`hostNetwork: true` 是必须的** — Pod 需要直接访问宿主机网卡抓包
2. **Capabilities** — 最小权限方案: `CAP_NET_RAW + CAP_NET_ADMIN`；如 PSP/SCC 不允许，改用 `privileged: true`
3. **节点标签** — 默认只在 `capture-agent-enabled=true` 节点上运行，移除 `nodeSelector` 可全量铺开
4. **网络策略** — 确保 capture-agent Pod 可以访问 Kafka、Prometheus 可以访问 9091 端口
5. **镜像仓库** — 生产环境将 `overlays/prod/kustomization.yaml` 中的 `newName`/`newTag` 改为内网仓库地址

---

## 故障排查

### 问题 1: 权限不足

```bash
# 错误: socket: operation not permitted
# 解决: 检查用户权限或 capabilities

# 方法 1: 使用 root 用户
sudo systemctl restart capture-agent

# 方法 2: 添加 capabilities（非 root 用户）
sudo setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/capture-agent
```

### 问题 2: 无法抓到流量

```bash
# 检查网卡是否存在
ip link show

# 检查混杂模式
ip link show eth0 | grep PROMISC

# 手动启用混杂模式
sudo ip link set eth0 promisc on

# 验证抓包（tcpdump 测试）
sudo tcpdump -i eth0 -n -c 10
```

### 问题 3: Kafka 连接失败

```bash
# 检查网络连通性
telnet kafka-broker 9092

# 检查 DNS 解析
nslookup kafka-broker

# 查看 capture-agent 日志
sudo journalctl -u capture-agent -e | grep -i kafka
```

### 问题 4: K8s Pod 无法启动

```bash
# 查看 Pod 事件
kubectl describe pod -n monitoring <pod-name>

# 查看容器日志
kubectl logs -n monitoring <pod-name>

# 检查安全策略
kubectl get psp  # PodSecurityPolicy
kubectl get scc  # SecurityContextConstraints (OpenShift)
```

### 问题 5: 静态二进制依赖缺失

```bash
# 验证完全静态链接
ldd capture-agent
# 应显示: "not a dynamic executable"

# 如果显示依赖，重新构建
make clean
make build-static
```

---

## 性能调优

### 系统参数优化

```bash
# /etc/sysctl.d/99-capture-agent.conf
net.core.rmem_max=134217728
net.core.rmem_default=134217728
net.core.netdev_max_backlog=5000
net.ipv4.tcp_rmem=4096 87380 134217728

# 应用配置
sudo sysctl -p /etc/sysctl.d/99-capture-agent.conf
```

### capture-agent 配置调优

```yaml
# /etc/capture-agent/config.yml
capture:
  afpacket:
    block_size: 8388608      # 8MB（高流量场景）
    num_blocks: 256          # 增加 ring buffer 块数
    fanout_type: hash        # 多核负载均衡
```

---

## 安全建议

1. **最小权限原则**:
   ```bash
   # 仅授予必要的 capabilities
   sudo setcap cap_net_raw,cap_net_admin+eip /usr/local/bin/capture-agent
   ```

2. **网络隔离**:
   - 限制 capture-agent 仅能访问 Kafka/Loki
   - 使用防火墙规则或 K8s NetworkPolicy

3. **日志审计**:
   - 定期审查 capture-agent 日志
   - 监控异常抓包行为

4. **加密传输**:
   - 配置 Kafka TLS/SASL 认证（`capture-agent.kafka.sasl` + `capture-agent.kafka.tls`）
   - 使用 NetworkPolicy 限制 capture-agent Pod 的入站流量

---

## 附录

### A. 支持的 Linux 发行版测试矩阵

| 发行版 | 版本 | 架构 | 测试状态 |
|--------|------|------|----------|
| Ubuntu | 22.04 LTS | amd64 | ✅ Passed |
| Ubuntu | 22.04 LTS | arm64 | ✅ Passed |
| Debian | 12 (Bookworm) | amd64 | ✅ Passed |
| RHEL | 8.7 | amd64 | 🟡 Pending |
| SUSE | 15 SP4 | amd64 | 🟡 Pending |
| Alpine | 3.18 | amd64 | ✅ Passed |

### B. 资源需求估算

| 场景 | CPU | 内存 | 磁盘 I/O |
|------|-----|------|----------|
| 低流量 (<10K pps) | 0.5 核 | 512 MB | 低 |
| 中流量 (10K-100K pps) | 1-2 核 | 1-2 GB | 中 |
| 高流量 (100K-500K pps) | 2-4 核 | 2-4 GB | 高 |

### C. 相关文档

- [配置参考](../configs/config.yml)
- [架构设计](architecture.md)
- [开发指南](implementation-plan.md)

---

**更新时间**: 2026-02-22  
**维护者**: capture-agent 开发团队
