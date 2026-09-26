# Video Processor Worker

Worker responsável pelo processamento assíncrono de vídeos. Ele não expõe uma API HTTP pública.

O Worker consome jobs publicados pela API no RabbitMQ, processa os vídeos utilizando FFmpeg, armazena os arquivos no storage S3 compatível e atualiza o status do processamento no PostgreSQL.

## Responsabilidades

1. Consumir mensagens da fila `video_jobs` no RabbitMQ.
2. Localizar o vídeo no storage S3/SeaweedFS.
3. Executar o `ffmpeg` para extrair um frame por segundo.
4. Compactar os frames em um arquivo ZIP.
5. Enviar o ZIP processado para o storage S3/SeaweedFS.
6. Atualizar o job no PostgreSQL para `Concluido` ou `Erro`.
7. Enviar um e-mail ao usuário quando o processamento falhar.
8. Registrar eventos de processamento no MongoDB.

## Fluxo de processamento

```text
RabbitMQ
   │
   │ video_jobs
   ▼
┌─────────────────────┐
│  Video Processor    │
│      Worker         │
└──────────┬──────────┘
           │
           ├──► S3/SeaweedFS
           │      └── videos/<job_id>_<filename>
           │
           ├──► FFmpeg
           │      └── 1 frame por segundo
           │
           ├──► ZIP
           │      └── frames_<job_id>.zip
           │
           └──► S3/SeaweedFS
                  └── outputs/frames_<job_id>.zip

           ├──► PostgreSQL
           │      └── atualiza status do job
           │
           └──► MongoDB
                  └── registra eventos
```

## Storage

O Worker utiliza um storage compatível com a API S3. No ambiente local, o storage é fornecido pelo **SeaweedFS**.

Os objetos seguem a seguinte estrutura:

```text
videos/
└── <job_id>_<nome_original>.mp4

outputs/
└── frames_<job_id>.zip
```

O vídeo original é obtido utilizando o `object_key` recebido no job.

Após o processamento, o ZIP é enviado para:

```text
outputs/frames_<job_id>.zip
```

O Worker não depende de um volume compartilhado entre os containers para trocar vídeos ou resultados com a API.

## Dependências

- **RabbitMQ:** consumo da fila `video_jobs` e gerenciamento de retry/DLQ.
- **PostgreSQL:** leitura e atualização dos jobs.
- **S3/SeaweedFS:** armazenamento dos vídeos e arquivos processados.
- **MongoDB:** armazenamento dos logs da aplicação.
- **FFmpeg:** extração dos frames.
- **SMTP/Mailpit:** envio e visualização das notificações por e-mail em ambiente local.

## Variáveis de ambiente

| Variável | Padrão | Uso |
|---|---|---|
| `POSTGRES_DSN` | vazio | DSN completo opcional |
| `POSTGRES_HOST` | `localhost` | Host do PostgreSQL |
| `POSTGRES_PORT` | `5432` | Porta do PostgreSQL |
| `POSTGRES_USER` | `video_processor` | Usuário do PostgreSQL |
| `POSTGRES_PASSWORD` | vazio | Senha do PostgreSQL |
| `POSTGRES_DB` | `video_processor` | Banco do PostgreSQL |
| `RABBITMQ_HOST` | `localhost` | Host do RabbitMQ |
| `RABBITMQ_PORT` | `5672` | Porta do RabbitMQ |
| `RABBITMQ_USER` | vazio | Usuário do RabbitMQ |
| `RABBITMQ_PASSWORD` | vazio | Senha do RabbitMQ |
| `RABBITMQ_QUEUE` | `video_jobs` | Fila consumida pelo Worker |
| `RABBITMQ_DLX` | `video_jobs.dlx` | Exchange de dead-letter |
| `RABBITMQ_CONFIRM_TIMEOUT` | `5s` | Timeout de publisher confirms |
| `WORKER_CONCURRENCY` | `2` | Número máximo de vídeos processados simultaneamente |
| `WORKER_CPU_LIMIT` | número de CPUs | Limite lógico de CPUs do Worker |
| `WORKER_MAX_MEMORY_MB` | `0` | Limite de memória do runtime; `0` desativa |
| `WORKER_MEMORY_PER_JOB_MB` | `512` | Memória estimada por processamento |
| `WORKER_PREFETCH` | valor de `WORKER_CONCURRENCY` | Mensagens pré-buscadas pelo RabbitMQ |
| `WORKER_MAX_ATTEMPTS` | `3` | Número total de tentativas antes da DLQ |
| `WORKER_RETRY_BASE_DELAY` | `5s` | Backoff inicial; dobra a cada tentativa |
| `S3_ENDPOINT` | `http://localhost:8333` | Endpoint do storage S3 |
| `S3_BUCKET` | `video-processor` | Bucket utilizado pelo Worker |
| `S3_ACCESS_KEY` | `minio` | Access key do storage |
| `S3_SECRET_KEY` | `minio123` | Secret key do storage |
| `S3_REGION` | `us-east-1` | Região utilizada para assinatura S3 |
| `MONGO_URI` | `mongodb://localhost:27017` | Conexão com MongoDB |
| `MONGO_DATABASE` | `video_processor_logs` | Banco dos logs |
| `MONGO_COLLECTION` | `application_logs` | Collection dos logs |
| `SMTP_HOST` | `localhost` | Host do servidor SMTP |
| `SMTP_PORT` | `25` | Porta do servidor SMTP |
| `SMTP_USERNAME` | vazio | Usuário SMTP |
| `SMTP_PASSWORD` | vazio | Senha SMTP |
| `SMTP_FROM` | `noreply@video-processor.local` | Remetente das notificações |
| `SMTP_TO` | vazio | Destinatário fallback quando o job não tiver e-mail |

### Mailpit

No ambiente local, utilizando Mailpit:

```env
SMTP_HOST=mailpit
SMTP_PORT=1025
SMTP_FROM=noreply@example.com
```

`SMTP_USERNAME` e `SMTP_PASSWORD` devem permanecer vazios.

Os e-mails recebidos podem ser consultados em:

```text
http://localhost:8025
```

### SeaweedFS

No ambiente Docker local:

```env
S3_ENDPOINT=http://s3:8333
S3_BUCKET=video-processor
S3_ACCESS_KEY=minio
S3_SECRET_KEY=minio123
S3_REGION=us-east-1
```

As credenciais acima são utilizadas apenas pelo ambiente local do SeaweedFS.

## Execução local

O Worker precisa conseguir acessar o RabbitMQ, PostgreSQL, MongoDB, SMTP e o storage S3 configurados no ambiente.

Como o projeto utiliza Docker para o ambiente de execução, as dependências podem ser disponibilizadas pelos containers da infraestrutura.

Caso o Go esteja disponível localmente:

```bash
go mod tidy
go run .
```

O Worker ficará aguardando mensagens na fila:

```text
video_jobs
```

## Retry e Dead Letter Queue

Falhas durante o processamento são tratadas utilizando retry com backoff exponencial.

O fluxo é:

```text
video_jobs
    │
    ▼
processamento
    │
    ├── sucesso ──► job Concluido
    │
    └── erro
          │
          ▼
       retry/TTL
          │
          ├── nova tentativa
          │
          └── limite atingido
                  │
                  ▼
             video_jobs.dead
                  │
                  ▼
              job Erro
                  │
                  ▼
            notificação
```

O número máximo de tentativas é configurado por:

```env
WORKER_MAX_ATTEMPTS=3
```

O intervalo inicial do backoff é configurado por:

```env
WORKER_RETRY_BASE_DELAY=5s
```

## Docker

O Dockerfile instala o FFmpeg dentro da imagem do Worker.

Para criar a imagem:

```bash
docker build -t video-processor-worker .
```

Para executar utilizando o ambiente Docker da aplicação, o Worker deve estar conectado à mesma rede Docker dos serviços que ele consome.

### Compose

A API e o Worker são projetos independentes e possuem seus próprios arquivos `docker-compose.yml`.

Primeiro, no repositório da API, devem estar disponíveis os serviços de infraestrutura:

```bash
docker compose up -d --build
```

Depois, neste repositório:

```bash
docker compose up -d --build
```

O Compose do Worker utiliza a rede Docker externa:

```text
video-processor
```

Essa rede permite que o Worker acesse os serviços da infraestrutura pelos nomes dos containers, por exemplo:

```text
postgres
rabbitmq
mongo
mailpit
s3
```

A API e o Worker continuam sendo repositórios independentes.

## Execução manual do container

Exemplo de execução utilizando a rede compartilhada:

```bash
docker run --name video-worker \
    --env-file .env \
    --network video-processor \
    -e POSTGRES_HOST=postgres \
    -e POSTGRES_PORT=5432 \
    -e POSTGRES_USER=video_processor \
    -e POSTGRES_DB=video_processor \
    -e RABBITMQ_HOST=rabbitmq \
    -e RABBITMQ_PORT=5672 \
    -e MONGO_URI=mongodb://mongo:27017 \
    -e S3_ENDPOINT=http://s3:8333 \
    -e S3_BUCKET=video-processor \
    -e S3_ACCESS_KEY=minio \
    -e S3_SECRET_KEY=minio123 \
    -e S3_REGION=us-east-1 \
    video-processor-worker
```

Não é necessário montar um volume compartilhado para os vídeos processados, pois a comunicação com o storage é feita através da API S3.

## Kubernetes

Os manifests e as instruções específicas de deploy do Worker estão disponíveis em:

```text
k8s/README.md
```

A imagem utilizada pelo deployment é:

```text
nuriancoelho/video-processor-worker:latest
```

As configurações de RabbitMQ, PostgreSQL, MongoDB, SMTP e S3 devem ser fornecidas através dos mecanismos de configuração do ambiente Kubernetes.

## CI/CD

O workflow:

```text
.github/workflows/ci.yml
```

é responsável por executar as etapas de CI do Worker, incluindo testes, compilação e build da imagem Docker.

Pushes para `main` também podem publicar a imagem no Docker Hub utilizando:

```text
DOCKERHUB_USERNAME
DOCKERHUB_TOKEN
```

## Estrutura do projeto

```text
video-processor-worker/
├── main.go
├── storage.go
├── repository.go
├── queue.go
├── logger.go
├── mailer.go
├── go.mod
├── go.sum
├── Dockerfile
├── docker-compose.yml
├── .env
├── k8s/
└── .github/
    └── workflows/
        └── ci.yml
```

Os nomes dos arquivos podem variar conforme a organização atual do projeto; a responsabilidade principal do Worker permanece concentrada no consumo da fila, processamento do vídeo, persistência dos resultados e atualização do job.