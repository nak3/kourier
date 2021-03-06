#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

KOURIER_NAMESPACE=kourier-system
KNATIVE_NAMESPACE=knative-serving

if ! command -v microk8s.kubectl >/dev/null; then
  echo "You need to install microk8s"
  exit 1
fi

tag="test_$(git rev-parse --abbrev-ref HEAD)"
# In CircleCI, PR branches that come from forks have the format "pull/n", where
# n is the PR number. "/" is not accepted in docker tags, so we need to replace
# it.
tag=$(echo "$tag" | tr / -)

KNATIVE_VERSION=v0.11.1
# Deploys kourier and patches it.
microk8s.kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-crds.yaml
microk8s.kubectl apply -f https://github.com/knative/serving/releases/download/${KNATIVE_VERSION}/serving-core.yaml
mkdir -p "$HOME"/.kube/
microk8s.kubectl config view --raw >"$HOME"/.kube/config
chown -R circleci "$HOME"/.kube

#Builds the kourier image and imports into the k8s cluster
docker build -t 3scale-kourier:"$tag" ./
docker image save 3scale-kourier:"$tag" >image.tar
microk8s.ctr --namespace k8s.io images import image.tar

# Builds and imports the kourier and gateway images from docker into the k8s cluster
docker build -f ./utils/extauthz_test_image/Dockerfile -t test_externalauthz:test ./utils/extauthz_test_image/
docker image save test_externalauthz:test >image.tar
microk8s.ctr --namespace k8s.io images import image.tar

# Enable the microk8s DNS plugin
microk8s.enable dns

# Deploys kourier and patches it.
microk8s.kubectl apply -f deploy/kourier-knative.yaml
microk8s.kubectl patch deployment 3scale-kourier-control -n ${KOURIER_NAMESPACE} --patch "{\"spec\": {\"template\": {\"spec\": {\"containers\": [{\"name\": \"kourier-control\",\"image\": \"3scale-kourier:$tag\",\"imagePullPolicy\": \"IfNotPresent\"}]}}}}"
microk8s.kubectl patch configmap/config-domain -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"127.0.0.1.nip.io":""}}'
microk8s.kubectl patch configmap/config-network -n ${KNATIVE_NAMESPACE} --type merge -p '{"data":{"clusteringress.class":"kourier.ingress.networking.knative.dev","ingress.class":"kourier.ingress.networking.knative.dev"}}'

retries=0
while [[ $(microk8s.kubectl get pods -n ${KOURIER_NAMESPACE} -l app=3scale-kourier-control -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier control pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timed out waiting for kourier control pod"
    exit 1
  fi
  retries=$((retries + 1))
done

retries=0
while [[ $(microk8s.kubectl get pods -n ${KOURIER_NAMESPACE} -l app=3scale-kourier-gateway -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}') != "True" ]]; do
  echo "Waiting for kourier gateway pod to be ready "
  sleep 10
  if [ $retries -ge 7 ]; then
    echo "timed out waiting for kourier gateway pod"
    exit 1
  fi
  retries=$((retries + 1))
done

microk8s.kubectl port-forward --namespace ${KOURIER_NAMESPACE} "$(microk8s.kubectl get pod -n ${KOURIER_NAMESPACE} -l "app=3scale-kourier-gateway" --output=jsonpath="{.items[0].metadata.name}")" 8080:8080 19000:19000 &>/dev/null &
