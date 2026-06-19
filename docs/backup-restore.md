# Backup and Restore Runbook

This runbook covers Soniq application data only. Soniq uses external Postgres,
Temporal, and S3-compatible object storage; each dependency keeps its own backup
responsibility.

## Data Ownership

Back up these Soniq-owned stores:

- Soniq application Postgres: users, workspaces, sessions, CSRF state,
  recordings, transcript rows, summary rows, mind maps, soft-delete state, and
  purge cleanup rows.
- S3-compatible object storage bucket: original audio when retained, normalized
  audio, and generated binary artifacts.

Back up these outside the Soniq application runbook:

- Temporal history and visibility storage. Use the Temporal deployment's backup
  strategy. Never run Soniq application migrations against Temporal's database.
- Kubernetes Secret material. Store and recover it through the cluster secret
  manager, External Secrets, Vault, or the cloud secret manager used by the
  environment.
- Helm private values and infrastructure definitions. Keep them in the private
  deployment repository or infrastructure state system.

## Backup Consistency

The safest backup is a maintenance-window backup:

1. Stop new writes by taking the Soniq API and worker out of service.
2. Back up Soniq application Postgres.
3. Back up the object storage bucket.
4. Restart API and worker.

Hot backups are possible, but they need extra storage guarantees. Use Postgres
PITR/WAL archiving plus object storage versioning, snapshots, or replication if
the environment needs point-in-time recovery without a maintenance window.

If a hot backup uses only `pg_dump` plus `s3 sync`, prefer dumping Postgres first
and syncing objects second. That can leave harmless extra objects in the backup,
but it reduces the chance that restored database rows point at objects created
after the object backup.

## Maintenance Pause

For a Helm deployment, pause API and worker before a strict backup or restore.
If API autoscaling is enabled, disable it for the pause so the HPA does not
recreate API pods.

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

For raw manifests, remove or disable the API HPA first if it exists, then scale
API and worker to zero:

```bash
kubectl -n soniq delete hpa soniq-api --ignore-not-found
kubectl -n soniq scale deployment/soniq-api --replicas=0
kubectl -n soniq scale deployment/soniq-worker --replicas=0
kubectl -n soniq rollout status deployment/soniq-api --timeout=120s
kubectl -n soniq rollout status deployment/soniq-worker --timeout=120s
```

Resume the Helm deployment from the real production values file, not from the
temporary pause flags:

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  -f values.production.yaml \
  --wait \
  --timeout 5m
```

For raw manifests, scale deployments back and reapply the API HPA if the
environment uses it:

```bash
kubectl -n soniq scale deployment/soniq-api --replicas=2
kubectl -n soniq scale deployment/soniq-worker --replicas=2
kubectl apply -f deploy/kubernetes/base/api-hpa.yaml
```

## Backup Procedure

Create a backup directory outside the repository and capture release context:

```bash
export BACKUP_ROOT="${BACKUP_ROOT:-../soniq-backups}"
export BACKUP_ID="$(date -u +%Y%m%dT%H%M%SZ)"
export BACKUP_DIR="${BACKUP_ROOT}/${BACKUP_ID}"
mkdir -p "${BACKUP_DIR}"

kubectl -n soniq get deployment,job,service,hpa,pdb,networkpolicy,configmap,serviceaccount \
  -l app.kubernetes.io/part-of=soniq \
  -o yaml > "${BACKUP_DIR}/kubernetes-resources.yaml"
```

Do not write Kubernetes Secret values into the backup directory unless the
directory is encrypted and access-controlled by the production backup policy.

Back up Soniq application Postgres with a custom-format dump:

```bash
pg_dump "${POSTGRES_DSN}" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file "${BACKUP_DIR}/soniq.postgres.dump"
```

Back up the S3-compatible bucket with a portable object sync:

```bash
aws --endpoint-url "${S3_ENDPOINT}" \
  s3 sync "s3://${S3_BUCKET}/" "${BACKUP_DIR}/objects/"
```

Alternatively, for MinIO-compatible tooling:

```bash
mc mirror "${MC_ALIAS}/${S3_BUCKET}" "${BACKUP_DIR}/objects"
```

Record backup metadata:

```bash
cat > "${BACKUP_DIR}/metadata.txt" <<EOF
backup_id=${BACKUP_ID}
created_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
postgres_dump=soniq.postgres.dump
object_backup=objects/
EOF
```

## Restore Procedure

Restore in a maintenance window. API and worker should stay stopped until both
Postgres and object storage have been restored.

1. Recreate the target Soniq application Postgres database.
2. Recreate or empty the target S3-compatible bucket.
3. Restore objects.
4. Restore the Postgres dump.
5. Recreate Kubernetes Secret/config from the target environment.
6. Run Soniq migrations only after the restored database is in place.
7. Start API and worker.

Restore objects:

```bash
aws --endpoint-url "${S3_ENDPOINT}" \
  s3 sync "${BACKUP_DIR}/objects/" "s3://${S3_BUCKET}/"
```

Restore Postgres:

```bash
pg_restore \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  --dbname "${POSTGRES_DSN}" \
  "${BACKUP_DIR}/soniq.postgres.dump"
```

Run migrations for the target Soniq release:

```bash
kubectl -n soniq delete job soniq-migrate --ignore-not-found
kubectl apply -f deploy/kubernetes/base/migrate-job.yaml
kubectl -n soniq wait --for=condition=complete job/soniq-migrate --timeout=120s
kubectl -n soniq logs job/soniq-migrate
```

For Helm, use the normal install or upgrade path so the migration hook runs:

```bash
helm upgrade --install soniq deploy/helm/soniq \
  --namespace soniq \
  -f values.production.yaml \
  --wait \
  --timeout 5m
```

## Temporal Recovery

Completed recording results are stored in Soniq Postgres and object storage.
Temporal history is mainly needed to resume workflows that were in progress at
the time of backup.

If Temporal is restored with its own database backup, keep it aligned with the
same Soniq release and task queue configuration.

If Temporal is not restored, treat in-flight workflows as interrupted. After API
and worker are running again, inspect recordings that were uploading or
processing during the incident and retry them through the Soniq retry path where
appropriate.

## Post-Restore Verification

Check API health and readiness:

```bash
kubectl -n soniq rollout status deployment/soniq-api
kubectl -n soniq rollout status deployment/soniq-worker
kubectl -n soniq port-forward service/soniq-api 8080:80
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
```

Check representative data:

- Sign in with a known test account or create a new admin/test account.
- Open several existing recordings and verify transcript, summary, mind map, and
  Trash state.
- Verify at least one restored object exists in the bucket with `HeadObject` or
  equivalent tooling.
- Upload a small fake-provider test recording and confirm it reaches completed
  status.
- Inspect worker logs for retry loops or missing object errors.

## Frequency and Retention

Baseline production guidance:

- Postgres: daily full dump at minimum; use PITR/WAL archiving for tighter RPO.
- Object storage: daily bucket backup at minimum; prefer versioning or provider
  snapshots for strict recovery points.
- Secrets: recover from the secret management system, not from committed files.
- Restore drills: run at least once per release train or before production
  launch.

Document the environment's actual RPO, RTO, retention period, encryption policy,
and restore owner outside this repository if they depend on company policy.
