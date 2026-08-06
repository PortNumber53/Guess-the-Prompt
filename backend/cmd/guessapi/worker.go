package guesscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"guessapi/internal/config"
	"guessapi/internal/db"
	"guessapi/internal/queue"

	"github.com/spf13/cobra"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Process scaffold tasks from RabbitMQ queues",
	Long: `Starts a long-running worker that consumes from a RabbitMQ queue.

Examples:
  go run . worker --queue=prompt     # process prompt generation tasks
  go run . worker --queue=image      # process image generation tasks`,
	Run: func(cmd *cobra.Command, args []string) {
		queueFlag, _ := cmd.Flags().GetString("queue")

		var queueName string
		switch queueFlag {
		case "prompt":
			queueName = queue.QueuePrompt
		case "image":
			queueName = queue.QueueImage
		default:
			log.Fatalf("Unknown queue %q — use 'prompt' or 'image'", queueFlag)
		}

		rabbitURL := config.AppConfig.RabbitMQURL
		if rabbitURL == "" {
			log.Fatal("RABBITMQ_URL is not configured")
		}

		dbURL := config.AppConfig.DatabaseURL
		if dbURL == "" {
			dbURL = "postgres://postgres:postgres@localhost:5432/guessdb?sslmode=disable"
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		database, err := db.Connect(ctx, dbURL)
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer database.Pool.Close()

		mqClient, err := queue.Connect(rabbitURL)
		if err != nil {
			log.Fatalf("Failed to connect to RabbitMQ: %v", err)
		}
		defer mqClient.Close()

		deliveries, err := mqClient.Consume(queueName, fmt.Sprintf("worker-%s-%d", queueFlag, os.Getpid()))
		if err != nil {
			log.Fatalf("Failed to start consuming from %s: %v", queueName, err)
		}

		// Graceful shutdown on SIGINT / SIGTERM
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		log.Printf("Worker started — consuming from %s (ctrl-c to stop)", queueName)

		for {
			select {
			case sig := <-sigCh:
				log.Printf("Received %s, shutting down…", sig)
				cancel()
				return

			case d, ok := <-deliveries:
				if !ok {
					log.Println("Delivery channel closed, exiting")
					return
				}

				var processErr error
				switch queueName {
				case queue.QueuePrompt:
					processErr = handlePromptTask(ctx, database, mqClient, d.Body)
				case queue.QueueImage:
					processErr = handleImageTask(ctx, database, d.Body)
				}

				if processErr != nil {
					log.Printf("ERROR processing message: %v", processErr)
					// Nack with requeue so another worker can retry
					d.Nack(false, true)
				} else {
					d.Ack(false)
				}
			}
		}
	},
}

func init() {
	workerCmd.Flags().StringP("queue", "q", "", "Queue to consume: 'prompt' or 'image' (required)")
	workerCmd.MarkFlagRequired("queue")
}

// handlePromptTask generates prompts, inserts puzzles into DB, and publishes
// image tasks for each new puzzle.
func handlePromptTask(ctx context.Context, database *db.Database, mqClient *queue.Client, body []byte) error {
	var task queue.PromptTask
	if err := json.Unmarshal(body, &task); err != nil {
		return fmt.Errorf("unmarshal prompt task: %w", err)
	}

	log.Printf("Processing prompt task: count=%d presets=%v model=%s", task.Count, task.UsePresets, task.OllamaModel)

	// Generate puzzle templates (reuse existing logic)
	var puzzles []PuzzleTemplate
	var err error
	if task.UsePresets {
		puzzles = generatePuzzleTemplates(task.Count)
	} else {
		puzzles, err = generateOllamaPuzzleTemplates(ctx, task.Count, task.OllamaModel, task.OllamaURL)
		if err != nil {
			log.Printf("WARNING: Ollama failed, falling back to presets: %v", err)
			puzzles = generatePuzzleTemplates(task.Count)
		}
	}

	// Insert puzzles into DB and collect image tasks
	var imageTasks []queue.ImageTask
	created := 0
	for _, p := range puzzles {
		var puzzleID int
		err := database.Pool.QueryRow(ctx, `
			INSERT INTO puzzles (prompt, prize_pool, image_url, status, tags)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id
		`, p.Prompt, p.Prize, "/ai-generated-image.png", p.Status, p.Tags).Scan(&puzzleID)
		if err != nil {
			log.Printf("WARNING: Failed to insert puzzle: %v", err)
			continue
		}
		created++

		imageTasks = append(imageTasks, queue.ImageTask{
			PuzzleID:       puzzleID,
			Prompt:         p.Prompt,
			ObjectsDir:     task.ObjectsDir,
			ObjectsBaseURL: "/objects/",
		})
	}
	log.Printf("Inserted %d/%d puzzles", created, task.Count)

	// Publish image generation tasks
	if err := mqClient.PublishImageTasks(ctx, imageTasks); err != nil {
		return fmt.Errorf("publish image tasks: %w", err)
	}
	log.Printf("Published %d image tasks to queue", len(imageTasks))

	return nil
}

// handleImageTask generates an AI image for a single puzzle and updates the DB.
func handleImageTask(ctx context.Context, database *db.Database, body []byte) error {
	var task queue.ImageTask
	if err := json.Unmarshal(body, &task); err != nil {
		return fmt.Errorf("unmarshal image task: %w", err)
	}

	log.Printf("Processing image task: puzzle #%d", task.PuzzleID)

	if !isMfluxAvailable() {
		return fmt.Errorf("mflux-generate-z-image-turbo not found — install with: uv tool install mflux")
	}

	objectsDir := task.ObjectsDir
	if objectsDir == "" {
		objectsDir = "./objects"
	}

	imgURL, err := generateAndSaveImage(task.Prompt, objectsDir, task.ObjectsBaseURL)
	if err != nil {
		return fmt.Errorf("generate image for puzzle #%d: %w", task.PuzzleID, err)
	}

	_, err = database.Pool.Exec(ctx, `UPDATE puzzles SET image_url = $1 WHERE id = $2`, imgURL, task.PuzzleID)
	if err != nil {
		return fmt.Errorf("update puzzle #%d image: %w", task.PuzzleID, err)
	}

	log.Printf("Puzzle #%d image saved: %s", task.PuzzleID, imgURL)
	return nil
}
