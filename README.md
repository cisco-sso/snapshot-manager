Snapshot Manager
=

Kubernetes controller for easier management of [VolumeSnapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/)
supporting HA application deployments. Introduces two CRD `SnapshotRevert` for 
a convenient way to revert your application to a specific point in time when 
`VolumeSnapshots` were taken for each replica, and customizable `ValidationStrategy` 
that tries to best effort validate the application snapshots.

### Example

The [cassandra](/tests/cassandra/deployment/) example contains a walk through 
how to use snapshot-manager for reverts of snapshots taken with `SnapshotPolicy`.

Cassandra `StatefulSet` defines the application
```yaml
kind: StatefulSet
apiVersion: apps/v1
metadata:
  labels:
    app: cassandra
  name: cassandra
spec:
  replicas: 5
  selector:
    matchLabels:
      app: cassandra
  serviceName: cassandra
...
```

`SnapshotPolicy` ensures periodic snapshots of each PV that cassandra `StatefulSet` manages using matching label selector.
```yaml
apiVersion: snapshotpolicy.ciscosso.io/v1alpha1
kind: SnapshotPolicy
metadata:
  name: cassandra-snapshots
spec:
  selectors:
  - matchLabels:
      app: cassandra
  unit: hour
  period: 24
  retention: 1
  strategy:
    name: inuse
```

`SnapshotRevert` is a description of actions to be taken on cassandra `StatefulSet`
as well as collection of operations that has already been performed.
```yaml
apiVersion: snapshotmanager.ciscosso.io/v1alpha1
kind: SnapshotRevert 
metadata:
  name: cassandra-revert
spec:
  statefulSet:
    name: cassandra
    claim: data
    snapshotClaimStorageClass: snapshot
```

In order to use the revert, a patch on `SnapshotRevert` tells the snapshot-manager
to take an action
```
$ kubectl patch snapshotrevert cassandra-revert --patch='{"spec":{"action":{"type":"latest"}}}' --type=merge
```

Snapshot manager will scale-down the `StatefulSet`, create appropriate `PVCs` from 
`VolumeSnapshots`, and finally scale-up the `StatefulSet` back. During this, it records
previous `PVCs` and sets old `PVs` reclaim policy to not lose the original data for
potential undo action.

Undo action is again a simple patch
```
$ kubectl patch snapshotrevert cassandra-revert --patch='{"spec":{"action":{"type":"undo"}}}' --type=merge
```

Also check out [snapshot policy](https://github.com/cisco-sso/snapshotpolicy/),
which complements snapshot-manager with periodic snapshots.
