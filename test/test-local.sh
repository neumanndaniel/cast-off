#!/bin/bash

# SPDX-License-Identifier: Apache-2.0
# Copyright Daniel Neumann

CILIUM_START_VERSION=$1
CILIUM_UPGRADE_VERSION=$2
L7_PROXY_TEST=$3

make docker-build --directory=../

kind create cluster --config=kind.yaml
kind load docker-image quay.io/neumanndaniel/cast-off:dev --name kind

CONTROL_PLANE_NODE_IPS=$(kubectl get nodes -l node-role.kubernetes.io/control-plane="" -o jsonpath='{range .items[*]}{range .status.addresses[?(@.type=="InternalIP")]}{"https://"}{.address}{":6443 "}{end}{end}')

# Cilium
helm repo add cilium https://helm.cilium.io/ || true
helm repo update
helm upgrade --install cilium cilium/cilium --version $CILIUM_START_VERSION \
    --namespace kube-system \
    --set k8s.apiServerURLs="$CONTROL_PLANE_NODE_IPS" \
    --set l7Proxy=$L7_PROXY_TEST \
    --values cilium.yaml

cilium status --wait --wait-duration 5m

# Cast Off
helm upgrade --install cast-off ../charts/cast-off \
    --wait \
    --set config.checkIntervalMinutes=1 \
    --set image.tag=dev \
    --namespace cast-off \
    --create-namespace

read -r -p "Cast Off deployed. Press Enter to continue with Cilium upgrade..."

helm upgrade --install cilium cilium/cilium --version $CILIUM_UPGRADE_VERSION \
    --namespace kube-system \
    --set k8s.apiServerURLs="$CONTROL_PLANE_NODE_IPS" \
    --set l7Proxy=$L7_PROXY_TEST \
    --values cilium.yaml

cilium status --wait --wait-duration 10m

read -r -p "Deleted kind cluster? Press Enter to delete the cluster..."
kind delete cluster --name kind
