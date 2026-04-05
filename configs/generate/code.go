package main

import (
	"bytes"
	"go/format"
	"os"
	"sort"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"slices"
)

var funcMap = template.FuncMap{
	"toFieldName": func(env string) string {
		caser := cases.Title(language.English)
		words := strings.FieldsFunc(env, func(r rune) bool {
			return r == '_'
		})
		if len(words) > 1 && words[0] == "MATE" {
			words = words[1:]
		}
		for i, word := range words {
			words[i] = caser.String(strings.ToLower(word))
		}
		return strings.Join(words, "")
	},
	"toConstName": func(s string) string {
		return strings.TrimPrefix(s, "MATE_")
	},
	"defaultVal": func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	},
	"toGoFunc": func(goType string) string {
		return "to" + strings.ToUpper(goType[:1]) + goType[1:]
	},
	"splitLines": func(s string) []string {
		return strings.Split(s, "\n")
	},
	"mapstructure": func(s string) string {
		return "`mapstructure:\"" + s + "\"`"
	},
	"isUsedBy": func(env Env, service string) bool {
		return slices.Contains(env.UsedBy, service)
	},
	"getServices": func(envs []Env) []string {
		serviceMap := make(map[string]bool)
		for _, env := range envs {
			for _, service := range env.UsedBy {
				if service != "cli" {
					serviceMap[service] = true
				}
			}
		}
		services := make([]string, 0, len(serviceMap))
		for service := range serviceMap {
			services = append(services, service)
		}
		sort.Strings(services)
		return services
	},
	"capitalize": func(s string) string {
		if s == "" {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	},
}

func generateCodeFile(path string, envs []Env) {
	tmpl := template.Must(template.New("code").Funcs(funcMap).Parse(codeTemplate))
	var buff bytes.Buffer
	if err := tmpl.Execute(&buff, envs); err != nil {
		panic(err)
	}
	code, err := format.Source(buff.Bytes())
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, code, 0644); err != nil {
		panic(err)
	}
}

const codeTemplate = `package configs

import (
    "fmt"
    "os"
    "strings"
    "github.com/spf13/viper"
)

var ErrNotDefined = fmt.Errorf("variable not defined")

func init() {
    viper.AutomaticEnv()
}

const (
{{ range . -}}
    {{ toConstName .Name }} = "{{ .Name }}"
{{ end }}
{{ range . -}}
    {{ if .File -}}{{ toConstName .Name }}_FILE = "{{ .Name }}_FILE"{{ end }}
{{ end -}}
)

func SetDefaults() {
{{ range . }}
{{ if .Default }}    viper.SetDefault({{ toConstName .Name }}, "{{ defaultVal .Default }}")
{{ end }}
{{ end }}
}

{{ $services := getServices . -}}
{{ range $service := $services }}
type {{ capitalize $service }}Config struct {
{{ range $ }}
{{ if and (isUsedBy . $service) (not .Omit) -}}
    {{ toFieldName .Name }} {{ .GoType }} {{ mapstructure .Name }}
{{ end -}}
{{ end }}
}

func Load{{ capitalize $service }}Config() (*{{ capitalize $service }}Config, error) {
    SetDefaults()

    var cfg {{ capitalize $service }}Config
    var err error

{{ range $ }}
{{ if and (isUsedBy . $service) (not .Omit) -}}
    cfg.{{ toFieldName .Name }}, err = Get{{ toFieldName .Name }}()
    if err != nil && err != ErrNotDefined {
        return nil, fmt.Errorf("failed to get {{ .Name }}: %w", err)
    } else if err == ErrNotDefined {
        return nil, fmt.Errorf("{{ .Name }} is required for the {{ $service }} service: %w", err)
    }
{{ end -}}
{{ end }}

    return &cfg, nil
}
{{ end }}

{{ range . }}
func Get{{ toFieldName .Name }}() ({{ .GoType }}, error) {
    s := viper.GetString({{ toConstName .Name }})
    {{- if .File }}
    if s == "" {
        filename := viper.GetString({{toConstName .Name}}_FILE)
        contents, err := os.ReadFile(filename)
        if err != nil {
            return notDefined{{ .GoType }}(), fmt.Errorf("failed to parse %s: %w", {{ toConstName .Name }}_FILE, err)
        }
        s = strings.TrimSpace(string(contents))
    }
    {{- end }}
    if s != "" {
        v, err := {{ toGoFunc .GoType }}(s)
        if err != nil {
            return v, fmt.Errorf("failed to parse %s: %w", {{ toConstName .Name }}, err)
        }
        return v, nil
    }
    return notDefined{{ .GoType }}(), fmt.Errorf("%s: %w", {{ toConstName .Name }}, ErrNotDefined)
}
{{ end }}
`
