package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type DataFile struct {
	Name    string
	Content string
}

type Event struct {
	Files []DataFile
}

type Producer struct {
	batchSize  int
	batch      []DataFile
	eventQueue chan Event
	inputDir   string
	outputDir  string
}

func NewProducer(batchSize int, inputDir, outputDir string, eventQueue chan Event) *Producer {
	return &Producer{
		batchSize:  batchSize,
		batch:      make([]DataFile, 0, batchSize),
		eventQueue: eventQueue,
		inputDir:   inputDir,
		outputDir:  outputDir,
	}
}

func (p *Producer) AddFile(file DataFile) {
	p.batch = append(p.batch, file)
	if len(p.batch) >= p.batchSize {
		p.sendBatch()
	}
}

func (p *Producer) sendBatch() {
	if len(p.batch) > 0 {
		// Update the .in file before sending the batch
		inFileName := p.updateInFile()
		if inFileName != "" {
			p.batch = append(p.batch, DataFile{Name: inFileName, Content: ""})
		}

		event := Event{Files: p.batch}
		p.eventQueue <- event
		p.moveProcessedFiles()
		p.batch = make([]DataFile, 0, p.batchSize)
	}
}

func (p *Producer) moveProcessedFiles() {
	for _, file := range p.batch {
		sourcePath := filepath.Join(p.inputDir, file.Name)
		destPath := filepath.Join(p.outputDir, file.Name)
		err := MoveFile(sourcePath, destPath)
		if err != nil {
			fmt.Printf("Error moving file %s: %v\n", file.Name, err)
		}
	}
}

func (p *Producer) ReadFiles() {
	files, err := os.ReadDir(p.inputDir)
	failOnError(err, "Failed reading input directory")
	for _, file := range files {
		if !file.IsDir() {
			content, err := os.ReadFile(filepath.Join(p.inputDir, file.Name()))
			if err != nil {
				log.Printf("Error reading file %s: %v\n", file.Name(), err)
				continue
			}
			p.AddFile(DataFile{Name: file.Name(), Content: string(content)})
		}
	}
}

func (p *Producer) updateInFile() string {
	println("updating .in file")
	templateInFilePath := "/docker/starlight/config_files_starlight/grid_example.in"
	inFileOutputPath := "/starlight/runtime/infiles/"
	newInFileName := fmt.Sprintf("grid_example_%d.in", rand.Int())

	// Check if the template .in file exists
	if exists, _ := exists(templateInFilePath); !exists {
		println("Error: file does not exist")
		return ""
	}

	f, err := os.Open(templateInFilePath)
	defer f.Close()
	if err != nil {
		println("Error opening file")
		panic(err)
	}

	scanner := bufio.NewScanner(f)
	i := 0
	var newFile string
	for scanner.Scan() {
		i++
		if i == 16 {
			// Replace the input file name in the .in file
			res := strings.Split(scanner.Text(), "  ")
			res[0] = p.batch[0].Name // Use the first file in the batch as the input file
			res[5] = "output_" + p.batch[0].Name
			overwrite_string := strings.Join(res, "  ")
			newFile = newFile + overwrite_string + "\n"
		} else {
			newFile = newFile + scanner.Text() + "\n"
		}
	}

	// Write the updated .in file to the output directory
	log.Printf("Writing updated .in file to %s", inFileOutputPath+newInFileName)
	err = os.WriteFile(inFileOutputPath+newInFileName, []byte(newFile), 0644)
	if err != nil {
		println("Error writing .in file: ", err.Error())
		return ""
	}

	return newInFileName
}

func mainRun() {
	eventQueue := make(chan Event, 10)

	inputDir := os.Getenv("INPUT_DIR")
	outputDir := os.Getenv("OUTPUT_DIR")

	if inputDir == "" || outputDir == "" {
		log.Fatal("INPUT_DIR  and OUTPUT_DIR environment variables is required")
	}

	batchSize, err := strconv.Atoi(os.Getenv("BATCH_SIZE"))
	if err != nil {
		log.Fatal("Failed to convert BATCH_SIZE to an integer")
	}
	producer := NewProducer(batchSize, inputDir, outputDir, eventQueue)

	go func() {
		for event := range eventQueue {
			fmt.Printf("Sent event with %d files\n", len(event.Files))
			send(event)
		}
	}()

	for {
		producer.ReadFiles()
		producer.sendBatch()
		time.Sleep(10 * time.Second)
	}
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Panicf("%s: %s", msg, err.Error())
	}
}

func MoveFile(source, destination string) error {
	return os.Rename(source, destination)
}

func send(event Event) {
	username := os.Getenv("RABBITMQ_USER")
	password := os.Getenv("RABBITMQ_PASSWORD")
	host := os.Getenv("RABBITMQ_HOST")
	port := os.Getenv("RABBITMQ_PORT")
	url := fmt.Sprintf("amqp://%s:%s@%s:%s/", username, password, host, port)

	conn, err := amqp.Dial(url)
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"starlight", // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)
	failOnError(err, "Failed to declare a queue")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, f := range event.Files {
		body := f.Content
		headers := make(amqp.Table)
		headers["filename"] = f.Name

		err = ch.PublishWithContext(ctx,
			"",     // exchange
			q.Name, // routing key
			false,  // mandatory
			false,  // immediate
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(body),
				Headers:     headers,
			})
		failOnError(err, "Failed to publish a message")
		log.Printf(" [x] Sent %s\n", body[0:10])
	}
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
