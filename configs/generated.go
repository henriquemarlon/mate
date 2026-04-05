package configs

import (
	"fmt"
	"github.com/spf13/viper"
	"os"
	"strings"
)

var ErrNotDefined = fmt.Errorf("variable not defined")

func init() {
	viper.AutomaticEnv()
}

const (
	ANKI_CONNECT_URL       = "MATE_ANKI_CONNECT_URL"
	ANKI_DECK_PREFIX       = "MATE_ANKI_DECK_PREFIX"
	ANKI_MODEL_NAME        = "MATE_ANKI_MODEL_NAME"
	ANTHROPIC_API_KEY      = "MATE_ANTHROPIC_API_KEY"
	DBSCAN_EPSILON         = "MATE_DBSCAN_EPSILON"
	DBSCAN_MIN_POINTS      = "MATE_DBSCAN_MIN_POINTS"
	DRIVE_CREDENTIALS      = "MATE_DRIVE_CREDENTIALS"
	DRIVE_FOLDER_ID        = "MATE_DRIVE_FOLDER_ID"
	GOOGLE_API_KEY         = "MATE_GOOGLE_API_KEY"
	GOOGLE_EMBEDDING_MODEL = "MATE_GOOGLE_EMBEDDING_MODEL"
	QDRANT_ADDRESS         = "MATE_QDRANT_ADDRESS"
	QDRANT_COLLECTION      = "MATE_QDRANT_COLLECTION"
	LOG_COLOR              = "MATE_LOG_COLOR"
	LOG_LEVEL              = "MATE_LOG_LEVEL"
	SKILLS_DIR             = "MATE_SKILLS_DIR"
	STATE_DB_PATH          = "MATE_STATE_DB_PATH"

	ANTHROPIC_API_KEY_FILE = "MATE_ANTHROPIC_API_KEY_FILE"

	DRIVE_CREDENTIALS_FILE = "MATE_DRIVE_CREDENTIALS_FILE"

	GOOGLE_API_KEY_FILE = "MATE_GOOGLE_API_KEY_FILE"
)

func SetDefaults() {

	viper.SetDefault(ANKI_CONNECT_URL, "http://localhost:8765")

	viper.SetDefault(ANKI_DECK_PREFIX, "Mate")

	viper.SetDefault(ANKI_MODEL_NAME, "Basic")

	viper.SetDefault(DBSCAN_EPSILON, "0.3")

	viper.SetDefault(DBSCAN_MIN_POINTS, "2")

	viper.SetDefault(GOOGLE_EMBEDDING_MODEL, "text-embedding-004")

	viper.SetDefault(QDRANT_ADDRESS, "localhost:6334")

	viper.SetDefault(QDRANT_COLLECTION, "notes")

	viper.SetDefault(LOG_COLOR, "true")

	viper.SetDefault(LOG_LEVEL, "info")

	viper.SetDefault(SKILLS_DIR, "~/.mate/skills")

	viper.SetDefault(STATE_DB_PATH, "~/.mate/state.db")

}

type MateConfig struct {
	AnkiConnectUrl string `mapstructure:"MATE_ANKI_CONNECT_URL"`

	AnkiDeckPrefix string `mapstructure:"MATE_ANKI_DECK_PREFIX"`

	AnkiModelName string `mapstructure:"MATE_ANKI_MODEL_NAME"`

	AnthropicApiKey RedactedString `mapstructure:"MATE_ANTHROPIC_API_KEY"`

	DbscanEpsilon string `mapstructure:"MATE_DBSCAN_EPSILON"`

	DbscanMinPoints string `mapstructure:"MATE_DBSCAN_MIN_POINTS"`

	DriveCredentials RedactedString `mapstructure:"MATE_DRIVE_CREDENTIALS"`

	DriveFolderId string `mapstructure:"MATE_DRIVE_FOLDER_ID"`

	GoogleApiKey RedactedString `mapstructure:"MATE_GOOGLE_API_KEY"`

	GoogleEmbeddingModel string `mapstructure:"MATE_GOOGLE_EMBEDDING_MODEL"`

	QdrantAddress string `mapstructure:"MATE_QDRANT_ADDRESS"`

	QdrantCollection string `mapstructure:"MATE_QDRANT_COLLECTION"`

	LogColor bool `mapstructure:"MATE_LOG_COLOR"`

	LogLevel LogLevel `mapstructure:"MATE_LOG_LEVEL"`

	SkillsDir string `mapstructure:"MATE_SKILLS_DIR"`

	StateDbPath string `mapstructure:"MATE_STATE_DB_PATH"`
}

func LoadMateConfig() (*MateConfig, error) {
	SetDefaults()

	var cfg MateConfig
	var err error

	cfg.AnkiConnectUrl, err = GetAnkiConnectUrl()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_ANKI_CONNECT_URL: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_ANKI_CONNECT_URL is required for the mate service: %w", err)
	}

	cfg.AnkiDeckPrefix, err = GetAnkiDeckPrefix()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_ANKI_DECK_PREFIX: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_ANKI_DECK_PREFIX is required for the mate service: %w", err)
	}

	cfg.AnkiModelName, err = GetAnkiModelName()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_ANKI_MODEL_NAME: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_ANKI_MODEL_NAME is required for the mate service: %w", err)
	}

	cfg.AnthropicApiKey, err = GetAnthropicApiKey()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_ANTHROPIC_API_KEY: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_ANTHROPIC_API_KEY is required for the mate service: %w", err)
	}

	cfg.DbscanEpsilon, err = GetDbscanEpsilon()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_DBSCAN_EPSILON: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_DBSCAN_EPSILON is required for the mate service: %w", err)
	}

	cfg.DbscanMinPoints, err = GetDbscanMinPoints()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_DBSCAN_MIN_POINTS: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_DBSCAN_MIN_POINTS is required for the mate service: %w", err)
	}

	cfg.DriveCredentials, err = GetDriveCredentials()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_DRIVE_CREDENTIALS: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_DRIVE_CREDENTIALS is required for the mate service: %w", err)
	}

	cfg.DriveFolderId, err = GetDriveFolderId()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_DRIVE_FOLDER_ID: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_DRIVE_FOLDER_ID is required for the mate service: %w", err)
	}

	cfg.GoogleApiKey, err = GetGoogleApiKey()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_GOOGLE_API_KEY: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_GOOGLE_API_KEY is required for the mate service: %w", err)
	}

	cfg.GoogleEmbeddingModel, err = GetGoogleEmbeddingModel()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_GOOGLE_EMBEDDING_MODEL: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_GOOGLE_EMBEDDING_MODEL is required for the mate service: %w", err)
	}

	cfg.QdrantAddress, err = GetQdrantAddress()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_QDRANT_ADDRESS: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_QDRANT_ADDRESS is required for the mate service: %w", err)
	}

	cfg.QdrantCollection, err = GetQdrantCollection()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_QDRANT_COLLECTION: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_QDRANT_COLLECTION is required for the mate service: %w", err)
	}

	cfg.LogColor, err = GetLogColor()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_LOG_COLOR: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_LOG_COLOR is required for the mate service: %w", err)
	}

	cfg.LogLevel, err = GetLogLevel()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_LOG_LEVEL: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_LOG_LEVEL is required for the mate service: %w", err)
	}

	cfg.SkillsDir, err = GetSkillsDir()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_SKILLS_DIR: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_SKILLS_DIR is required for the mate service: %w", err)
	}

	cfg.StateDbPath, err = GetStateDbPath()
	if err != nil && err != ErrNotDefined {
		return nil, fmt.Errorf("failed to get MATE_STATE_DB_PATH: %w", err)
	} else if err == ErrNotDefined {
		return nil, fmt.Errorf("MATE_STATE_DB_PATH is required for the mate service: %w", err)
	}

	return &cfg, nil
}

func GetAnkiConnectUrl() (string, error) {
	s := viper.GetString(ANKI_CONNECT_URL)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", ANKI_CONNECT_URL, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", ANKI_CONNECT_URL, ErrNotDefined)
}

func GetAnkiDeckPrefix() (string, error) {
	s := viper.GetString(ANKI_DECK_PREFIX)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", ANKI_DECK_PREFIX, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", ANKI_DECK_PREFIX, ErrNotDefined)
}

func GetAnkiModelName() (string, error) {
	s := viper.GetString(ANKI_MODEL_NAME)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", ANKI_MODEL_NAME, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", ANKI_MODEL_NAME, ErrNotDefined)
}

func GetAnthropicApiKey() (RedactedString, error) {
	s := viper.GetString(ANTHROPIC_API_KEY)
	if s == "" {
		filename := viper.GetString(ANTHROPIC_API_KEY_FILE)
		contents, err := os.ReadFile(filename)
		if err != nil {
			return notDefinedRedactedString(), fmt.Errorf("failed to parse %s: %w", ANTHROPIC_API_KEY_FILE, err)
		}
		s = strings.TrimSpace(string(contents))
	}
	if s != "" {
		v, err := toRedactedString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", ANTHROPIC_API_KEY, err)
		}
		return v, nil
	}
	return notDefinedRedactedString(), fmt.Errorf("%s: %w", ANTHROPIC_API_KEY, ErrNotDefined)
}

func GetDbscanEpsilon() (string, error) {
	s := viper.GetString(DBSCAN_EPSILON)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", DBSCAN_EPSILON, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", DBSCAN_EPSILON, ErrNotDefined)
}

func GetDbscanMinPoints() (string, error) {
	s := viper.GetString(DBSCAN_MIN_POINTS)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", DBSCAN_MIN_POINTS, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", DBSCAN_MIN_POINTS, ErrNotDefined)
}

func GetDriveCredentials() (RedactedString, error) {
	s := viper.GetString(DRIVE_CREDENTIALS)
	if s == "" {
		filename := viper.GetString(DRIVE_CREDENTIALS_FILE)
		contents, err := os.ReadFile(filename)
		if err != nil {
			return notDefinedRedactedString(), fmt.Errorf("failed to parse %s: %w", DRIVE_CREDENTIALS_FILE, err)
		}
		s = strings.TrimSpace(string(contents))
	}
	if s != "" {
		v, err := toRedactedString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", DRIVE_CREDENTIALS, err)
		}
		return v, nil
	}
	return notDefinedRedactedString(), fmt.Errorf("%s: %w", DRIVE_CREDENTIALS, ErrNotDefined)
}

func GetDriveFolderId() (string, error) {
	s := viper.GetString(DRIVE_FOLDER_ID)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", DRIVE_FOLDER_ID, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", DRIVE_FOLDER_ID, ErrNotDefined)
}

func GetGoogleApiKey() (RedactedString, error) {
	s := viper.GetString(GOOGLE_API_KEY)
	if s == "" {
		filename := viper.GetString(GOOGLE_API_KEY_FILE)
		contents, err := os.ReadFile(filename)
		if err != nil {
			return notDefinedRedactedString(), fmt.Errorf("failed to parse %s: %w", GOOGLE_API_KEY_FILE, err)
		}
		s = strings.TrimSpace(string(contents))
	}
	if s != "" {
		v, err := toRedactedString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", GOOGLE_API_KEY, err)
		}
		return v, nil
	}
	return notDefinedRedactedString(), fmt.Errorf("%s: %w", GOOGLE_API_KEY, ErrNotDefined)
}

func GetGoogleEmbeddingModel() (string, error) {
	s := viper.GetString(GOOGLE_EMBEDDING_MODEL)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", GOOGLE_EMBEDDING_MODEL, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", GOOGLE_EMBEDDING_MODEL, ErrNotDefined)
}

func GetQdrantAddress() (string, error) {
	s := viper.GetString(QDRANT_ADDRESS)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", QDRANT_ADDRESS, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", QDRANT_ADDRESS, ErrNotDefined)
}

func GetQdrantCollection() (string, error) {
	s := viper.GetString(QDRANT_COLLECTION)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", QDRANT_COLLECTION, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", QDRANT_COLLECTION, ErrNotDefined)
}

func GetLogColor() (bool, error) {
	s := viper.GetString(LOG_COLOR)
	if s != "" {
		v, err := toBool(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", LOG_COLOR, err)
		}
		return v, nil
	}
	return notDefinedbool(), fmt.Errorf("%s: %w", LOG_COLOR, ErrNotDefined)
}

func GetLogLevel() (LogLevel, error) {
	s := viper.GetString(LOG_LEVEL)
	if s != "" {
		v, err := toLogLevel(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", LOG_LEVEL, err)
		}
		return v, nil
	}
	return notDefinedLogLevel(), fmt.Errorf("%s: %w", LOG_LEVEL, ErrNotDefined)
}

func GetSkillsDir() (string, error) {
	s := viper.GetString(SKILLS_DIR)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", SKILLS_DIR, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", SKILLS_DIR, ErrNotDefined)
}

func GetStateDbPath() (string, error) {
	s := viper.GetString(STATE_DB_PATH)
	if s != "" {
		v, err := toString(s)
		if err != nil {
			return v, fmt.Errorf("failed to parse %s: %w", STATE_DB_PATH, err)
		}
		return v, nil
	}
	return notDefinedstring(), fmt.Errorf("%s: %w", STATE_DB_PATH, ErrNotDefined)
}
