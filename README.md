# Video Processor Worker

Worker responsavel pelo processamento assincrono de videos. Ele nao expoe uma API HTTP publica.

## Responsabilidades

1. Consumir mensagens da fila `video_jobs` no RabbitMQ.
2. Localizar o video recebido pela API.
3. Executar o `ffmpeg` com um frame por segundo.
4. Compactar os frames em um arquivo ZIP.
5. Atualizar o job no PostgreSQL para `Concluido` ou `Erro`.
6. Registrar eventos de processamento no sistema de logs.

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
| `POSTGRES_PASSWORD` | `video_processor` | Senha do PostgreSQL |
| `POSTGRES_DB` | `video_processor` | Banco do PostgreSQL |
| `RABBITMQ_HOST` | `localhost` | Host do RabbitMQ |
| `RABBITMQ_PORT` | `5672` | Porta do RabbitMQ |
| `RABBITMQ_USER` | `video_processor` | Usuario do RabbitMQ |
| `RABBITMQ_PASSWORD` | `video_processor` | Senha do RabbitMQ |
| `RABBITMQ_QUEUE` | `video_jobs` | Fila consumida pelo Worker |
| `MONGO_URI` | `mongodb://localhost:27017` | Conexao dos logs |
| `MONGO_DATABASE` | `video_processor_logs` | Banco dos logs |
| `MONGO_COLLECTION` | `application_logs` | Collection dos logs |

## Execucao local

O Worker precisa acessar os mesmos servicos da API e o mesmo storage compartilhado:

```bash
go mod tidy
go run .
```

Ele ficara aguardando mensagens no RabbitMQ na fila `video_jobs`.

## Docker

O Dockerfile instala o `ffmpeg` na imagem:

```bash
docker build -t video-processor-worker .
docker run --name video-worker video-processor-worker
```

Em um ambiente com containers, use a mesma rede da API, RabbitMQ e PostgreSQL. O diretorio de entrada e saida precisa ser compartilhado com a API, por volume Docker ou storage de objetos.

## CI/CD

`.github/workflows/ci.yml` executa testes, compilacao e build da imagem. Pushes para `main` tambem publicam a imagem no Docker Hub com `DOCKERHUB_USERNAME` e `DOCKERHUB_TOKEN`.

## Pendencia de integracao

Os arquivos de logging presentes no Worker usam o cliente MongoDB, mas o entrypoint precisa chamar o construtor correspondente (`NewMongoLogger`) e manter as dependencias do `go.mod` alinhadas. Resolva essa nomenclatura antes do build final.
