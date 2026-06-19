# 备份和恢复 Runbook

这份 runbook 只覆盖 Soniq 应用自己的数据。Soniq 使用外部 Postgres、Temporal 和
S3-compatible object storage；这些依赖各自有自己的备份责任边界。

## 数据归属

需要备份的 Soniq 数据：

- Soniq application Postgres：users、workspaces、sessions、CSRF state、
  recordings、transcript rows、summary rows、mind maps、soft-delete state 和
  purge cleanup rows。
- S3-compatible object storage bucket：保留的 original audio、normalized audio
  和生成的二进制 artifacts。

下面这些不通过 Soniq 应用 runbook 备份：

- Temporal history 和 visibility storage。它们应该按照 Temporal 部署自己的策略备份。
  不要把 Soniq application migrations 跑到 Temporal 的数据库上。
- Kubernetes Secret 内容。它应该通过集群 secret manager、External Secrets、Vault
  或云厂商 secret manager 恢复。
- Helm 私有 values 和基础设施定义。它们应该保存在私有部署仓库或基础设施状态系统中。

## 备份一致性

最安全的是维护窗口备份：

1. 先停止 Soniq API 和 worker，阻止新的写入。
2. 备份 Soniq application Postgres。
3. 备份 object storage bucket。
4. 重新启动 API 和 worker。

热备也可以做，但需要额外的存储能力保证一致性。如果环境需要不停机的 point-in-time
recovery，应该使用 Postgres PITR/WAL archiving，再配合 object storage versioning、
snapshot 或 replication。

如果热备只使用 `pg_dump` 加 `s3 sync`，优先先 dump Postgres，再 sync object。这样
backup 里可能多出一些数据库没有引用的 object，但能降低恢复后数据库行指向“备份里不存在
的 object”的概率。

## 维护暂停

Helm 部署下，严格备份或恢复前先暂停 API 和 worker。如果 API autoscaling 已开启，需要
在暂停时关闭它，否则 HPA 可能重新创建 API pods。

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  --reuse-values \
  --set api.autoscaling.enabled=false \
  --set api.replicas=0 \
  --set worker.autoscaling.enabled=false \
  --set worker.replicas=0 \
  --set migrate.enabled=false \
  --wait \
  --timeout 5m
```

raw manifests 部署下，如果 API HPA 存在，先删除或禁用 HPA，再把 API 和 worker 缩到 0：

```bash
kubectl -n soniq delete hpa soniq-api --ignore-not-found
kubectl -n soniq scale deployment/soniq-api --replicas=0
kubectl -n soniq scale deployment/soniq-worker --replicas=0
kubectl -n soniq rollout status deployment/soniq-api --timeout=120s
kubectl -n soniq rollout status deployment/soniq-worker --timeout=120s
```

恢复 Helm deployment 时，应该重新使用真实 production values 文件，而不是复用临时暂停参数：

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  -f values.production.yaml \
  --wait \
  --timeout 5m
```

raw manifests 部署下，把 deployments 缩回来；如果环境使用 API HPA，再重新 apply HPA：

```bash
kubectl -n soniq scale deployment/soniq-api --replicas=2
kubectl -n soniq scale deployment/soniq-worker --replicas=2
kubectl apply -f deploy/kubernetes/base/api-hpa.yaml
```

## 备份流程

在仓库外部创建 backup 目录，并记录 release context：

```bash
export BACKUP_ROOT="${BACKUP_ROOT:-../soniq-backups}"
export BACKUP_ID="$(date -u +%Y%m%dT%H%M%SZ)"
export BACKUP_DIR="${BACKUP_ROOT}/${BACKUP_ID}"
mkdir -p "${BACKUP_DIR}"

kubectl -n soniq get deployment,job,service,hpa,pdb,networkpolicy,configmap,serviceaccount \
  -l app.kubernetes.io/part-of=soniq \
  -o yaml > "${BACKUP_DIR}/kubernetes-resources.yaml"
```

不要把 Kubernetes Secret 值写入 backup 目录，除非这个目录已经按照生产备份策略加密并做了访问控制。

用 Postgres custom format 备份 Soniq application Postgres：

```bash
pg_dump "${POSTGRES_DSN}" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file "${BACKUP_DIR}/soniq.postgres.dump"
```

用通用 object sync 备份 S3-compatible bucket：

```bash
aws --endpoint-url "${S3_ENDPOINT}" \
  s3 sync "s3://${S3_BUCKET}/" "${BACKUP_DIR}/objects/"
```

如果使用 MinIO 兼容工具，也可以用：

```bash
mc mirror "${MC_ALIAS}/${S3_BUCKET}" "${BACKUP_DIR}/objects"
```

记录 backup metadata：

```bash
cat > "${BACKUP_DIR}/metadata.txt" <<EOF
backup_id=${BACKUP_ID}
created_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
postgres_dump=soniq.postgres.dump
object_backup=objects/
EOF
```

## 恢复流程

恢复应该在维护窗口内执行。Postgres 和 object storage 都恢复完成之前，不要启动 API 和
worker。

1. 重建目标 Soniq application Postgres database。
2. 重建或清空目标 S3-compatible bucket。
3. 恢复 objects。
4. 恢复 Postgres dump。
5. 按目标环境重新创建 Kubernetes Secret/config。
6. 在恢复后的数据库上运行 Soniq migrations。
7. 启动 API 和 worker。

恢复 objects：

```bash
aws --endpoint-url "${S3_ENDPOINT}" \
  s3 sync "${BACKUP_DIR}/objects/" "s3://${S3_BUCKET}/"
```

恢复 Postgres：

```bash
pg_restore \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  --dbname "${POSTGRES_DSN}" \
  "${BACKUP_DIR}/soniq.postgres.dump"
```

对目标 Soniq release 运行 migrations：

```bash
kubectl -n soniq delete job soniq-migrate --ignore-not-found
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
kubectl -n soniq wait --for=condition=complete job/soniq-migrate --timeout=120s
kubectl -n soniq logs job/soniq-migrate
```

Helm 部署则走正常 install 或 upgrade，让 migration hook 执行：

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  -f values.production.yaml \
  --wait \
  --timeout 5m
```

## Temporal 恢复

已完成 recording 的结果存储在 Soniq Postgres 和 object storage 中。Temporal history
主要用于恢复备份发生时还在进行中的 workflow。

如果要恢复 Temporal，需要使用 Temporal 自己的 database backup，并确保它和同一个
Soniq release、同一个 task queue 配置匹配。

如果不恢复 Temporal，就把事故发生时还在 processing 的 workflow 当成中断处理。API 和
worker 恢复后，检查上传中或处理中状态的 recordings，并在合适时通过 Soniq retry 路径重试。

## 恢复后验证

检查 API health 和 readiness：

```bash
kubectl -n soniq rollout status deployment/soniq-api
kubectl -n soniq rollout status deployment/soniq-worker
kubectl -n soniq port-forward service/soniq-api 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

检查代表性数据：

- 用已知测试账号登录，或者创建新的 admin/test account。
- 打开几个已有 recordings，检查 transcript、summary、mind map 和 Trash 状态。
- 用 `HeadObject` 或等价工具确认至少一个恢复的 object 存在。
- 上传一个 fake-provider 小音频，确认状态可以走到 completed。
- 查看 worker logs，确认没有持续 retry 或 missing object errors。

## 频率和保留

生产环境基线建议：

- Postgres：至少每天一次 full dump；如果 RPO 要求更高，使用 PITR/WAL archiving。
- Object storage：至少每天一次 bucket backup；严格恢复点优先使用 versioning 或 provider
  snapshots。
- Secrets：从 secret management system 恢复，不从已提交文件恢复。
- Restore drills：每个 release train 至少演练一次，或者生产上线前至少演练一次。

实际环境的 RPO、RTO、保留周期、加密策略和恢复负责人如果依赖公司策略，应记录在这个仓库外部。
