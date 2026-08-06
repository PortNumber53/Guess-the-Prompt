package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	QueuePrompt = "scaffold.prompt"
	QueueImage  = "scaffold.image"
)

// PromptTask is published by `scaffold` and consumed by `worker --queue=prompt`.
type PromptTask struct {
	Count       int    `json:"count"`
	UsePresets  bool   `json:"usePresets"`
	OllamaModel string `json:"ollamaModel"`
	OllamaURL   string `json:"ollamaURL"`
	ObjectsDir  string `json:"objectsDir"`
}

// ImageTask is published by the prompt worker and consumed by `worker --queue=image`.
type ImageTask struct {
	PuzzleID       int    `json:"puzzleId"`
	Prompt         string `json:"prompt"`
	ObjectsDir     string `json:"objectsDir"`
	ObjectsBaseURL string `json:"objectsBaseURL"`
}

// Client wraps an AMQP connection and channel.
type Client struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Connect dials RabbitMQ and opens a channel.
func Connect(url string) (*Client, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	// Declare both queues so producers and consumers are idempotent
	for _, name := range []string{QueuePrompt, QueueImage} {
		_, err := ch.QueueDeclare(name, true, false, false, false, nil)
		if err != nil {
			ch.Close()
			conn.Close()
			return nil, fmt.Errorf("declare queue %s: %w", name, err)
		}
	}
	// Fair dispatch — one message at a time per worker
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq qos: %w", err)
	}
	return &Client{conn: conn, ch: ch}, nil
}

// Close tears down the channel and connection.
func (c *Client) Close() {
	if c.ch != nil {
		c.ch.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// Publish serialises payload as JSON and sends it to the named queue.
func (c *Client) Publish(ctx context.Context, queueName string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.ch.PublishWithContext(ctx, "", queueName, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         body,
	})
}

// Consume returns a delivery channel for the named queue. The caller must
// Ack or Nack each delivery.
func (c *Client) Consume(queueName, consumerTag string) (<-chan amqp.Delivery, error) {
	return c.ch.Consume(queueName, consumerTag, false, false, false, false, nil)
}

// PublishImageTasks is a convenience helper that publishes one ImageTask per puzzle.
func (c *Client) PublishImageTasks(ctx context.Context, tasks []ImageTask) error {
	for _, t := range tasks {
		if err := c.Publish(ctx, QueueImage, t); err != nil {
			return fmt.Errorf("publish image task for puzzle %d: %w", t.PuzzleID, err)
		}
		log.Printf("  Queued image task for puzzle #%d", t.PuzzleID)
	}
	return nil
}
