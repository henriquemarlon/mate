//go:generate go run .

package main

func main() {
	data := readTOML("Config.toml")
	config := decodeTOML(data)
	envs := sortConfig(config)
	for _, env := range envs {
		env.validate()
	}
	generateDocsFile("../../docs/config.md", envs)
	generateCodeFile("../generated.go", envs)
}
