#!/bin/bash

export HELM="${DEBUG}helm"

$HELM install cisco-sso/volumesnapshot --name=volumesnapshot --namespace=snapshots --values=volume_snapshots_overrides.yaml
$HELM install cisco-sso-private/snapshotpolicy --name=snapshotpolicy --namespace=snapshots --values=snapshotpolicy_overrides.yaml

$KUBECTL create clusterrolebinding --serviceaccount=snapshots:volumesnapshot --clusterrole=cluster-admin columesnapshot-ca
