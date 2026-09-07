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
	"time"

)

type VideoJob struct {
	ID        string    `json:"id"`
	User      string    `json:"user"`
	ObjectKey string    `json:"object_key"`
	Status    string    `json:"status"`
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
	queue, err := NewRabbitConsumer(rabbitConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer queue.Close()

	messages, err := queue.Consume()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Worker iniciado; aguardando tarefas no RabbitMQ")
	for message := range messages {
		var job VideoJob
		if err := json.Unmarshal(message.Body, &job); err != nil {
			logger.Log("job_invalid", "", err.Error())
			message.Nack(false, false)
			continue
		}

		if err := processJob(job, envOr("WORKER_STORAGE_DIR", "uploads"), envOr("WORKER_OUTPUT_DIR", "outputs")); err != nil {
			job.Status = "Erro"
			logger.Log("job_failed", job.ID, err.Error())
			if updateErr := repository.UpdateStatus(job.ID, job.Status); updateErr != nil {
				logger.Log("job_database_error", job.ID, updateErr.Error())
			}
			message.Ack(false)
			continue
		}

		job.Status = "Concluido"
		if err := repository.UpdateStatus(job.ID, job.Status); err != nil {
			logger.Log("job_database_error", job.ID, err.Error())
			message.Nack(false, true)
			continue
		}
		logger.Log("job_completed", job.ID, "video processing completed")
		message.Ack(false)
	}
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

