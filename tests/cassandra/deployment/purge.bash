#!/bin/bash

#export DEBUG="echo "
export HELM="${DEBUG}helm"
export KUBECTL="${DEBUG}kubectl"

$HELM delete --purge volumesnapshot
$HELM delete --purge snapshotpolicy

$KUBECTL delete namespace snapshots
