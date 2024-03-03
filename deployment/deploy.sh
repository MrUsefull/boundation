#!/bin/bash
set -xe

helm repo add bitnami https://charts.bitnami.com/bitnami

helm repo update

# install external-dns with helm
helm upgrade --install external-dns-unbound bitnami/external-dns \
    -f values.yml \
    --set sidecars[0].image=registry.comphychaircoding.net/external-dns-provider-unbound:${CI_COMMIT_SHA} \
    --set sidecars[0].env[1].value=https://router.comphychaircoding.net \
    --namespace externaldns
