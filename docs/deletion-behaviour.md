# Deletion behaviour configuration

The provisioner offers multiple layers of configuration to decide what
should happen to a backing directory when the bound PVC or PV is
removed. Settings from more specific scopes always win over more general
ones.

## Order of precedence

1. **PVC annotations**
2. **PersistentVolume annotations** stored during provisioning
3. **StorageClass parameters**
4. **Controller-wide defaults** defined with environment variables

## PVC annotations

| Annotation                 | Accepted values    | Description |
| -------------------------- | ------------------ | ----------- |
| `nfs.io/on-delete`         | `delete`, `retain` | Overrides the deletion strategy for a specific PVC. |
| `nfs.io/archive-on-delete` | `true`, `false`    | Overrides whether the provisioner archives the directory. Ignored when `nfs.io/on-delete` forces `delete` or `retain`. |

## StorageClass parameters

| Parameter         | Accepted values    | Description |
| ----------------- | ------------------ | ----------- |
| `onDelete`        | `delete`, `retain` | Sets the default deletion strategy for all PVCs that use the StorageClass. |
| `archiveOnDelete` | `true`, `false`    | Controls whether volumes are archived when the PVC is deleted. Ignored when `onDelete` is set. |

## Environment variables

| Variable                         | Accepted values      | Description |
| -------------------------------- | -------------------- | ----------- |
| `PROVISIONER_ON_DELETE`         | `delete`, `retain`   | Provides a controller-wide default deletion strategy. |
| `PROVISIONER_ARCHIVE_ON_DELETE` | `true`, `false`      | Sets the global archive behaviour when no other scope specifies it. |

## Example

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: logs
  annotations:
    nfs.io/on-delete: retain
spec:
  accessModes: [ReadWriteMany]
  storageClassName: nfs-client
  resources:
    requests:
      storage: 5Gi
```

The example above ensures the `logs` directory is retained even if the
StorageClass or controller would otherwise delete or archive it.
