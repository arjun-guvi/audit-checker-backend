package worker

import (
	"fmt"

	"github.com/gocraft/work"
)

type context struct{}

func HelloWorld(job *work.Job) error {
	fmt.Println("Hello World")
	return nil
}
