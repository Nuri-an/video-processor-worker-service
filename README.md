# Video Processor Worker

Worker responsavel pelo processamento assincrono de videos. Ele nao expoe uma API HTTP publica.

## Responsabilidades

1. Consumir mensagens da fila `video_jobs` no RabbitMQ.
2. Localizar o video recebido pela API.
3. Executar o `ffmpeg` com um frame por segundo.
4. Compactar os frames em um arquivo ZIP.
5. Atualizar o job no PostgreSQL para `Concluido` ou `Erro`.
6. Enviar um e-mail ao usuario quando o processamento falhar.
7. Registrar eventos de processamento no sistema de logs.

## Dependencias

- RabbitMQ: consumo da fila `video_jobs`;
- PostgreSQL: atualizacao dos jobs;
- MongoDB: logs da aplicacao;
- `ffmpeg`: extracao dos frames;
- filesystem compartilhado: videos de entrada e ZIPs de saida.

## Variaveis de ambiente

| Variavel | Padrao | Uso |
|---|---|---|
| `WORKER_STORAGE_DIR` | `uploads` | Diretorio dos videos de entrada |
| `WORKER_OUTPUT_DIR` | `outputs` | Diretorio dos ZIPs |
| `POSTGRES_DSN` | vazio | DSN completo opcional |
| `POSTGRES_HOST` | `localhost` | Host do PostgreSQL |
| `POSTGRES_PORT` | `5432` | Porta do PostgreSQL |
| `POSTGRES_USER` | `video_processor` | Usuario do PostgreSQL |
| `POSTGRES_PASSWORD` | vazio | Senha do PostgreSQL |
| `POSTGRES_DB` | `video_processor` | Banco do PostgreSQL |
| `RABBITMQ_HOST` | `localhost` | Host do RabbitMQ |
| `RABBITMQ_PORT` | `5672` | Porta do RabbitMQ |
| `RABBITMQ_USER` | vazio | Usuario do RabbitMQ |
| `RABBITMQ_PASSWORD` | vazio | Senha do RabbitMQ |
| `RABBITMQ_QUEUE` | `video_jobs` | Fila consumida pelo Worker |
| `RABBITMQ_DLX` | `video_jobs.dlx` | Exchange de dead-letter |
| `RABBITMQ_CONFIRM_TIMEOUT` | `5s` | Timeout de publisher confirms |
| `WORKER_CONCURRENCY` | `2` | Numero maximo de videos processados simultaneamente |
| `WORKER_CPU_LIMIT` | numero de CPUs | Limite de CPUs logico do worker |
| `WORKER_MAX_MEMORY_MB` | `0` | Limite de memoria do runtime; `0` desativa |
| `WORKER_MEMORY_PER_JOB_MB` | `512` | Memoria estimada por processamento para limitar concorrencia |
| `WORKER_PREFETCH` | valor de `WORKER_CONCURRENCY` | Mensagens pre-buscadas pelo RabbitMQ |
| `WORKER_MAX_ATTEMPTS` | `3` | Tentativas totais antes da DLQ |
| `WORKER_RETRY_BASE_DELAY` | `5s` | Backoff inicial; dobra a cada tentativa |
| `MONGO_URI` | `mongodb://localhost:27017` | Conexao dos logs |
| `MONGO_DATABASE` | `video_processor_logs` | Banco dos logs |
| `MONGO_COLLECTION` | `application_logs` | Collection dos logs |
| `SMTP_HOST` | `localhost` | Host do servidor SMTP |
| `SMTP_PORT` | `25` | Porta do servidor SMTP |
| `SMTP_USERNAME` | vazio | Usuario SMTP |
| `SMTP_PASSWORD` | vazio | Senha SMTP |
| `SMTP_FROM` | `noreply@video-processor.local` | Remetente das notificacoes |
| `SMTP_TO` | vazio | Destinatario fallback quando o job nao tiver e-mail |

Com Mailpit, use `SMTP_HOST=mailpit`, `SMTP_PORT=1025`, `SMTP_FROM=noreply@example.com`
e deixe `SMTP_USERNAME` e `SMTP_PASSWORD` vazios. O e-mail recebido pode ser consultado
em `http://localhost:8025`.

## Execucao local

O Worker precisa acessar os mesmos servicos da API e o mesmo storage compartilhado:

```bash
go mod tidy
go run .
```

Ele ficara aguardando mensagens no RabbitMQ na fila `video_jobs`.

Falhas de processamento sao reenfileiradas em filas TTL com backoff exponencial.
Depois do limite de tentativas, a mensagem vai para `video_jobs.dead`, o job recebe
status `Erro` e o usuario e notificado por e-mail.

## Docker

O Dockerfile instala o `ffmpeg` na imagem:

```bash
docker build -t video-processor-worker .
docker run --name video-worker video-processor-worker
````
### Rodando com o Compose da API:

Primeiro, suba a API e as dependencias no repositorio `video-processor-api`:

```bash
docker compose up -d --build
```

Depois, neste repositorio, suba somente o worker:

```bash
docker compose up -d --build
```

O Compose da API cria a rede externa `video-processor` e o volume externo
`video-processor-data`. O Compose do Worker apenas se conecta a esses recursos;
os repositorios continuam independentes.

### Rodando o container manualmente:

```bash
docker run --name video-worker \
	--env-file .env \
	--network video-processor \
	--mount source=video-processor-data,target=/data \
	-e WORKER_STORAGE_DIR=/data/uploads \
	-e WORKER_OUTPUT_DIR=/data/outputs \
	-e POSTGRES_HOST=postgres \
	-e POSTGRES_PORT=5432 \
	-e POSTGRES_USER=video_processor \
	-e POSTGRES_DB=video_processor \
	-e RABBITMQ_HOST=rabbitmq \
	-e RABBITMQ_PORT=5672 \
	-e MONGO_URI=mongodb://mongo:27017 \
	video-processor-worker
```

Em um ambiente com containers, use a mesma rede da API, RabbitMQ e PostgreSQL. O diretorio de entrada e saida precisa ser compartilhado com a API, por volume Docker ou storage de objetos.

## Kubernetes

Os manifests e as instrucoes de deploy do worker estao em
[k8s/README.md](k8s/README.md). O worker usa a imagem publicada
`nuriancoelho/video-processor-worker:latest`.

## CI/CD

`.github/workflows/ci.yml` executa testes, compilacao e build da imagem. Pushes para `main` tambem publicam a imagem no Docker Hub com `DOCKERHUB_USERNAME` e `DOCKERHUB_TOKEN`.

## Pendencia de integracao

Os arquivos de logging presentes no Worker usam o cliente MongoDB, mas o entrypoint precisa chamar o construtor correspondente (`NewMongoLogger`) e manter as dependencias do `go.mod` alinhadas. Resolva essa nomenclatura antes do build final.
