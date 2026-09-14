# Kubernetes

- [deployment.yaml](deployment.yaml): duas replicas iniciais;
- [pdb.yaml](pdb.yaml): protecao contra interrupcao voluntaria;
- [hpa.yaml](hpa.yaml): escala de duas a dez replicas por CPU;
- [overlays/local](overlays/local): imagem local para desenvolvimento;
- [overlays/prod](overlays/prod): imagem publicada no Docker Hub.

## Pre-requisitos

O namespace `video-processor` precisa existir e conter o Secret
`video-processor-secrets`, os Services `postgres`, `rabbitmq` e `mongo` e o PVC
compartilhado `video-processor-data`. O worker tambem depende da API e do
RabbitMQ estarem disponiveis.

Esses recursos sao gerenciados pelo ambiente da API. Este repositorio nao aplica
nem recria recursos de outro repositorio.

O Metrics Server e necessario para o HPA.

```bash
kubectl -n video-processor get pods -o wide
```

## Desenvolvimento local

Construa a imagem localmente na raiz deste repositorio e aplique o overlay local:

```bash
docker build -t video-processor-worker:latest .
kubectl config use-context docker-desktop
kubectl kustomize --load-restrictor LoadRestrictionsNone k8s/overlays/local | kubectl apply -f -
```

O overlay local usa `video-processor-worker:latest` e `imagePullPolicy:
IfNotPresent`.

## Producao

O workflow de CI/CD publica `nuriancoelho/video-processor-worker` no Docker Hub
quando ocorre push na branch `main`. Tambem sao publicadas tags com o SHA do
commit.

O overlay de producao usa `nuriancoelho/video-processor-worker:latest`:

```bash
kubectl kustomize --load-restrictor LoadRestrictionsNone k8s/overlays/prod | kubectl apply -f -
```

Para uma versao imutavel, altere a tag em
[overlays/prod/kustomization.yaml](overlays/prod/kustomization.yaml), por
exemplo `sha-abc1234`, antes de aplicar.

## Verificacao

```bash
kubectl -n video-processor get deployment/video-processor-worker
kubectl -n video-processor get pods -l app=video-processor-worker
kubectl -n video-processor get hpa/video-processor-worker
kubectl -n video-processor logs deployment/video-processor-worker --tail=100
```

Se o HPA mostrar `cpu: <unknown>`, verifique o Metrics Server:

```bash
kubectl top nodes
kubectl -n video-processor describe hpa/video-processor-worker
```

Depois de publicar uma nova imagem com a tag `latest`, force a atualizacao:

```bash
kubectl -n video-processor rollout restart deployment/video-processor-worker
kubectl -n video-processor rollout status deployment/video-processor-worker
```
