Snapshot Manager
=

Kubernetes controller for easier management of [VolumeSnapshots](https://kubernetes.io/docs/concepts/storage/volume-snapshots/)
supporting HA application deployments. Introduces two CRD `SnapshotRevert` for 
a convenient way to revert your application to a specific point in time when 
`VolumeSnapshots` were taken for each replica, and customizable `ValidationStrategy` 
that tries to best effort validate the application snapshots.

### Usage
The [kubectl-snapshotmanager](https://github.com/cisco-sso/snapshot-manager/blob/master/tools/kubectl-snapshotmanager) is a [kubectl plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/) simplifying usage of snapshot manager to an end user.

Install it and call `kubectl snapshotmanager` from your CLI to get this information about how to use it:
```
kubectl-snapshotmanager is kubectl plugin for VolumeSnapshots
Commands:
  describe  Show volume snapshots related to a specified resource
            requires either statefulset/sts persistentvolumeclaim/pvc or pod/po and name of the resource
  create    Create a snapshot-manager maintained resources, VolumeSnapshot/SnapshotRevert
            shortcut for simple creation of resources defined by volume snapshot CRDs, initialized with default values
  revert    Revert application defined in specified SnapshotRevert to certain set of snapshots
  undo      Undo latest revert in specified SnapshotRevert

Examples:
  # Describe PVCs and snapshots for mysts StatefulSet
  kubectl snapshotmanager describe sts mysts

  # Create mysts SnapshotRevert resource for mysts StatefulSet
  kubectl snapshotmanager create snapshotrevert --sts=mysts mysts-revert

  # Revert mysts StatefulSet to latest set of VolumeSnapshots
  kubectl snapshotmanager revert mysts-revert

  # Undo latest revert of mysts StatefulSet
  kubectl snapshotmanager undo mysts-revert

Usage:
  kubectl snapshotmanager <command> [options]
```

Example describing snapshots for StatefulSet cassandra:
```
$ kubectl snapshotmanager describe sts cassandra
POD          PVC               SNAPSHOT                                               DATA                                                      TIMESTAMP
cassandra-0  data-cassandra-0  data-cassandra-0-25e6108c-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-34af85e5-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:54Z
cassandra-1  data-cassandra-1  data-cassandra-1-24d93e04-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-31c73f67-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:52Z
cassandra-2  data-cassandra-2  data-cassandra-2-254da2d6-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-33cf66aa-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:53Z
cassandra-3  data-cassandra-3  data-cassandra-3-24da2c88-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-2cb2f890-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:52Z
cassandra-4  data-cassandra-4  data-cassandra-4-260f1bb2-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-3544c1f8-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:54Z
cassandra-5  data-cassandra-5  data-cassandra-5-25ccb2b4-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-3456eea2-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:54Z
cassandra-6  data-cassandra-6  data-cassandra-6-24d24448-35df-11e9-98a5-0a580ae96404  k8s-volume-snapshot-291de6a6-35df-11e9-93e0-0a580ae94bad  2019-02-21T13:46:52Z
```

### Example
In case your cluster is older than 1.13.x, you can use the raw CRDs as well.
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
