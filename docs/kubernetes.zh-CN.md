# Kubernetes 部署

本文描述 Soniq 第一版 Kubernetes deployment foundation。它包含
`deploy/kubernetes/base/` 下便于直接检查的 raw manifests，也包含
`deploy/helm/soniq/` 下用于参数化部署的第一版 Helm chart。

## 范围

raw manifests 和 Helm chart 只部署 Soniq 自己拥有的资源：

- `soniq-migrate` Job。
- `soniq-api` Deployment 和 Service。
- `soniq-worker` Deployment。
- `soniq-config` ConfigMap。
- `soniq-secret` 引用；raw manifests 里有 example Secret，Helm 里可以选择性渲染 Secret。
- raw manifests 里有 `soniq` Namespace，Helm 里可以选择性渲染 Namespace。

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

Helm chart 相关工作也使用同样的“本机工具优先，否则 Docker fallback”模式。可以用下面命令检查 Helm CLI：

```bash
make helm-version
```

如果本机没有安装 `helm`，Makefile 会 fallback 到 Docker 里的 Helm 镜像。

校验 Helm chart：

```bash
make helm-lint
make helm-template
```

默认 chart 会渲染 ServiceAccount、ConfigMap、API Service、API Deployment、
worker Deployment，以及作为 pre-install/pre-upgrade hook 的 migration Job。它会引用
`soniq-secret`，但默认不会渲染 Secret，所以真实 secret material 不会进入已提交的
values。它也默认跟随 Helm release namespace，而不是创建 Namespace resource。

如果只是想在本地渲染可选的 Namespace 和占位 Secret 路径：

```bash
HELM_TEMPLATE_ARGS='--set namespace.create=true --set secret.create=true --set migrate.hook.enabled=false --set-string secret.stringData.POSTGRES_DSN=postgres://soniq_user:change-me@postgres.example.com:5432/soniq?sslmode=require --set-string secret.stringData.S3_ACCESS_KEY=change-me --set-string secret.stringData.S3_SECRET_KEY=change-me' \
  make helm-template
```

真实 Helm install 时，先创建 namespace 和 Secret，然后安装到这个 namespace：

```bash
kubectl create namespace soniq --dry-run=client -o yaml | kubectl apply -f -
kubectl -n soniq create secret generic soniq-secret \
  --from-literal=POSTGRES_DSN='postgres://soniq_user:change-me@postgres.example.com:5432/soniq?sslmode=require' \
  --from-literal=S3_ACCESS_KEY='change-me' \
  --from-literal=S3_SECRET_KEY='change-me' \
  --dry-run=client -o yaml | kubectl apply -f -
```

然后带上私有配置覆盖来应用 chart：

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  --wait \
  --timeout 5m \
  -f values.production.yaml
```

默认情况下，migration Job 会作为 Helm `pre-install,pre-upgrade` hook 执行，所以
migrations 成功完成后才会继续应用 API/worker 等应用资源。因为 pre-install hook
会在普通 chart resources 创建前执行，所以 `soniq-secret` 引用的 Secret 必须在
install 前已经存在。hook 会直接从 chart values 接收非敏感 config，从预先存在的
Secret 接收敏感值。只有在明确想单独管理 migration Job 时，才设置
`migrate.hook.enabled=false`。chart 会拒绝 `secret.create=true` 和默认 migration
hook 同时开启，因为这个组合在首次 install 时无法工作。

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

默认情况下，kind smoke 还会注册或登录一个 smoke 用户，通过 port-forward 后的
API 上传一个生成的 WAV 文件，等待 Temporal workflow 完成，校验 transcript、
summary 和 mind map 已经持久化，然后继续验证 recording 生命周期：soft delete、
Trash restore、再次 soft delete、永久 purge、purge artifact cleanup rows，以及
S3 object 删除。如果只想跑部署 readiness 部分，可以设置 `KIND_SMOKE_WORKFLOW=0`。

通过 Helm chart 运行同一条 kind smoke：

```bash
make k8s-smoke-kind-helm
```

这条路径仍然使用 Compose 管理的 Postgres、Temporal 和 MinIO，但会用
`helm upgrade --install` 部署 Soniq API、worker 和 migration hook。因为 migration
hook 需要在普通 chart resources 应用前读取 Secret，smoke 脚本会先创建外部
`soniq-secret`，再调用 Helm。如果本机没有安装 `helm`，这个 target 会通过
`scripts/helm-cluster.sh` 使用 Dockerized Helm，并通过 host network 和挂载的
`~/.kube` 访问本地 kind API server。

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

Helm chart 已经通过 pre-install/pre-upgrade hooks 处理 migration 重跑；当前 raw-manifest 路径保持行为显式。

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

- 还没有 Ingress 或 TLS manifest。
- 还没有 HPA、PDB、NetworkPolicy 或 topology spread constraints。
- 还没有远程集群 release smoke；当前 cluster smoke 覆盖的是本地 kind 下的
  raw manifests 和 Helm 两条路径。
- Worker 还没有暴露 HTTP health endpoint，所以 worker health 当前主要依赖进程状态和日志。
