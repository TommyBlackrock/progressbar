package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/TommyBlackrock/progressbar"
)

type demoTask struct {
	name  string
	steps int
	delay time.Duration
}

func main() {
	fmt.Println("progressbar demo")

	bar := progressbar.NewConsoleProgressBar(20)
	tasks := []demoTask{
		{name: "download", steps: 20, delay: 45 * time.Millisecond},
		{name: "compile", steps: 16, delay: 65 * time.Millisecond},
		{name: "package", steps: 12, delay: 85 * time.Millisecond},
	}

	var renderWG sync.WaitGroup
	var workerWG sync.WaitGroup

	for _, task := range tasks {
		bar.Add(task.name)
	}

	for _, task := range tasks {
		task := task
		progress := make(chan int, 1)

		renderWG.Add(1)
		go func() {
			defer renderWG.Done()
			if err := bar.Run(task.name, progress); err != nil {
				fmt.Fprintf(os.Stderr, "progress render error for %s: %v\n", task.name, err)
			}
		}()

		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			defer close(progress)

			for step := 0; step <= task.steps; step++ {
				progress <- step * 100 / task.steps
				time.Sleep(task.delay)
			}
		}()
	}

	workerWG.Wait()
	renderWG.Wait()

	fmt.Println("all demo tasks finished")
}
