package main

import "log"

func main() {
	envs, err := loadConfig("generate/Config.toml")
	if err != nil {
		log.Fatal(err)
	}
	if err := generateCode("generated.go", envs); err != nil {
		log.Fatal(err)
	}
	if err := generateDocs("../docs/config.md", envs); err != nil {
		log.Fatal(err)
	}
}
