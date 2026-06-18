# Kubernetes 部署

本文描述 Soniq 第一版 Kubernetes deployment foundation。它刻意使用 `deploy/kubernetes/base/` 下的 raw manifests，而不是 Helm，这样部署契约更容易直接检查。

## 范围

base manifests 只部署 Soniq 自己拥有的资源：

- `soniq-migrate` Job。
- `soniq-api` Deployment 和 Service。
- `soniq-worker` Deployment。
- `soniq-config` ConfigMap。
- `soniq-secret` 示例 Secret。
- `soniq` Namespace。

它们不部署 Postgres、Temporal、MinIO、Ingress、TLS certificates、Prometheus、Grafana 或 external secret controllers。

## 前置条件

在把 manifests 应用到真实集群之前，需要先准备：

- 一个 Soniq application Postgres 数据库。
- 一个 Temporal frontend address 和 namespace。
- 一个 S3-compatible bucket 和 credentials。
- 已构建并推送的镜像：
  - `soniq-api`
  - `soniq-worker`
  - `soniq-migrate`

在仓库根目录构建本地镜像 tag：

```bash
make docker-build
```

如果是远程集群，需要把镜像推送到 registry，并更新这些文件里的 `image` 字段：

- `deploy/kubernetes/base/api-deployment.yaml`
- `deploy/kubernetes/base/worker-deployment.yaml`
- `deploy/kubernetes/base/migrate-job.yaml`

## 配置

编辑 `deploy/kubernetes/base/configmap.yaml`，设置非敏感配置：

- `APP_PUBLIC_URL`
- `TEMPORAL_ADDRESS`
- `TEMPORAL_NAMESPACE`
- `TEMPORAL_TASK_QUEUE`
- `S3_ENDPOINT`
- `S3_REGION`
- `S3_BUCKET`
- `S3_FORCE_PATH_STYLE`
- provider selection 和 model names

基于 `deploy/kubernetes/base/secret.example.yaml` 创建真实 Secret，并在真实部署前替换所有 `change-me` 值。这个 example 文件可以安全提交，因为它只包含占位值。

必需的 Secret keys：

- `POSTGRES_DSN`
- `S3_ACCESS_KEY`
- `S3_SECRET_KEY`

Provider-specific Secret keys：

- `TRANSCRIPTION_API_KEY`
- `MIMO_API_KEY`
- `DASHSCOPE_API_KEY`
- `LLM_API_KEY`

## 校验 Manifests

本地渲染 base manifests：

```bash
make k8s-render
```

这个 target 会执行 `kubectl kustomize deploy/kubernetes/base`。如果本机没有
安装 `kubectl`，Makefile 会 fallback 到 Docker 里的 kubectl 镜像。

运行本地 manifest smoke：

```bash
make k8s-smoke
```

这个脚本会渲染 base manifests，并检查预期的 Kubernetes resource contract。
这个 smoke 需要 `python3` 和 PyYAML，不需要 Kubernetes API server。

当有可用集群时，运行 server-side dry-run：

```bash
make k8s-dry-run
```

Server-side dry-run 会请求 Kubernetes API server 校验 resource kinds 和 schemas，
但不会真的持久化这些应用对象。如果 `soniq` namespace 不存在，带
`K8S_SMOKE_SERVER_DRY_RUN=1` 的 `make k8s-smoke` 会为了校验临时创建它，并在结束后删除。

运行本地 kind 部署 smoke：

```bash
make k8s-smoke-kind
```

这个脚本会启动 Compose 管理的 Postgres、Temporal 和 MinIO，创建或复用
`soniq` kind 集群，把 kind control-plane 容器连接到 Compose Docker network，
然后应用一份临时 smoke manifest。Soniq API、worker 和 migrate 仍然运行在
Kubernetes 里，但 Compose 依赖会通过 Kubernetes `Service` 和 `EndpointSlice` 暴露给 Pod：

- `soniq-postgresql:5432`
- `temporal:7233`
- `minio:9000`

这样 Pod 不需要使用 `localhost`。在 Pod 里，`localhost` 指的是 Pod 自己，
不是开发机，也不是 Compose 容器。

## Apply

先应用 config 和 secret：

```bash
kubectl apply -f deploy/kubernetes/base/namespace.yaml
kubectl apply -f deploy/kubernetes/base/configmap.yaml
kubectl apply -f deploy/kubernetes/base/secret.example.yaml
```

运行 migrations：

```bash
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
kubectl -n soniq wait --for=condition=complete job/soniq-migrate --timeout=120s
kubectl -n soniq logs job/soniq-migrate
```

然后启动 API 和 worker：

```bash
kubectl apply -f deploy/kubernetes/base/api-deployment.yaml
kubectl apply -f deploy/kubernetes/base/api-service.yaml
kubectl apply -f deploy/kubernetes/base/worker-deployment.yaml
```

等待 API ready：

```bash
kubectl -n soniq rollout status deployment/soniq-api
kubectl -n soniq get pods
```

如果没有 Ingress，只是本地检查：

```bash
kubectl -n soniq port-forward service/soniq-api 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

如果 `AUTH_COOKIE_SECURE=true`，浏览器登录需要 HTTPS。仅通过本地 port-forward 使用 HTTP 测试时，可以临时在 `soniq-config` 中设置 `AUTH_COOKIE_SECURE=false`。

## 更新 Migrations

Kubernetes Jobs 在 pod template 变化后不会原地更新。raw-manifest 阶段如果要重新跑 migration，需要删除并重新创建 Job：

```bash
kubectl -n soniq delete job soniq-migrate
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
```

后面 Helm pre-install/pre-upgrade hooks 可以处理这个问题；当前 base layer 保持行为显式。

## 运维检查

API readiness：

```bash
kubectl -n soniq exec deploy/soniq-api -- /soniq-api --version
kubectl -n soniq logs deploy/soniq-api
```

Worker logs：

```bash
kubectl -n soniq logs deploy/soniq-worker
```

Migration logs：

```bash
kubectl -n soniq logs job/soniq-migrate
```

## 当前限制

- 还没有 Helm chart。
- 还没有 Ingress 或 TLS manifest。
- 还没有 HPA、PDB、NetworkPolicy 或 topology spread constraints。
- 还没有完整集群级 Kubernetes smoke script。
- Worker 还没有暴露 HTTP health endpoint，所以 worker health 当前主要依赖进程状态和日志。
