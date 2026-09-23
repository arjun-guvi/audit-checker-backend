package worker

import (
	"fmt"

	"github.com/gocraft/work"
)

type jobContext struct{}

func HelloWorld(job *work.Job) error {
	fmt.Println("Hello World")
	return nil
}
