#!/bin/bash

export HELM="${DEBUG}helm"
export KUBECTL="${DEBUG}kubectl"

kubectl create -f deploy-01-init.yaml
kubectl create -f deploy-02-service.yaml
kubectl create -f deploy-03-statefulset.yaml
./deploy-04-snapshot-controllers.bash
kubectl create -f deploy-05-snapshot-manager.yaml
echo sleep 300
sleep 300
kubectl delete deployment snapshot-manager
kubectl create -f manual-deploy-05-job.yaml
echo sleep 30
sleep 30
kubectl create -f manual-deploy-06-snapshotpolicy.yaml
echo sleep 60
sleep 60
kubectl create -f manual-deploy-05-job2.yaml
