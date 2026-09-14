package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type VideoJob struct {
	ID        string    `json:"id"`
	User      string    `json:"user"`
	Email     string    `json:"email,omitempty"`
	ObjectKey string    `json:"object_key"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func main() {
	repository, err := NewPostgresJobRepository(postgresConfig())
	if err != nil {
		log.Fatal(err)
	}
	if err := repository.Init(); err != nil {
		log.Fatal(err)
	}

	logger, err := NewMongoLogger(mongoConfig())
	if err != nil {
		log.Fatal(err)
	}
	notifier := smtpNotifierConfig()
	concurrency := workerConcurrency()
	queue, err := NewRabbitConsumer(rabbitConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer queue.Close()

	messages, err := queue.Consume()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Worker iniciado; aguardando tarefas no RabbitMQ (concorrencia=%d)", concurrency)
	semaphore := make(chan struct{}, concurrency)
	var workers sync.WaitGroup
	for message := range messages {
		semaphore <- struct{}{}
		workers.Add(1)
		go func(message amqp091.Delivery) {
			defer workers.Done()
			defer func() { <-semaphore }()
			handleMessage(message, repository, logger, notifier, queue)
		}(message)
	}
	workers.Wait()
}

func handleMessage(message amqp091.Delivery, repository *PostgresJobRepository, logger Logger, notifier SMTPNotifier, queue *RabbitConsumer) {
		var job VideoJob
		if err := json.Unmarshal(message.Body, &job); err != nil {
			logger.Log("job_invalid", "", err.Error())
			if err := queue.PublishDeadLetter(message); err != nil {
				message.Nack(false, true)
				return
			}
			message.Ack(false)
			return
		}

		if err := processJobSafely(job, envOr("WORKER_STORAGE_DIR", "uploads"), envOr("WORKER_OUTPUT_DIR", "outputs")); err != nil {
			attempt := retryAttempt(message)
			if attempt < queue.config.MaxAttempts-1 {
				if retryErr := queue.PublishRetry(message, attempt+1); retryErr != nil {
					logger.Log("job_retry_error", job.ID, retryErr.Error())
					message.Nack(false, true)
					return
				}
				logger.Log("job_retry_scheduled", job.ID, err.Error())
				message.Ack(false)
				return
			}
			job.Status = "Erro"
			job.Error = err.Error()
			logger.Log("job_failed", job.ID, err.Error())
			if updateErr := repository.UpdateStatus(job.ID, job.Status, job.Error); updateErr != nil {
				logger.Log("job_database_error", job.ID, updateErr.Error())
			}
			if notifyErr := notifier.NotifyProcessingError(notificationRecipient(job), job, err); notifyErr != nil {
				logger.Log("job_notification_error", job.ID, notifyErr.Error())
			}
			if deadLetterErr := queue.PublishDeadLetter(message); deadLetterErr != nil {
				logger.Log("job_dead_letter_error", job.ID, deadLetterErr.Error())
				message.Nack(false, true)
				return
			}
			message.Ack(false)
			return
		}

		job.Status = "Concluido"
		if err := repository.UpdateStatus(job.ID, job.Status, ""); err != nil {
			logger.Log("job_database_error", job.ID, err.Error())
			message.Nack(false, true)
			return
		}
		logger.Log("job_completed", job.ID, "video processing completed")
		message.Ack(false)
	}
}

func retryAttempt(message amqp091.Delivery) int {
	value, ok := message.Headers["x-retry-attempt"]
	if !ok {
		return 0
	}
	switch attempt := value.(type) {
	case int:
		return attempt
	case int32:
		return int(attempt)
	case int64:
		return int(attempt)
	default:
		return 0
	}
}

func workerConcurrency() int {
	cpuLimit := envInt("WORKER_CPU_LIMIT", runtime.NumCPU())
	if cpuLimit > runtime.NumCPU() {
		cpuLimit = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(cpuLimit)
	concurrency := envInt("WORKER_CONCURRENCY", cpuLimit)
	if concurrency > cpuLimit {
		concurrency = cpuLimit
	}
	if memoryLimitMB := envInt("WORKER_MAX_MEMORY_MB", 0); memoryLimitMB > 0 {
		debug.SetMemoryLimit(int64(memoryLimitMB) * 1024 * 1024)
		perJobMB := envInt("WORKER_MEMORY_PER_JOB_MB", 512)
		memoryConcurrency := memoryLimitMB / perJobMB
		if memoryConcurrency < 1 {
			memoryConcurrency = 1
		}
		if concurrency > memoryConcurrency {
			concurrency = memoryConcurrency
		}
	}
	return concurrency
}

func processJobSafely(job VideoJob, storageDir, outputDir string) (processingError error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			processingError = fmt.Errorf("processamento interrompido: %v", recovered)
		}
	}()
	return processJob(job, storageDir, outputDir)
}

func notificationRecipient(job VideoJob) string {
	if strings.TrimSpace(job.Email) != "" {
		return strings.TrimSpace(job.Email)
	}
	if strings.Contains(job.User, "@") {
		return strings.TrimSpace(job.User)
	}
	return strings.TrimSpace(envOr("SMTP_TO", ""))
}

func processJob(job VideoJob, storageDir, outputDir string) error {
	videoPath := filepath.Join(storageDir, filepath.Base(job.ObjectKey))
	tempDir := filepath.Join("temp", job.ID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	frames := filepath.Join(tempDir, "frame_%04d.png")
	output, err := exec.Command("ffmpeg", "-i", videoPath, "-vf", "fps=1", "-y", frames).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, output)
	}
	files, err := filepath.Glob(filepath.Join(tempDir, "*.png"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("nenhum frame extraido")
	}
	return createZip(files, filepath.Join(outputDir, "frames_"+job.ID+".zip"))
}

func createZip(files []string, target string) error {
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	defer archive.Close()
	for _, name := range files {
		input, err := os.Open(name)
		if err != nil {
			return err
		}
		info, err := input.Stat()
		if err != nil {
			input.Close()
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			input.Close()
			return err
		}
		header.Name = filepath.Base(name)
		header.Method = zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err == nil {
			_, err = io.Copy(writer, input)
		}
		input.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

