package main

import (
	"fmt"
)

func main() {
	cfg, err := loadConfig("demo-config.json")
	if err != nil {
		fmt.Println("invalid config:", err)
		return
	}

	worker := Worker{Config: cfg}

	if err := worker.Validate(); err != nil {
		fmt.Println("invalid worker:", err)
		return
	}

	item, err := loadWorkItem("demo-item.json")
	if err != nil {
		fmt.Println("invalid work item:", err)
		return
	}

	if err := worker.Run(item); err != nil {
		fmt.Println("worker failed:", err)
		return
	}

}
