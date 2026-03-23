package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Shell struct {
	Templates []string `json:"templates"`
}

func main() {
	data, err := os.ReadFile("shelly-rs/shelly.json")
	if err != nil {
		panic(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		panic(err)
	}
	shells := cfg["shells"].(map[string]interface{})
	for name, s := range shells {
		shell := s.(map[string]interface{})
		if templates, ok := shell["templates"].([]interface{}); ok {
			fmt.Printf("Shell: %s\n", name)
			for _, t := range templates {
				fmt.Printf("  %s\n", t.(string))
			}
		}
	}
}
