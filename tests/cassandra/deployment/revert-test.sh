#!/bin/bash

export HELM="${DEBUG}helm"
export KUBECTL="${DEBUG}kubectl"

kubectl create -f deploy-01-init.yaml
kubectl create -f deploy-07-strategy.yaml
kubectl patch validationstrategy cassandra -p '{"spec":{"autoTrigger":false}}' --type=merge 
kubectl create -f sec.yaml
kubectl create -f deploy-02-service.yaml
kubectl create -f deploy-03-statefulset.yaml
./deploy-04-snapshot-controllers.bash
kubectl create -f deploy-05-snapshot-manager.yaml
echo "waiting, enter to continue"
read -t 300
kubectl delete deployment snapshot-manager
kubectl create -f manual-deploy-05-job.yaml
echo "waiting, enter to continue"
read -t 60
kubectl create -f manual-deploy-06-snapshotpolicy.yaml
echo "waiting, enter to continue"
read -t 60
kubectl create -f manual-deploy-05-job2.yaml
