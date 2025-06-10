package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// Реализовать примитивный worker-pool с возможностью динамически добавлять и удалять воркеры.
// Входные данные (строки) поступают в канал, воркеры их обрабатывают
// (например, выводят на экран номер воркера и сами данные).
// Задание на базовые знания каналов и горутин

var messages = []string{
	"От улыбки станет день светлей,",
	"От улыбки в небе радуга проснётся.",
	"Поделись улыбкою своей,",
	"И она к тебе не раз ещё вернётся.",

	"И тогда наверняка",
	"Вдруг запляшут облака,",
	"И кузнечик запиликает на скрипке.",
	"С голубого ручейка",
	"Начинается река,",
	"Ну а дружба начинается с улыбки.",

	"С голубого ручейка",
	"Начинается река,",
	"Ну а дружба начинается с улыбки.",

	"От улыбки станет всем теплей —",
	"И слону, и даже маленькой улитке.",
	"Так пускай повсюду на земле",
	"Будто лампочки включаются улыбки.",

	"И тогда наверняка",
	"Вдруг запляшут облака,",
	"И кузнечик запиликает на скрипке.",
	"С голубого ручейка",
	"Начинается река,",
	"Ну а дружба начинается с улыбки.",

	"С голубого ручейка",
	"Начинается река,",
	"Ну а дружба начинается с улыбки.",
}

var (
	workerNotExist     = errors.New("worker doesn't exist")
	noWorkersToRemove  = errors.New("no workersID to remove")
	incorrectID        = errors.New("ID must be int")
	nonexistentCommand = errors.New("non-existent command")
	workerExist        = errors.New("worker with this id already exists")

	cntWorker = 10
)

const (
	EXIT       = "exit"
	HELP       = "help"
	ADD        = "add"
	REMOVE     = "remove"
	REMOVEBYID = "removebyid"
	LIST       = "list"
)

type WorkerPool struct {
	mu        sync.RWMutex
	workersID []int
	workers   map[int]*Worker
	ch        chan string
	done      chan struct{}
	closeOnce sync.Once
}

type Worker struct {
	ID     int
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWorkerPool(cntWorkers int) *WorkerPool {
	ch := make(chan string)
	workers := make([]int, 0, cntWorkers)
	workersMap := make(map[int]*Worker, cntWorkers)
	done := make(chan struct{})
	return &WorkerPool{
		workersID: workers,
		ch:        ch,
		workers:   workersMap,
		done:      done,
	}
}

func NewWorker(parentCtx context.Context, ID int) *Worker {
	ctx, cancel := context.WithCancel(parentCtx)
	return &Worker{ID: ID, ctx: ctx, cancel: cancel}
}

func (wp *WorkerPool) AddWorker(worker *Worker) error {
	const op = "AddWorker"

	wp.mu.Lock()
	defer wp.mu.Unlock()
	if _, ok := wp.workers[worker.ID]; ok {
		return fmt.Errorf("%s: %w", op, workerExist)
	}
	wp.workersID = append(wp.workersID, worker.ID)
	wp.workers[worker.ID] = worker
	return nil
}

func (wp *WorkerPool) RemoveWorker() error {
	const op = "RemoveWorker"
	wp.mu.Lock()
	defer wp.mu.Unlock()
	if len(wp.workersID) == 0 {
		return fmt.Errorf("%s: %w", op, noWorkersToRemove)
	}

	ID := wp.workersID[len(wp.workersID)-1]
	worker := wp.workers[ID]
	wp.workersID = wp.workersID[:len(wp.workersID)-1]
	delete(wp.workers, ID)
	worker.cancel()

	return nil
}

func (wp *WorkerPool) RemoveWorkerByID(ID int) error {
	const op = "RemoveWorkerByID"

	wp.mu.RLock()
	if len(wp.workersID) == 0 {
		return fmt.Errorf("%s: %w", op, noWorkersToRemove)
	}
	wp.mu.RUnlock()

	numb, err := wp.GetNumbWorker(ID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	wp.mu.Lock()
	defer wp.mu.Unlock()
	worker := wp.workers[ID]
	delete(wp.workers, ID)

	if numb == len(wp.workersID)-1 {
		wp.workersID = wp.workersID[:numb]
	} else {
		wp.workersID = append(wp.workersID[:numb], wp.workersID[numb+1:]...)
	}

	worker.cancel()

	return nil
}

func (wp *WorkerPool) GetNumbWorker(ID int) (int, error) {
	const op = "GetNumbWorker"
	workersID := wp.SnapShotWorkersID()

	for i, w := range workersID {
		if w == ID {
			return i, nil
		}
	}

	return -1, fmt.Errorf("%s %d: %w", op, ID, workerNotExist)
}

func (wp *WorkerPool) SnapShotWorkersID() []int {
	wp.mu.RLock()
	workers := make([]int, len(wp.workersID))
	copy(workers, wp.workersID)
	wp.mu.RUnlock()
	return workers
}

func (wp *WorkerPool) SendMessage(messages []string) {
	for _, msg := range messages {
		select {
		case <-wp.done:
			return
		case wp.ch <- msg:
		}
		durInt := rand.Intn(10) + 1
		time.Sleep(time.Duration(durInt) * time.Second)
	}
}

func (wp *WorkerPool) GraceFullShutDown() {
	wp.mu.Lock()
	for _, worker := range wp.workers {
		worker.cancel()
	}
	defer wp.mu.Unlock()

	wp.closeOnce.Do(func() {
		close(wp.done)
		close(wp.ch)
	})
}

func (wp *WorkerPool) List() {
	workersID := wp.SnapShotWorkersID()

	log.Println("Workers:")
	for _, ID := range workersID {
		log.Printf(" - Worker %d\n", ID)
	}
}

func (w *Worker) Run(ch <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-w.ctx.Done():
			log.Printf("\t\tWorker %d stopped\n", w.ID)
			return
		case msg, ok := <-ch:
			time.Sleep(500 * time.Millisecond)
			if !ok {
				log.Printf("\t\tChan closed, Worker %d stopped\n", w.ID)
				return
			}
			log.Printf("\t\tWorker %d\n\t message: %s\n", w.ID, msg)
		}
	}
}

func main() {

	workerPool := NewWorkerPool(cntWorker)
	ctx := context.Background()
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		HandleCommand(ctx, workerPool, &wg)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		workerPool.SendMessage(messages)
	}()

	wg.Wait()
}

func HandleCommand(ctx context.Context, wp *WorkerPool, wg *sync.WaitGroup) {
	const op = "handleCommand"

	var cmd string
	PrintMenu()

	for {
		_, err := fmt.Scanln(&cmd)
		if err != nil {
			log.Printf("error happened %s: %v", op, err)
		}
		cmd = strings.ToLower(cmd)
		switch cmd {
		case EXIT:
			log.Println("Команда EXIT")
			wp.GraceFullShutDown()
			return
		case HELP:
			log.Println("Команда HELP")
			PrintMenu()
			continue
		case "":
			continue
		case ADD:
			log.Println("Команда ADD")

			ID := ScanID()

			worker := NewWorker(ctx, ID)
			if err := wp.AddWorker(worker); err != nil {
				log.Printf("%s: %v", op, err)
			}
			wg.Add(1)
			go worker.Run(wp.ch, wg)

		case REMOVE:
			log.Println("Команда REMOVE")
			go func() {
				if err := wp.RemoveWorker(); err != nil {
					log.Printf("error happened: %v", err)
					return
				}
			}()
		case REMOVEBYID:
			log.Println("Команда REMOVEBYID")
			ID := ScanID()
			go func(ID int) {
				if err := wp.RemoveWorkerByID(ID); err != nil {
					log.Printf("error happened: %v", err)
					return
				}
			}(ID)
		case LIST:
			log.Println("Команда LIST")
			wp.List()
		default:
			log.Printf("error happened: %v", nonexistentCommand)
		}
	}
}

func ScanID() int {
	var ID int
	log.Print("Введите ID: ")
	for {
		if _, err := fmt.Scanln(&ID); err != nil {
			log.Printf("Некорректный ввод, введите ID: %v\n", incorrectID)
			continue
		} else {
			return ID
		}
	}
}

func PrintMenu() {
	fmt.Printf(`
add         - добавить воркера
remove      - удалить последнего воркера
removebyid  - удалить воркера по ID
list        - показать список воркеров

exit        - выход
help        - справка

`)
}
